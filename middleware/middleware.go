package middleware

import (
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/samber/lo"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services"
	"orchdio/util"
	"strings"
)

// VerifyToken verifies a token and set the context local called "claim" to a type of *blueprint.OrchdioUserToken
func VerifyToken(ctx *fiber.Ctx) error {
	log.Printf("[middleware][VerifyToken] method - Verifying token...\n")
	jt := ctx.Locals("authToken")
	if jt == nil {
		log.Printf("[middlware][VerifyToken] method - JWT header missing")
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "JWT header is missing")
	}
	jwtToken := jt.(*jwt.Token)
	claims := jwtToken.Claims.(*blueprint.OrchdioUserToken)
	ctx.Locals("claims", claims)
	log.Printf("[middleware][VerifyToken] method - Token verified. Claims set")
	return ctx.Next()
}

func VerifyAppJWT(ctx *fiber.Ctx) error {
	log.Printf("[middleware][VerifyAppJWT] method - Verifying app JWT...\n")
	jt := ctx.Locals("appToken")
	if jt == nil {
		log.Printf("[middlware][VerifyAppJWT] method - JWT header missing")
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "JWT header is missing")
	}
	jwtToken := jt.(*jwt.Token)
	claims := jwtToken.Claims.(*blueprint.AppJWT)
	ctx.Locals("app_jwt", claims)
	log.Printf("[middleware][VerifyAppJWT] method - Token verified. Claims set")
	return ctx.Next()
}

func ExtractLinkInfoFromBody(ctx *fiber.Ctx) error {
	// adding all in order to support wildcard. when the option is empty, we can presume they want to convert
	// to all platforms (that they have added their credentials for and the user has authed, that is)
	platforms := []string{"ytmusic", "spotify", "deezer", "applemusic", "tidal", "all"}
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	linkBody := ctx.Body()
	// todo move this to a real type in blueprint. the keys are url and target_platform
	conversionBody := map[string]string{}

	err := json.Unmarshal(linkBody, &conversionBody)
	if err != nil {
		log.Printf("[middleware][ExtractLinkInfoFromBody] error - Could not unmarshal conversionBody body: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}
	url := conversionBody["url"]

	if url == "" {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - URL not detected. Skipping...\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Check you're using the '?conversionBody' query string")
	}
	linkInfo, err := services.ExtractLinkInfo(url)
	linkInfo.App = app.UID.String()
	linkInfo.Developer = app.Developer.String()

	if err != nil {
		if err == blueprint.EHOSTUNSUPPORTED {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented.")
		}

		if err == blueprint.EINVALIDLINK {
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

	if conversionBody["target_platform"] == "" {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Track conversion but no target platform specified. \n")
		conversionBody["target_platform"] = "all"
	}

	if !lo.Contains(platforms, conversionBody["target_platform"]) {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - track platform is invalid. please pass a valid platform value. \n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request body. Please make sure you pass a valid target platform")
	}
	linkInfo.TargetPlatform = conversionBody["target_platform"]
	// set ctx local called "linkInfo" to the linkInfo type. this is for a track conversion.
	// it looks like: {TargetLink: "https://music.youtube.com/watch?v=Z2X4uZL2o8Q", TargetPlatform: "spotify"}
	ctx.Locals("linkInfo", linkInfo)

	if strings.Contains(linkInfo.TargetLink, "playlist") {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] method - Playlist detected. Checking for target platform\n")
		// if the target platform is not set, we'll exit here. keep in mind in case of testing and it doesnt work as before.
		if conversionBody["target_platform"] == "" {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Target platform not detected. Skipping...\n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "You are trying to convert a playlist. Please specify a target platform.")
		}

		log.Printf("\n[middleware][ExtractLinkInfoFromBody] method - converting from '%s' to '%s'\n", strings.ToUpper(linkInfo.Platform), strings.ToUpper(conversionBody["target_platform"]))
		// if the target platform is set, we'll check if it's valid. if it's not, we'll exit here.
		playlistPlatforms := []string{"spotify", "deezer", "applemusic", "tidal"}
		if !lo.Contains(playlistPlatforms, conversionBody["target_platform"]) {
			log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - track platform is invalid. please pass a valid platform value. \n")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request body. Please make sure you pass a valid target platform")
		}
		linkInfo.TargetPlatform = conversionBody["target_platform"]
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
		if err == blueprint.EHOSTUNSUPPORTED {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
		}

		if err == blueprint.EINVALIDLINK {
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

	log.Printf("\n[middleware][ExtractLinkInfo] method - Extracted link info is: %v\n", linkInfo)
	ctx.Locals("linkInfo", linkInfo)
	return ctx.Next()
}
