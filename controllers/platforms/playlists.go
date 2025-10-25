package platforms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/universal"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/oauth2"

	spotify2 "github.com/zmb3/spotify/v2"
)

// AddPlaylistToAccount adds a playlist to a user's account
func (p *Platforms) AddPlaylistToAccount(ctx *fiber.Ctx) error {
	// get the platform they want to add the playlist to
	platform := ctx.Params("platform")
	if platform == "" {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No platform in context")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Platform not found")
	}

	webhookSender := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)

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
	case spotify.IDENTIFIER:
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
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error unmarshalling credentials - could not decrypt integration credentials for platform %s: %v\n", platform, err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while unmarshalling credentials")
	}
	var credentials blueprint.IntegrationCredentials
	err = json.Unmarshal(cred, &credentials)
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error unmarshalling credentials - could not unmarshal integration credentials for platform %s: %v\n", platform, err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while unmarshalling credentials")
	}

	//title := fmt.Sprintf("Zoove playlist: %s", createBodyData.Title)
	description := "powered by Orchdio. https://orchdio.com"
	playlistlink := ""
	switch platform {
	// todo: fix this, dont use magic string, similar to other platforms
	case spotify.IDENTIFIER:
		spotifyService := spotify.NewService(&credentials, p.DB, p.Redis, app, webhookSender)
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
				return util.ErrorResponse(ctx, http.StatusInternalServerError, blueprint.ErrUnAuthorized, "Please reauthenticate this app with the permission to read playlists and try again.")
			}
		}

		var trackIds []spotify2.ID
		for _, track := range createBodyData.Tracks {
			if track != "" {
				trackIds = append(trackIds, spotify2.ID(track))
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

		deezerService := deezer.NewService(&credentials, p.DB, p.Redis, app, webhookSender)
		id, err := deezerService.CreateNewPlaylist(createBodyData.Title, user.PlatformID, string(t), createBodyData.Tracks)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create a new playlist for user")
		}

		playlistlink = fmt.Sprintf("https://www.deezer.com/en/playlist/%s", id)

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", createBodyData.Title)

	case applemusic.IDENTIFIER:
		applemusicService := applemusic.NewService(&credentials, p.DB, p.Redis, app)
		pl, err := applemusicService.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount][error] - an error occurred while adding playlist to user platform account - %v\n", err)
			if err == blueprint.ErrForbidden {
				log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
		}

	case tidal.IDENTIFIER:
		tidalService := tidal.NewService(&credentials, p.DB, p.Redis, app, webhookSender)
		pl, err := tidalService.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount][error] - an error occurred while adding playlist to user platform account - %v\n", err)
			if err == blueprint.ErrForbidden {
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
	platform := ctx.Params("platform")

	if userId == "" {
		log.Printf("[platforms][FetchPlatformPlaylists] error - userId is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "userId is empty")
	}
	if platform == "" {
		log.Printf("[platforms][FetchPlatformPlaylists] error - targetPlatform is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "targetPlatform is empty")
	}

	log.Printf("[platforms][FetchPlatformPlaylists] App %s is trying to fetch user %s's %s playlists\n", app.Name, userId, strings.ToUpper(platform))

	// get the user via the id to make sure the user exists
	database := db.NewDB{DB: p.DB}
	//user, err := database.FindUserByUUID(userId, targetPlatform)
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), platform)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[platforms][FetchPlatformPlaylists] error - user not found %v\n", err)
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User not found")
		}
		log.Printf("[platforms][FetchPlatformPlaylists] error - error fetching user %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	log.Printf("[platforms][FetchPlatformPlaylists] user found. Target platform is %v %s\n", user.Username, platform)

	var refreshToken string
	// if the user refresh token is nil, the user has not connected this platform to Orchdio.
	// this is because everytime a user connects a platform to Orchdio, the refresh token is updated for the platform the user connected
	if user.RefreshToken == nil && platform != "tidal" {
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

	libraryPlaylists, err := universal.FetchLibraryPlaylists(platform, refreshToken, app.UID.String(), p.DB, p.Redis)

	if err != nil {
		log.Printf("\n[controllers][platforms][%s][FetchLibraryPlaylists] error - %v\n", platform, err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", fmt.Sprintf("Could not fetch user library albums on platform %s", platform))
	}

	return util.SuccessResponse(ctx, http.StatusOK, libraryPlaylists)
}
