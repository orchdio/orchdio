package platforms

import (
	"github.com/gofiber/fiber/v2"
	"log"
	"orchdio/blueprint"
	"orchdio/services/applemusic"
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
	}

	return ctx.Status(fiber.StatusNotFound).JSON("Platform not found")
}
