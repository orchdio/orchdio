package platforms

import (
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"log"
	"orchdio/blueprint"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/util"
	"os"
)

// FetchPlatformAlbums fetches the user's library albums from the specified platform
func (p *Platforms) FetchPlatformAlbums(ctx *fiber.Ctx) error {
	log.Println("[platforms][FetchPlatformAlbums] info - Fetching platform albums")
	userCtx := ctx.Locals("userCtx").(*blueprint.AuthMiddlewareUserInfo)
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	platform := userCtx.Platform

	switch platform {
	case applemusic.IDENTIFIER:
		// TODO: implement fetching user library albums from apple music api
		albums, err := applemusic.FetchLibraryAlbums(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Apple Music")
		}

		var resp = blueprint.UserLibraryAlbums{
			Payload: albums,
			Total:   len(albums),
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, resp)

	case deezer.IDENTIFIER:
		log.Printf("[platforms][FetchPlatformAlbums] info - Fetching user library albums from Deezer")
		albums, err := deezer.FetchLibraryAlbums(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Deezer")
		}

		// todo: make sure that the length of the albums is indeed the total number of albums in the user library
		var resp = blueprint.UserLibraryAlbums{
			Payload: albums,
			Total:   len(albums),
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, resp)

	case spotify.IDENTIFIER:
		log.Printf("[platforms][FetchPlatformAlbums] info - Fetching user library albums from Spotify")
		credBytes, err := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - could not decrypt app's spotify credentials while fetching user library albums%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to decrypt Spotify credentials")
		}

		var credentials blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - could not unmarshal app's spotify credentials while fetching user library albums%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to decrypt Spotify credentials")
		}
		spotifyService := spotify.NewService(credentials.AppID, credentials.AppSecret, p.Redis)
		albums, err := spotifyService.FetchLibraryAlbums(userCtx.RefreshToken)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Spotify")
		}

		var resp = blueprint.UserLibraryAlbums{
			Payload: albums,
			Total:   len(albums),
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, resp)

	case tidal.IDENTIFIER:
		log.Printf("[platforms][FetchPlatformAlbums] info - Fetching user library albums from Tidal")
		log.Printf("[platforms][FetchPlatformAlbums] info - Platform ID: %s", userCtx.PlatformID)
		albums, err := tidal.FetchLibraryAlbums(userCtx.PlatformID)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - %s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to fetch albums from Tidal")
		}
		var resp = blueprint.UserLibraryAlbums{
			Payload: albums,
			Total:   len(albums),
		}
		return util.SuccessResponse(ctx, fiber.StatusOK, resp)
	}
	return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", fmt.Sprintf("Platform %s is not supported", platform))
}
