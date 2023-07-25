package platforms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	spotify2 "orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/util"
	"os"
	"strconv"
	"strings"
)

// AddPlaylistToAccount adds a playlist to a user's account
func (p *Platforms) AddPlaylistToAccount(ctx *fiber.Ctx) error {
	// get the platform they want to add the playlist to
	platform := ctx.Params("platform")
	if platform == "" {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No platform in context")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Platform not found")
	}

	app := ctx.Locals("app").(*blueprint.DeveloperApp)

	// get the playlist creation body
	var createBodyData = struct {
		User   string   `json:"user"`
		Title  string   `json:"title"`
		Tracks []string `json:"tracks"`
	}{}

	err := ctx.BodyParser(&createBodyData)
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body. Please make sure the body is valid.")
	}

	if len(createBodyData.Tracks) == 0 {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No tracks in playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No tracks to insert into playlist. please add tracks to the playlist")
	}

	if createBodyData.Title == "" {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No title in playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No title to insert into playlist. please add title to the playlist")
	}
	// find the user in the database
	database := db.NewDB{DB: p.DB}
	user, err := database.FetchPlatformAndUserInfoByIdentifier(createBodyData.User, app.UID.String(), platform)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "App not found")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User has not authorized this app. Please authorize the app to continue.")
		}
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
	}

	// get the user's access token
	t, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))

	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error decrypting user refresh token - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while decrypting refresh token")
	}

	var credentialsBytes []byte
	switch platform {
	case spotify2.IDENTIFIER:
		credentialsBytes = app.SpotifyCredentials
	case tidal.IDENTIFIER:
		credentialsBytes = app.TidalCredentials
	case deezer.IDENTIFIER:
		credentialsBytes = app.DeezerCredentials
	case applemusic.IDENTIFIER:
		credentialsBytes = app.AppleMusicCredentials
	}

	cred, err := util.Decrypt(credentialsBytes, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error unmarshalling credentials - could not decrypt integration credentials for platform %s: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while unmarshalling credentials")
	}
	var credentials blueprint.IntegrationCredentials
	err = json.Unmarshal(cred, &credentials)
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error unmarshalling credentials - could not unmarshal integration credentials for platform %s: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while unmarshalling credentials")
	}

	//title := fmt.Sprintf("Zoove playlist: %s", createBodyData.Title)
	description := "powered by Orchdio. https://orchdio.com"
	playlistlink := ""
	switch platform {
	case spotify2.IDENTIFIER:
		spotifyService := spotify2.NewService(&credentials, p.DB, p.Redis)
		client := spotifyService.NewClient(context.Background(), &oauth2.Token{
			RefreshToken: string(t),
		})
		createdPlaylist, pErr := client.CreatePlaylistForUser(context.Background(), user.PlatformID, createBodyData.Title, description, true, false)

		if pErr != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist for user - %v\n", pErr.Error())
			if strings.Contains(pErr.Error(), "oauth2: cannot fetch token: 400 Bad Request") {
				return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid client. The user or developer app's credentials might be invalid")
			}
			if strings.Contains(pErr.Error(), "This request requires user authentication") {
				return util.ErrorResponse(ctx, http.StatusInternalServerError, blueprint.EUNAUTHORIZED, "Please reauthenticate this app with the permission to read playlists and try again.")
			}
		}

		var trackIds []spotify.ID
		for _, track := range createBodyData.Tracks {
			if track != "" {
				trackIds = append(trackIds, spotify.ID(track))
			}
		}

		// update playlist with the tracks
		updated, cErr := client.AddTracksToPlaylist(context.Background(), createdPlaylist.ID, trackIds...)
		if cErr != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error adding new track to playlist - %v\n", cErr)
			if cErr.Error() == "No tracks specified." {
				return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", cErr.Error())
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, cErr.Error(), "An internal error occurred")
		}

		playlistlink = createdPlaylist.ExternalURLs["spotify"]

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", updated)

	case deezer.IDENTIFIER:
		deezerService := deezer.NewService(&credentials, p.DB, p.Redis)
		id, err := deezerService.CreateNewPlaylist(createBodyData.Title, user.PlatformID, string(t), createBodyData.Tracks)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create a new playlist for user")
		}

		playlistlink = fmt.Sprintf("https://www.deezer.com/en/playlist/%s", id)

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", createBodyData.Title)
	// get the user, to see if our token is valid

	case applemusic.IDENTIFIER:
		applemusicService := applemusic.NewService(&credentials, p.DB, p.Redis)
		pl, err := applemusicService.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount][error] - an error occurred while adding playlist to user platform account - %v\n", err)
			if err == blueprint.EFORBIDDEN {
				log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
		}

	case tidal.IDENTIFIER:
		tidalService := tidal.NewService(&credentials, p.DB, p.Redis)
		pl, err := tidalService.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount][error] - an error occurred while adding playlist to user platform account - %v\n", err)
			if err == blueprint.EFORBIDDEN {
				log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
		}
	}
	return util.SuccessResponse(ctx, http.StatusCreated, playlistlink)
}

