package platforms

import (
	"github.com/gofiber/fiber/v2"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/util"
)

// FetchTrackListeningHistory fetches the recently played tracks for a user
func (p *Platforms) FetchTrackListeningHistory(ctx *fiber.Ctx) error {
	log.Println("[platforms][FetchListeningHistory] info - Fetching listening history")
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
		history, err := spotify.FetchTrackListeningHistory(userCtx.RefreshToken)
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
