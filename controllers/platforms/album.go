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
	appCtx := ctx.Locals("appCtx").(*blueprint.AuthMiddlewareUserInfo)
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	platform := appCtx.Platform

	switch platform {
	case applemusic.IDENTIFIER:
		decryptedCredentials, err := util.DecryptIntegrationCredentials(app.AppleMusicCredentials)
		if err != nil {
			if err == blueprint.ENOCREDENTIALS {
				log.Printf("[platforms][FetchPlatformAlbums] error - Apple Music credentials are nil")
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "authorization error", "The developer has not provided Apple Music credentials for this app and cannot access this resource.")
			}
		}
		// TODO: implement fetching user library albums from apple music api
		// remember that for applemjsic, app refresh token field is equal to the apple api key. its encrypted
		// under the refreshtoken field for conformity with the other platforms.
		albums, err := applemusic.FetchLibraryAlbums(decryptedCredentials.AppRefreshToken, appCtx.RefreshToken)
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
		var deezerCredentials blueprint.IntegrationCredentials
		if app.DeezerCredentials == nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - Deezer credentials are nil")
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "authorization error", "The developer has not provided deezer credentials for this app and cannot access this resource.")
		}
		credBytes, err := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - could not decrypt app's deezer credentials while fetching user library albums%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to decrypt Deezer credentials")
		}
		err = json.Unmarshal(credBytes, &deezerCredentials)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - could not unmarshal app's deezer credentials while fetching user library albums%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to decrypt Deezer credentials")
		}

		deezerService := deezer.NewService(&deezerCredentials, p.DB, p.Redis)
		albums, err := deezerService.FetchLibraryAlbums(appCtx.RefreshToken)
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
		spotifyService := spotify.NewService(&credentials, p.DB, p.Redis)
		albums, err := spotifyService.FetchLibraryAlbums(appCtx.RefreshToken)
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
		log.Printf("[platforms][FetchPlatformAlbums] info - Platform ID: %s", appCtx.PlatformID)
		var tidalCredentials blueprint.IntegrationCredentials
		if app.TidalCredentials == nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - Tidal credentials are nil")
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "authorization error", "The developer has not provided tidal credentials for this app and cannot access this resource.")
		}

		credBytes, err := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - could not decrypt app's tidal credentials while fetching user library albums%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to decrypt Tidal credentials")
		}
		err = json.Unmarshal(credBytes, &tidalCredentials)
		if err != nil {
			log.Printf("[platforms][FetchPlatformAlbums] error - could not unmarshal app's tidal credentials while fetching user library albums%s", err.Error())
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Failed to decrypt Tidal credentials")
		}
		tidalService := tidal.NewService(&tidalCredentials, p.DB, p.Redis)
		albums, err := tidalService.FetchLibraryAlbums(appCtx.PlatformID)
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