// FetchPlatformPlaylists fetches the all the playlists on a user's platform library
func (p *Platforms) FetchPlatformPlaylists(ctx *fiber.Ctx) error {
	log.Printf("[platforms][FetchPlatformPlaylists] fetching platform playlists")
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	userId := ctx.Params("userId")
	targetPlatform := ctx.Params("platform")
	if userId == "" {
		log.Printf("[platforms][FetchPlatformPlaylists] error - userId is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "userId is empty")
	}
	if targetPlatform == "" {
		log.Printf("[platforms][FetchPlatformPlaylists] error - targetPlatform is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "targetPlatform is empty")
	}

	log.Printf("[platforms][FetchPlatformPlaylists] App %s is trying to fetch user %s's %s playlists\n", app.Name, userId, strings.ToUpper(targetPlatform))

	// get the user via the id to make sure the user exists
	database := db.NewDB{DB: p.DB}
	//user, err := database.FindUserByUUID(userId, targetPlatform)
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), targetPlatform)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[platforms][FetchPlatformPlaylists] error - user not found %v\n", err)
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User not found")
		}
		log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching user %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	log.Printf("[platforms][FetchPlatformPlaylists] user found. Target platform is %v %s\n", user.Username, targetPlatform)

	var refreshToken string
	// if the user refresh token is nil, the user has not connected this platform to Orchdio.
	// this is because everytime a user connects a platform to Orchdio, the refresh token is updated for the platform the user connected
	if user.RefreshToken == nil && targetPlatform != "tidal" {
		log.Printf("[platforms][FetchPlatformPlaylists] error - user's refresh token is empty %v\n", err)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "no access", "User has not connected this platform to Orchdio")
	}

	if user.RefreshToken != nil {
		// decrypt the user's access token
		r, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error decrypting access token %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		refreshToken = string(r)
	}

	switch targetPlatform {
	case deezer.IDENTIFIER:
		// get the deezer playlists
		var deezerCredentials blueprint.IntegrationCredentials
		if app.DeezerCredentials == nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - deezer credentials is nil")
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "internal error", "An unexpected error occured")
		}

		cred, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error decrypting deezer credentials while fetching user library playlists%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		err = json.Unmarshal(cred, &deezerCredentials)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error unmarshalling deezer credentials while fetching user library playlists%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		deezerService := deezer.NewService(&deezerCredentials, p.DB, p.Redis)
		playlists, err := deezerService.FetchUserPlaylists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching deezer playlists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		log.Printf("[platforms][FetchPlatformPlaylists] deezer playlists fetched successfully")
		// create a slice of UserLibraryPlaylists
		var userPlaylists []blueprint.UserPlaylist
		for _, playlist := range playlists.Data {
			userPlaylists = append(userPlaylists, blueprint.UserPlaylist{
				ID:            strconv.Itoa(int(playlist.ID)),
				Title:         playlist.Title,
				Duration:      util.GetFormattedDuration(playlist.Duration),
				DurationMilis: playlist.Duration * 1000,
				Public:        playlist.Public,
				Collaborative: playlist.Collaborative,
				NbTracks:      playlist.NbTracks,
				Fans:          playlist.Fans,
				URL:           playlist.Link,
				Cover:         playlist.PictureMedium,
				CreatedAt:     playlist.CreationDate,
				Checksum:      playlist.Checksum,
				Owner:         playlist.Creator.Name,
			})
		}
		var response = blueprint.UserLibraryPlaylists{
			Total:   playlists.Total,
			Payload: userPlaylists,
		}
		return util.SuccessResponse(ctx, http.StatusOK, response)
	case spotify2.IDENTIFIER:
		// decrypt integration credentials of the app
		credBytes, err := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error decrypting spotify credentials while fetching user library playlist.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		// unmarshal the credentials
		var spotifyCred blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &spotifyCred)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error unmarshalling spotify credentials while fetching user library playlist.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		// get the spotify playlists
		spotifyService := spotify2.NewService(&spotifyCred, p.DB, p.Redis)
		playlists, err := spotifyService.FetchUserPlaylist(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching spotify playlists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		log.Printf("[platforms][FetchPlatformPlaylists] spotify playlists fetched successfully")
		// create a slice of UserLibraryPlaylists
		var userPlaylists []blueprint.UserPlaylist
		for _, playlist := range playlists.Playlists {
			pix := playlist.Images
			var cover string
			if len(pix) > 0 {
				cover = pix[0].URL
			}
			userPlaylists = append(userPlaylists, blueprint.UserPlaylist{
				ID:            string(playlist.ID),
				Title:         playlist.Name,
				Public:        playlist.IsPublic,
				Collaborative: playlist.Collaborative,
				NbTracks:      int(playlist.Tracks.Total),
				URL:           playlist.ExternalURLs["spotify"],
				Cover:         cover,
				Checksum:      playlist.SnapshotID,
				Owner:         playlist.Owner.DisplayName,
			})
		}
		var response = blueprint.UserLibraryPlaylists{
			Total:   playlists.Total,
			Payload: userPlaylists,
		}
		return util.SuccessResponse(ctx, http.StatusOK, response)

	case tidal.IDENTIFIER:
		// get the tidal playlists
		var tidalCredentials blueprint.IntegrationCredentials
		if app.TidalCredentials == nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - tidal credentials not found while fetching user library playlists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer does not have credentials setup to access this resource.")
		}
		credBytes, dErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error decrypting tidal credentials while fetching user library playlists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		err = json.Unmarshal(credBytes, &tidalCredentials)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error unmarshalling tidal credentials while fetching user library playlists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		tidalService := tidal.NewService(&tidalCredentials, p.DB, p.Redis)
		playlists, err := tidalService.FetchUserPlaylists()
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching tidal playlists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		if playlists == nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching tidal playlists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		log.Printf("[platforms][FetchPlatformPlaylists] tidal playlists fetched successfully")
		// create a slice of UserLibraryPlaylists
		var userPlaylists []blueprint.UserPlaylist
		for _, playlist := range playlists.Items {
			if playlist.ItemType != "PLAYLIST" {
				log.Printf("[platforms][FetchPlatformPlaylists] Item is not a playlist data, skipping...\n")
				continue
			}

			data := playlist.Data
			userPlaylists = append(userPlaylists, blueprint.UserPlaylist{
				ID:            data.UUID,
				Title:         data.Title,
				Public:        util.TidalIsPrivate(data.SharingLevel),
				Collaborative: util.TidalIsCollaborative(data.ContentBehavior),
				NbTracks:      data.NumberOfTracks,
				URL:           playlist.Data.URL,
				Cover:         playlist.Data.Image,
				CreatedAt:     data.Created,
				Owner:         playlist.Data.Creator.Name,
			})
		}
		var response = blueprint.UserLibraryPlaylists{
			Total:   playlists.TotalNumberOfItems,
			Payload: userPlaylists,
		}
		return util.SuccessResponse(ctx, http.StatusOK, response)

	case applemusic.IDENTIFIER:
		// get the apple music playlists
		var appleMusicCredentials blueprint.IntegrationCredentials
		if app.AppleMusicCredentials == nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - apple music credentials not found while fetching user library playlists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer does not have credentials setup to access this resource.")
		}
		credBytes, dErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error decrypting apple music credentials while fetching user library playlists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		err = json.Unmarshal(credBytes, &appleMusicCredentials)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error unmarshalling apple music credentials while fetching user library playlists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		applemusicService := applemusic.NewService(&appleMusicCredentials, p.DB, p.Redis)
		playlists, err := applemusicService.FetchUserPlaylists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching apple music playlists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		if playlists == nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching apple music playlists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		// create a slice of UserLibraryPlaylists
		var userPlaylists []blueprint.UserPlaylist
		for _, playlist := range playlists {
			userPlaylists = append(userPlaylists, blueprint.UserPlaylist{
				ID:            playlist.ID,
				Title:         playlist.Title,
				Public:        playlist.Public,
				Collaborative: playlist.Collaborative,
				Description:   playlist.Description,
				URL:           playlist.URL,
				Cover:         playlist.Cover,
				CreatedAt:     playlist.CreatedAt,
				NbTracks:      playlist.NbTracks,
				Owner:         playlist.Owner,
			})
		}
		var response = blueprint.UserLibraryPlaylists{
			Total:   len(playlists),
			Payload: userPlaylists,
		}

		return util.SuccessResponse(ctx, http.StatusOK, response)
	}
	return util.ErrorResponse(ctx, http.StatusNotImplemented, "not implemented", "This platform is not yet supported")
}

