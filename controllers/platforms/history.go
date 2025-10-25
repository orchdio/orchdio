package platforms

import (
	"fmt"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/universal"
	"orchdio/util"
	"os"

	"github.com/gofiber/fiber/v2"
)

// FetchTrackListeningHistory fetches the recently played tracks for a user
func (p *Platforms) FetchTrackListeningHistory(ctx *fiber.Ctx) error {
	log.Println("[platforms][FetchListeningHistory] info - Fetching listening history")
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	userCtx := ctx.Locals("userCtx").(*blueprint.AuthMiddlewareUserInfo)
	userId := ctx.Params("userId")
	platform := userCtx.Platform

	database := db.NewDB{DB: p.DB}
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), platform)
	if err != nil {
		log.Printf("[platforms][FetchPlatformArtists] error - error fetching user %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}

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

	history, err := universal.FetchListeningHistory(platform, refreshToken, app.UID.String(), p.DB, p.Redis)

	if err != nil {
		log.Printf("\n[controllers][platforms][%s][FetchListeningHistory] error - %v\n", platform, err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", fmt.Sprintf("Could not fetch user library listening history on platform %s", platform))
	}

	var resp = blueprint.UserListeningHistory{
		Data:  history,
		Total: len(history),
	}

	return util.SuccessResponse(ctx, http.StatusOK, resp)
}
