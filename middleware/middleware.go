package middleware

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/samber/lo"
)

// VerifyToken verifies a token and set the context local called "claim" to a type of *blueprint.OrchdioUserToken
func VerifyToken(ctx *fiber.Ctx) error {
	jt := ctx.Locals("authToken")
	if jt == nil {
		log.Printf("[middlware][VerifyToken][error] method - JWT header missing")
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "JWT header is missing")
	}
	jwtToken := jt.(*jwt.Token)
	claims := jwtToken.Claims.(*blueprint.OrchdioUserToken)
	ctx.Locals("claims", claims)
	log.Printf("[middleware][VerifyToken] method - MusicToken verified. Claims set")
	return ctx.Next()
}

func VerifyAppJWT(ctx *fiber.Ctx) error {
	jt := ctx.Locals("appToken")
	if jt == nil {
		log.Printf("[middlware][VerifyAppJWT] method - JWT header missing")
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "JWT header is missing")
	}
	jwtToken := jt.(*jwt.Token)
	claims := jwtToken.Claims.(*blueprint.AppJWT)
	ctx.Locals("app_jwt", claims)
	log.Printf("[middleware][VerifyAppJWT] method - MusicToken verified. Claims set")
	return ctx.Next()
}

func ExtractLinkInfoFromBody(ctx *fiber.Ctx) error {
	// adding all in order to support wildcard. when the option is empty, we can presume they want to convert
	// to all platforms (that they have added their credentials for and the user has authed, that is)
	platforms := []string{ytmusic.IDENTIFIER, spotify.IDENTIFIER, deezer.IDENTIFIER, applemusic.IDENTIFIER, tidal.IDENTIFIER, "all"}
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	linkBody := ctx.Body()

	conversionBody := blueprint.ConversionBody{}

	err := json.Unmarshal(linkBody, &conversionBody)
	if err != nil {
		log.Printf("[middleware][ExtractLinkInfoFromBody] error - Could not unmarshal conversionBody body: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}

	if conversionBody.URL == "" {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - URL not detected. Skipping...\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Check you're using the '?conversionBody' query string")
	}
	linkInfo, err := services.ExtractLinkInfo(conversionBody.URL)
	linkInfo.App = app.UID.String()
	linkInfo.Developer = app.Developer.String()

	if err != nil {
		if errors.Is(err, blueprint.ErrHostUnsupported) {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented.")
		}

		if errors.Is(err, blueprint.ErrInvalidLink) {
			log.Printf("[middleware][ExtractLinkInfoFromBody][warning] invalid conversionBody. are you sure its a url? %s\n", conversionBody)
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request body. Please make sure you pass a valid conversionBody")
		}

		log.Printf("\n[middleware][ExtractLinkInfoFromBody] error - Could not extract conversionBody info: %v: for conversionBody: %v\n", err, conversionBody)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}

	if linkInfo == nil {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] error - No linkInfo retrieved for conversionBody: %v: \n", conversionBody)
		return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "URL info not found.")
	}

	// fixme: is this really needed?
	if conversionBody.TargetPlatform == "" || conversionBody.TargetPlatform == "all" {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Track conversion but no target platform specified. \n")
		conversionBody.TargetPlatform = "all"
	}

	if !lo.Contains(platforms, conversionBody.TargetPlatform) {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - track platform is invalid. please pass a valid platform value. \n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request body. Please make sure you pass a valid target platform")
	}
	linkInfo.TargetPlatform = conversionBody.TargetPlatform
	// set ctx local called "linkInfo" to the linkInfo type. this is for a track conversion.
	// it looks like: {TargetLink: "https://music.youtube.com/watch?v=Z2X4uZL2o8Q", TargetPlatform: "spotify"}
	ctx.Locals("linkInfo", linkInfo)

	// prevent entity conversion if the source platform is not supported
	switch linkInfo.Platform {
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Deezer credentials not found. Exiting entity conversion\n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Deezer credentials not found")
		}

	case spotify.IDENTIFIER:
		if len(app.SpotifyCredentials) == 0 {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Spotify credentials not found. Exiting entity conversion\n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Spotify credentials not found")
		}

	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Apple Music credentials not found. Exiting entity conversion\n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Apple Music credentials not found")
		}
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Tidal credentials not found. Exiting entity conversion\n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Tidal credentials not found")
		}
	}

	if strings.Contains(linkInfo.TargetLink, "playlist") {
		// if the target platform is not set, we'll exit here. keep in mind in case of testing and it doesnt work as before.
		if conversionBody.TargetPlatform == "" {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Target platform not detected. Skipping...\n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "You are trying to convert a playlist. Please specify a target platform.")
		}

		// if the target platform is set, we'll check if it's valid. if it's not, we'll exit here.
		playlistPlatforms := []string{"spotify", "deezer", "applemusic", "tidal"}
		if !lo.Contains(playlistPlatforms, conversionBody.TargetPlatform) {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - track platform is invalid. please pass a valid platform value. \n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request body. Please make sure you pass a valid target platform")
		}
		linkInfo.TargetPlatform = conversionBody.TargetPlatform
		// set ctx local called "linkInfo" to the linkInfo type. this is for a playlist conversion.
		// it looks like: {TargetLink: "https://music.youtube.com/playlist?list=OLAK5uy_m8ZQZ4Z1Z2X4uZL2o8Q", TargetPlatform: "spotify", Entity: "playlist"}
		ctx.Locals("linkInfo", linkInfo)
	}

	return ctx.Next()
}

// ExtractLinkInfo fetches the extracted info about a link and save it into local context called "linkInfo"
func ExtractLinkInfo(ctx *fiber.Ctx) error {
	link := ctx.Query("link")
	if link == "" {
		log.Printf("\n[middleware][ExtractLinkInfo] warning - URL not detected. Skipping...\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Check you're using the '?link' query string")
	}
	linkInfo, err := services.ExtractLinkInfo(link)
	if err != nil {
		if err == blueprint.ErrHostUnsupported {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
		}

		if err == blueprint.ErrInvalidLink {
			log.Printf("[middleware][ExtractLinkInfo][warning] invalid link. are you sure its a url? %s\n", link)
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body. The link is invalid")
		}

		log.Printf("\n[middleware][ExtractLinkInfo] error - Could not extract link info: %v: for link: %v\n", err, link)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}

	if linkInfo == nil {
		log.Printf("\n[middleware][ExtractLinkInfo] error - No linkInfo retrieved for link: %v: \n", link)
		return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "URL info not found.")
	}

	ctx.Locals("linkInfo", linkInfo)
	return ctx.Next()
}
