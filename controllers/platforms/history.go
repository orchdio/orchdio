package platforms

import (
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
)

// FetchTrackListeningHistory fetches the recently played tracks for a user
func (p *Platforms) FetchTrackListeningHistory(ctx *fiber.Ctx) error {
	log.Println("[platforms][FetchListeningHistory] info - Fetching listening history")
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	userCtx := ctx.Locals("userCtx").(*blueprint.AuthMiddlewareUserInfo)
	platform := userCtx.Platform

	switch platform {
	case applemusic.IDENTIFIER:
		// TODO: implement fetching user listening history from apple music api
		history, err := applemusic.FetchTrackListeningHistory(userCtx.RefreshToken)
		if err != nil {
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal server error", err.Error())
		}
		return util.SuccessResponse(ctx, http.StatusOK, history)

	case deezer.IDENTIFIER:
		history, err := deezer.FetchTracksListeningHistory(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchListeningHistory] error - %s", err.Error())
			if err.Error() == "unauthorized" {
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "App is not authorized to access this resource")
			}

			if err.Error() == "forbidden" {
				return util.ErrorResponse(ctx, fiber.StatusForbidden, "forbidden", "User has not granted access to this resource. Please re-authenticate")
			}

			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch listening history from Deezer")
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, history)

	case spotify.IDENTIFIER:
		credBytes, err := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchListeningHistory] error - could not decrypt app's credentials while fetching user library listening history%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch listening history from Spotify")
		}
		var credentials blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("[platforms][FetchListeningHistory] error - could not unmarshal app's credentials while fetching user library listening history%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch listening history from Spotify")
		}

		spotifyService := spotify.NewService(credentials.AppID, credentials.AppSecret, p.Redis)
		history, err := spotifyService.FetchTrackListeningHistory(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchListeningHistory] error - %s", err.Error())
			if err.Error() == "unauthorized" {
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "App is not authorized to access this resource")
			}

			if err.Error() == "forbidden" {
				return util.ErrorResponse(ctx, fiber.StatusForbidden, "forbidden", "User has not granted access to this resource")
			}
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch listening history from Spotify")
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, history)
	}
	return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Fetching listening history from this platform is not supported yet")
}