// FetchPlatformArtists fetches the artists from a given platform
func (p *Platforms) FetchPlatformArtists(ctx *fiber.Ctx) error {
	log.Printf("[platforms][FetchPlatformAlbums] fetching platform albums\n")
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	userId := ctx.Params("userId")
	platform := ctx.Params("platform")
	refreshToken := ""

	if userId == "" {
		log.Printf("[platforms][FetchPlatformArtists] error - no user id provided\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No user id provided")
	}

	if platform == "" {
		log.Printf("[platforms][FetchPlatformArtists] error - no platform provided\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No platform provided")
	}

	log.Printf("[platforms][FetchPlatformAlbums] app %s is trying to fetch %s's library artists on %s", app.Name, userId, platform)

	// get the user
	database := db.NewDB{DB: p.DB}
	//user, err := database.FindUserByUUID(userId, platform)
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), platform)
	if err != nil {
		log.Printf("[platforms][FetchPlatformArtists] error - error fetching user %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}

	if user.RefreshToken == nil && platform != "tidal" {
		log.Printf("[platforms][FetchPlatformArtists] error - no refresh token found for user %v\n", err)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "No refresh token found for user")
	}

	if user.RefreshToken != nil {
		r, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error decrypting refresh token %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		refreshToken = string(r)
	}

	switch platform {
	case applemusic.IDENTIFIER:
		// get the apple music artists
		log.Printf("[platforms][FetchPlatformArtists] fetching apple music artists\n")
		var appleMusicCredentials blueprint.IntegrationCredentials
		if app.AppleMusicCredentials == nil {
			log.Printf("[platforms][FetchPlatformArtists] error - no apple music credentials found\n")
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer has not set up apple music credentials")
		}
		credBytes, dErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error decrypting apple music credentials while fetching user library artists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		decErr := json.Unmarshal(credBytes, &appleMusicCredentials)
		if decErr != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error unmarshalling apple music credentials while fetching user library artists.%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		applemusicService := applemusic.NewService(&appleMusicCredentials, p.DB, p.Redis)
		artists, err := applemusicService.FetchUserArtists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching apple music artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case deezer.IDENTIFIER:
		var deezerCredentials blueprint.IntegrationCredentials
		if app.DeezerCredentials == nil {
			log.Printf("[platforms][FetchPlatformArtists] error - no deezer credentials found\n")
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer has not set up deezer credentials")
		}

		decErr := json.Unmarshal(app.DeezerCredentials, &deezerCredentials)
		if decErr != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error unmarshalling deezer credentials %v\n", decErr)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		// get the deezer artists
		log.Printf("[platforms][FetchPlatformArtists] fetching deezer artists\n")
		deezerService := deezer.NewService(&deezerCredentials, p.DB, p.Redis)
		artists, err := deezerService.FetchUserArtists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching deezer artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case spotify2.IDENTIFIER:
		// get the spotify artists
		log.Printf("[platforms][FetchPlatformArtists] fetching spotify artists\n")
		credBytes, err := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error decrypting spotify credentials while fetching user library spotify artists%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		var spotifyCreds blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &spotifyCreds)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error unmarshalling spotify credentials while fetching user library spotify artists%v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}

		spotifyService := spotify2.NewService(&spotifyCreds, p.DB, p.Redis)
		artists, err := spotifyService.FetchUserArtists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching spotify artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case tidal.IDENTIFIER:
		var tidalCredentials blueprint.IntegrationCredentials
		if app.TidalCredentials == nil {
			log.Printf("[platforms][FetchPlatformArtists] error - no tidal credentials found\n")
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer has not set up tidal credentials")
		}

		decErr := json.Unmarshal(app.TidalCredentials, &tidalCredentials)
		if decErr != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error unmarshalling tidal credentials %v\n", decErr)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		tidalService := tidal.NewService(&tidalCredentials, p.DB, p.Redis)
		// get the tidal artists
		log.Printf("[platforms][FetchPlatformArtists] fetching tidal artists\n")
		// deserialize the user platform ids
		//var platformIds map[string]string
		//if err != nil {
		//	log.Printf("[platforms][FetchPlatformArtists] error - error deserializing platform ids %v\n", err)
		//	return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		//}
		//artists, err := tidal.FetchUserArtists(platformIds["tidal"])
		artists, err := tidalService.FetchUserArtists(user.PlatformID)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching tidal artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	}
	return util.ErrorResponse(ctx, http.StatusNotImplemented, "not implemented", "This platform is not yet supported")
}
