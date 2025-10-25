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
	//user, err := database.FindUserByUUID(userId, platform)
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), platform)
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

	history, err := universal.FetchLibraryArtists(platform, refreshToken, app.UID.String(), p.DB, p.Redis)

	if err != nil {
		log.Printf("\n[controllers][platforms][%s][FetchLibraryArtists] error - %v\n", platform, err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", fmt.Sprintf("Could not fetch user library artists on platform %s", platform))
	}

	return util.SuccessResponse(ctx, http.StatusOK, history)
}
