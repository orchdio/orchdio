package platforms

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"log"
	"orchdio/blueprint"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/util"
)

// FetchPlatformAlbums fetches the user's library albums from the specified platform
func (p *Platforms) FetchPlatformAlbums(ctx *fiber.Ctx) error {
	log.Println("[platforms][FetchPlatformAlbums] info - Fetching platform albums")
	userCtx := ctx.Locals("userCtx").(*blueprint.AuthMiddlewareUserInfo)
	platform := userCtx.Platform

	switch platform {
	case applemusic.IDENTIFIER:
		// TODO: implement fetching user library albums from apple music api
		albums, err := applemusic.FetchLibraryAlbums(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Apple Music")
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, albums)

	case deezer.IDENTIFIER:
		log.Printf("[platforms][FetchPlatformAlbums] info - Fetching user library albums from Deezer")
		albums, err := deezer.FetchLibraryAlbums(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Deezer")
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, albums)

	case spotify.IDENTIFIER:
		log.Printf("[platforms][FetchPlatformAlbums] info - Fetching user library albums from Spotify")
		albums, err := spotify.FetchLibraryAlbums(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Spotify")
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, albums)

	case tidal.IDENTIFIER:
		log.Printf("[platforms][FetchPlatformAlbums] info - Fetching user library albums from Tidal")
		log.Printf("[platforms][FetchPlatformAlbums] info - Platform ID: %s", userCtx.PlatformIDs["tidal"])
		albums, err := tidal.FetchLibraryAlbums(userCtx.PlatformIDs["tidal"])
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Tidal")
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, albums)
	}

	return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", fmt.Sprintf("Platform %s is not supported", platform))
}
