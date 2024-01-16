package platforms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/gofiber/fiber/v2"
	"github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	logger2 "orchdio/logger"
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
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	spew.Dump("App trying to add...", app)
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", app.PublicKey.String()).String,
		Platform:             zap.String("platform", platform).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)
	p.Logger = orchdioLogger

	if platform == "" {
		p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - No platform in context", zap.String("platform", platform),
			zap.String("app_pub_key", app.PublicKey.String()), zap.String("request_id", ctx.Get("x-orchdio-request-id")))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Platform not found")
	}

	// get the playlist creation body
	var createBodyData = struct {
		User   string   `json:"user"`
		Title  string   `json:"title"`
		Tracks []string `json:"tracks"`
	}{}

	err := ctx.BodyParser(&createBodyData)
	if err != nil {
		p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error parsing request body", zap.Error(err),
			zap.String("app_pub_key", app.PublicKey.String()), zap.String("request_id", ctx.Get("x-orchdio-request-id")))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body. Please make sure the body is valid.")
	}

	if len(createBodyData.Tracks) == 0 {
		p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - No tracks in playlist", zap.String("app_pub_key", app.PublicKey.String()), zap.String("request_id", ctx.Get("x-orchdio-request-id")))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No tracks to insert into playlist. please add tracks to the playlist")
	}

	if createBodyData.Title == "" {
		p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - No title in playlist", zap.String("app_pub_key", app.PublicKey.String()), zap.String("request_id", ctx.Get("x-orchdio-request-id")))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No title to insert into playlist. please add title to the playlist")
	}

	// find the user in the database
	user := &blueprint.UserAppAndPlatformInfo{}
	database := db.New(p.DB, p.Logger)
	// get the user thats specified in the body
	_user, err := database.FetchPlatformAndUserInfoByIdentifier(createBodyData.User, app.UID.String(), platform)
	// copy it to a new variable. this is to allow use shadow it later on in the workaround for Ayomide's tidal support.
	user = _user

	if err != nil {
		// work around to support me adding my own playlist
		if errors.Is(err, sql.ErrNoRows) && !(createBodyData.User == "onigbindeayomide@gmail.com" || createBodyData.User == os.Getenv("MY_ID") && platform == tidal.IDENTIFIER) {
			p.Logger.Warn("[controllers][platforms][AddPlaylistToAccount] error - App not found for user. User has not authorized this app for the platform access.", zap.String("app_pub_key", app.PublicKey.String()), zap.String("platform", platform), zap.String("request_id", ctx.Get("x-orchdio-request-id")))
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthenticated", "User has not authorized this app. Please authorize the app to continue.")
		} else {
			if err != nil {
				p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error fetching user", zap.Error(err),
					zap.String("app_pub_key", app.PublicKey.String()), zap.String("request_id", ctx.Get("x-orchdio-request-id")))
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
			}
		}
	}

	// workaround for admin support for tidal.
	if createBodyData.User == "onigbindeayomide@gmail.com" || createBodyData.User == os.Getenv("MY_ID") && platform == tidal.IDENTIFIER {
		//_user2, dErr := database.FetchPlatformAndUserInfoByIdentifier("onigbindeayomide@gmail.com", app.UID.String(), platform)
		//if dErr != nil {
		//	p.Logger.Error("Error fetching Admin's platform info using identifier", zap.Error(err))
		//	return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "An internal error occurred.")
		//}
		//user = _user2

	}

	// get the user's access token
	t, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))

	if err != nil {
		p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error decrypting user refresh token", zap.Error(err))
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
		p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error decrypting integration credentials", zap.Error(err), zap.String("platform", platform))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while unmarshalling credentials")
	}
	var credentials blueprint.IntegrationCredentials
	err = json.Unmarshal(cred, &credentials)
	if err != nil {
		p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error unmarshalling integration credentials", zap.Error(err), zap.String("platform", platform))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while unmarshalling credentials")
	}

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
			p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error creating new playlist for user", zap.Error(pErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
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
		_, cErr := client.AddTracksToPlaylist(context.Background(), createdPlaylist.ID, trackIds...)
		if cErr != nil {
			p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error adding new track to playlist", zap.Error(cErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			if cErr.Error() == "No tracks specified." {
				return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", cErr.Error())
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, cErr.Error(), "An internal error occurred")
		}
		playlistlink = createdPlaylist.ExternalURLs["spotify"]
		p.Logger.Info("[controllers][platforms][AddPlaylistToAccount] - created playlist", zap.String("playlist_link", playlistlink), zap.String("platform", platform), zap.String("app_id", app.UID.String()))

	case deezer.IDENTIFIER:
		deezerService := deezer.NewService(&credentials, p.DB, p.Redis, p.Logger)
		id, dErr := deezerService.CreateNewPlaylist(createBodyData.Title, user.PlatformID, string(t), createBodyData.Tracks)
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error creating new playlist for user", zap.Error(dErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create a new playlist for user")
		}

		playlistlink = fmt.Sprintf("https://www.deezer.com/en/playlist/%s", id)
		p.Logger.Info("[controllers][platforms][AddPlaylistToAccount] - created playlist", zap.String("playlist_link", playlistlink), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
	// get the user, to see if our token is valid

	case applemusic.IDENTIFIER:
		applemusicService := applemusic.NewService(&credentials, p.DB, p.Redis, p.Logger)
		pl, cErr := applemusicService.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if cErr != nil {
			if errors.Is(cErr, blueprint.EFORBIDDEN) {
				p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error creating new playlist for user", zap.Error(cErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
			p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error creating new playlist for user", zap.Error(cErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
		}

	case tidal.IDENTIFIER:
		tidalService := tidal.NewService(&credentials, p.DB, p.Redis)
		pl, cErr := tidalService.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if cErr != nil {
			if errors.Is(cErr, blueprint.EFORBIDDEN) {
				p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error creating new playlist for user", zap.Error(cErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
			p.Logger.Error("[controllers][platforms][AddPlaylistToAccount] error - error creating new playlist for user", zap.Error(cErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
		}
	}
	return util.SuccessResponse(ctx, http.StatusCreated, playlistlink)
}

// FetchPlatformPlaylists fetches the all the playlists on a user's platform library
func (p *Platforms) FetchPlatformPlaylists(ctx *fiber.Ctx) error {
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	userId := ctx.Params("userId")
	targetPlatform := ctx.Params("platform")
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", app.PublicKey.String()).String,
		Platform:             zap.String("platform", targetPlatform).String,
		AppID:                zap.String("app_id", app.UID.String()).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)
	p.Logger = orchdioLogger

	p.Logger.Info("[controllers][platforms][FetchPlatformPlaylists] - fetching user's playlists", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
	if userId == "" {
		p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - userId is empty", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "userId is empty")
	}
	if targetPlatform == "" {
		p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - targetPlatform is empty", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "targetPlatform is empty")
	}

	p.Logger.Info("[controllers][platforms][FetchPlatformPlaylists] - fetching user's playlists", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))

	// get the user via the id to make sure the user exists
	database := db.NewDB{DB: p.DB}
	//user, err := database.FindUserByUUID(userId, targetPlatform)
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), targetPlatform)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - user not found", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User not found")
		}
		p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - error fetching user", zap.Error(err), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
	}
	p.Logger.Info("[controllers][platforms][FetchPlatformPlaylists] - user found", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
	var refreshToken string
	// if the user refresh token is nil, the user has not connected this platform to Orchdio.
	// this is because everytime a user connects a platform to Orchdio, the refresh token is updated for the platform the user connected
	if user.RefreshToken == nil && targetPlatform != "tidal" {
		p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - user's refresh token is empty", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
			zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "no access", "User has not connected this platform to Orchdio")
	}

	if user.RefreshToken != nil {
		// decrypt the user's access token
		r, dErr := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - error decrypting access token", zap.Error(dErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		refreshToken = string(r)
	}

	switch targetPlatform {
	case deezer.IDENTIFIER:
		// get the deezer playlists
		var deezerCredentials blueprint.IntegrationCredentials
		if app.DeezerCredentials == nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - deezer credentials is nil", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "internal error", "An unexpected error occurred")
		}

		cred, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - could not fetch user library playlist. error decrypting deezer credentials", zap.Error(decErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		err = json.Unmarshal(cred, &deezerCredentials)
		if err != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - could not fetch user tidal library playlist. error unmarshalling deezer credentials", zap.Error(err), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		deezerService := deezer.NewService(&deezerCredentials, p.DB, p.Redis, p.Logger)
		playlists, dErr := deezerService.FetchUserPlaylists(refreshToken)
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - could not fetch user deezer library playlist. error fetching deezer playlists", zap.Error(dErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		p.Logger.Info("[controllers][platforms][FetchPlatformPlaylists] - deezer playlists fetched successfully", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
			zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
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
		credBytes, dErr := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - could not fetch user spotify library playlist. error decrypting spotify credentials", zap.Error(dErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		// unmarshal the credentials
		var spotifyCred blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &spotifyCred)
		if err != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - could not fetch user spotify library playlist. error unmarshalling spotify credentials", zap.Error(err), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		// get the spotify playlists
		spotifyService := spotify2.NewService(&spotifyCred, p.DB, p.Redis)
		playlists, pErr := spotifyService.FetchUserPlaylist(refreshToken)
		if pErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - could not fetch user spotify library playlist. unknown error while fetching playlist", zap.Error(pErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		p.Logger.Info("[controllers][platforms][FetchPlatformPlaylists] - spotify playlists fetched successfully", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
			zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
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
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - tidal credentials not found while fetching user library playlists",
				zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer does not have credentials setup to access this resource.")
		}
		credBytes, dErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - tidal credentials not found while fetching user library playlists",
				zap.Error(dErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		err = json.Unmarshal(credBytes, &tidalCredentials)
		if err != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - tidal credentials not found while fetching user library playlists",
				zap.Error(err), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		tidalService := tidal.NewService(&tidalCredentials, p.DB, p.Redis)
		playlists, pErr := tidalService.FetchUserPlaylists()
		if pErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - tidal credentials not found while fetching user library playlists",
				zap.Error(pErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()),
				zap.String("user_id", user.PlatformID), zap.String("user_platform", user.Platform))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		if playlists == nil {
			log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching tidal playlists %v\n", err)
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - tidal credentials not found while fetching user library playlists", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		p.Logger.Info("[controllers][platforms][FetchPlatformPlaylists] - tidal playlists fetched successfully", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
		// create a slice of UserLibraryPlaylists
		var userPlaylists []blueprint.UserPlaylist
		for _, playlist := range playlists.Items {
			if playlist.ItemType != "PLAYLIST" {
				p.Logger.Warn("[controllers][platforms][FetchPlatformPlaylists] - Item is not a playlist data, skipping...", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
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
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - apple music credentials not found while fetching user library playlists", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer does not have credentials setup to access this resource.")
		}
		credBytes, dErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - apple music credentials not found while fetching user library playlists", zap.Error(dErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		err = json.Unmarshal(credBytes, &appleMusicCredentials)
		if err != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - apple music credentials not found while fetching user library playlists", zap.Error(err), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		applemusicService := applemusic.NewService(&appleMusicCredentials, p.DB, p.Redis, p.Logger)
		playlists, pErr := applemusicService.FetchUserPlaylists(refreshToken)
		if pErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - apple music credentials not found while fetching user library playlists", zap.Error(pErr), zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		if playlists == nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformPlaylists] error - apple music credentials not found while fetching user library playlists", zap.String("platform", targetPlatform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
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
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	userId := ctx.Params("userId")
	platform := ctx.Params("platform")
	orchdioLogger := logger2.NewZapSentryLogger(&blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", app.PublicKey.String()).String,
		Platform:             zap.String("platform", platform).String,
		AppID:                zap.String("app_id", app.UID.String()).String,
	})
	p.Logger = orchdioLogger

	p.Logger.Info("[controllers][platforms][FetchPlatformArtists] - fetching user's artists", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
	refreshToken := ""

	if userId == "" {
		p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - no user id provided", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No user id provided")
	}

	if platform == "" {
		p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - no platform provided", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No platform provided")
	}
	p.Logger.Info("[controllers][platforms][FetchPlatformArtists] - fetching user's artists", zap.String("platform", platform), zap.String("app_id", app.UID.String()), zap.String("app_name", app.Name))
	// get the user
	database := db.NewDB{DB: p.DB}
	//user, err := database.FindUserByUUID(userId, platform)
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), platform)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User not found")
		}
		p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error fetching user", zap.Error(err), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
	}

	if user.RefreshToken == nil && platform != "tidal" {
		p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - no refresh token found for user", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "No refresh token found for user")
	}

	if user.RefreshToken != nil {
		r, dErr := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error decrypting refresh token", zap.Error(dErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		refreshToken = string(r)
	}

	switch platform {
	case applemusic.IDENTIFIER:
		// get the apple music artists
		p.Logger.Info("[controllers][platforms][FetchPlatformArtists] - fetching user's artists", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
		var appleMusicCredentials blueprint.IntegrationCredentials
		if app.AppleMusicCredentials == nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - no apple music credentials found", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer has not set up apple music credentials")
		}
		credBytes, dErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error decrypting apple music credentials", zap.Error(dErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		decErr := json.Unmarshal(credBytes, &appleMusicCredentials)
		if decErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error unmarshalling apple music credentials", zap.Error(decErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		applemusicService := applemusic.NewService(&appleMusicCredentials, p.DB, p.Redis, p.Logger)
		artists, pErr := applemusicService.FetchUserArtists(refreshToken)
		if pErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error fetching apple music artists", zap.Error(pErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case deezer.IDENTIFIER:
		var deezerCredentials blueprint.IntegrationCredentials
		if app.DeezerCredentials == nil {
			p.Logger.Warn("[controllers][platforms][FetchPlatformArtists] error - developer has not set deezer credentials found", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer has not set up deezer credentials")
		}

		decErr := json.Unmarshal(app.DeezerCredentials, &deezerCredentials)
		if decErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error unmarshalling deezer credentials", zap.Error(decErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		// get the deezer artists
		p.Logger.Info("[controllers][platforms][FetchPlatformArtists] - fetching user's artists", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
		deezerService := deezer.NewService(&deezerCredentials, p.DB, p.Redis, p.Logger)
		artists, dErr := deezerService.FetchUserArtists(refreshToken)
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error fetching deezer artists", zap.Error(dErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case spotify2.IDENTIFIER:
		// get the spotify artists
		log.Printf("[platforms][FetchPlatformArtists] fetching spotify artists\n")
		credBytes, dErr := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error decrypting spotify credentials", zap.Error(dErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		var spotifyCreds blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &spotifyCreds)
		if err != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error unmarshalling spotify credentials", zap.Error(err), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}

		spotifyService := spotify2.NewService(&spotifyCreds, p.DB, p.Redis)
		artists, pErr := spotifyService.FetchUserArtists(refreshToken)
		if pErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error fetching spotify artists", zap.Error(pErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	case tidal.IDENTIFIER:
		var tidalCredentials blueprint.IntegrationCredentials
		if app.TidalCredentials == nil {
			p.Logger.Warn("[controllers][platforms][FetchPlatformArtists] error - developer has not set tidal credentials found", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "authorization error", "The developer has not set up tidal credentials")
		}

		decErr := json.Unmarshal(app.TidalCredentials, &tidalCredentials)
		if decErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error unmarshalling tidal credentials", zap.Error(decErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		tidalService := tidal.NewService(&tidalCredentials, p.DB, p.Redis)
		// get the tidal artists
		p.Logger.Info("[controllers][platforms][FetchPlatformArtists] - fetching user's artists", zap.String("platform", platform), zap.String("app_id", app.UID.String()))
		// deserialize the user platform ids
		//var platformIds map[string]string
		//if err != nil {
		//	log.Printf("[platforms][FetchPlatformArtists] error - error deserializing platform ids %v\n", err)
		//	return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		//}
		//artists, err := tidal.FetchUserArtists(platformIds["tidal"])
		artists, pErr := tidalService.FetchUserArtists(user.PlatformID)
		if pErr != nil {
			p.Logger.Error("[controllers][platforms][FetchPlatformArtists] error - error fetching tidal artists", zap.Error(pErr), zap.String("platform", platform), zap.String("app_id", app.UID.String()))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
		}
		return util.SuccessResponse(ctx, http.StatusOK, artists)
	}
	return util.ErrorResponse(ctx, http.StatusNotImplemented, "not implemented", "This platform is not yet supported")
}
