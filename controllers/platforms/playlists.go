package platforms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
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

	//// get the playlist ID
	//playlistID := ctx.Params("playlistId")
	//if playlistID == "" {
	//	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No playlist ID in context")
	//	return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request ,"Playlist ID not found")
	//}

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

	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] incoming body - %v\n", createBodyData)

	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - got user %v\n", createBodyData.User)

	// find the user in the database
	database := db.NewDB{DB: p.DB}
	user, err := database.FindUserByEmail(createBodyData.User, platform)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "App not found")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "App not found")
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

	//title := fmt.Sprintf("Zoove playlist: %s", createBodyData.Title)
	description := "powered by Orchdio. https://orchdio.com"
	playlistlink := ""
	switch platform {
	case "spotify":
		httpClient := spotifyauth.New().Client(context.Background(), &oauth2.Token{
			RefreshToken: string(t),
		})

		client := spotify.New(httpClient)
		createdPlaylist, err := client.CreatePlaylistForUser(context.Background(), user.PlatformID, createBodyData.Title, description, true, false)

		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error getting profile - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Internal server error")
		}

		var trackIds []spotify.ID
		for _, track := range createBodyData.Tracks {
			if track != "" {
				trackIds = append(trackIds, spotify.ID(track))
			}
		}

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - track ids %v\n", len(trackIds))
		// update playlist with the tracks
		updated, err := client.AddTracksToPlaylist(context.Background(), createdPlaylist.ID, trackIds...)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error getting profile - %v\n", err)
			if err.Error() == "No tracks specified." {
				return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", err.Error())
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err.Error(), "An internal error occurred")
		}

		playlistlink = createdPlaylist.ExternalURLs["spotify"]

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", updated)

	case "deezer":
		id, err := deezer.CreateNewPlaylist(createBodyData.Title, user.PlatformID, string(t), createBodyData.Tracks)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create a new playlist for user")
		}

		playlistlink = fmt.Sprintf("https://www.deezer.com/en/playlist/%s", id)

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", createBodyData.Title)
	// get the user, to see if our token is valid

	case "applemusic":
		pl, err := applemusic.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount][error] - an error occurred while adding playlist to user platform account - %v\n", err)
			if err == blueprint.EFORBIDDEN {
				log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
		}

	case "tidal":
		pl, err := tidal.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
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
	user, err := database.FindUserByUUID(userId, targetPlatform)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[platforms][FetchPlatformPlaylists] error - user not found %v\n", err)
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User not found")
		}
		log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching user %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	log.Printf("[platforms][FetchPlatformPlaylists] user found. Target platform is %v %s\n", user.Usernames, targetPlatform)

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
		playlists, err := deezer.FetchUserPlaylists(refreshToken)
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
			Total: playlists.Total,
			Data:  userPlaylists,
		}
		return util.SuccessResponse(ctx, http.StatusOK, response)
	case "spotify":
		// get the spotify playlists
		playlists, err := spotify2.FetchUserPlaylist(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching spotify playlists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		log.Printf("[platforms][FetchPlatformPlaylists] spotify playlists fetched successfully")
		// create a slice of UserLibraryPlaylists
		var userPlaylists []blueprint.UserPlaylist
		for _, playlist := range playlists.Playlists {
			log.Printf("[platforms][FetchPlatformPlaylists] playlist info url is- %v\n", playlist.Endpoint)
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
			Total: playlists.Total,
			Data:  userPlaylists,
		}
		return util.SuccessResponse(ctx, http.StatusOK, response)

	case tidal.IDENTIFIER:
		// get the tidal playlists
		playlists, err := tidal.FetchUserPlaylists()
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
			Total: playlists.TotalNumberOfItems,
			Data:  userPlaylists,
		}
		return util.SuccessResponse(ctx, http.StatusOK, response)

	case applemusic.IDENTIFIER:
		// get the apple music playlists
		playlists, err := applemusic.FetchUserPlaylists(refreshToken)
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
			Total: len(playlists),
			Data:  userPlaylists,
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
	user, err := database.FindUserByUUID(userId, platform)
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
		artists, err := applemusic.FetchUserArtists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching apple music artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case deezer.IDENTIFIER:
		// get the deezer artists
		log.Printf("[platforms][FetchPlatformArtists] fetching deezer artists\n")
		artists, err := deezer.FetchUserArtists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching deezer artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case spotify2.IDENTIFIER:
		// get the spotify artists
		log.Printf("[platforms][FetchPlatformArtists] fetching spotify artists\n")
		artists, err := spotify2.FetchUserArtists(refreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching spotify artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case tidal.IDENTIFIER:
		// get the tidal artists
		log.Printf("[platforms][FetchPlatformArtists] fetching tidal artists\n")
		// deserialize the user platform ids
		var platformIds map[string]string
		err := json.Unmarshal(user.PlatformIDs.([]byte), &platformIds)
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error deserializing platform ids %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		artists, err := tidal.FetchUserArtists(platformIds["tidal"])
		if err != nil {
			log.Printf("[platforms][FetchPlatformArtists] error - error fetching tidal artists %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	}
	return util.ErrorResponse(ctx, http.StatusNotImplemented, "not implemented", "This platform is not yet supported")
}
