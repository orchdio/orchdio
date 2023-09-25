package middleware

import (
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"log"
	"net/http"
	"orchdio/blueprint"
	logger2 "orchdio/logger"
	"orchdio/services"
	"orchdio/util"
	"strings"
)

func VerifyAppJWT(ctx *fiber.Ctx) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", ctx.Get("x-orchdio-app-pub-key")).String,
		Platform:             zap.String("platform", ctx.Get("x-orchdio-platform")).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)
	orchdioLogger.Info("[middleware][VerifyAppJWT] method - Verifying app JWT...")
	jt := ctx.Locals("appToken")
	if jt == nil {
		orchdioLogger.Error("[middleware][VerifyAppJWT] method - JWT header missing")
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "JWT header is missing")
	}
	jwtToken := jt.(*jwt.Token)

	claims := jwtToken.Claims.(*blueprint.AppJWT)
	ctx.Locals("app_jwt", claims)
	orchdioLogger.Info("[middleware][VerifyAppJWT] method - MusicToken verified. Claims set")
	return ctx.Next()
}

func ExtractLinkInfoFromBody(ctx *fiber.Ctx) error {
	// adding all in order to support wildcard. when the option is empty, we can presume they want to convert
	// to all platforms (that they have added their credentials for and the user has authed, that is)
	platforms := []string{"ytmusic", "spotify", "deezer", "applemusic", "tidal", "all"}
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", ctx.Get("x-orchdio-app-pub-key")).String,
		Platform:             zap.String("platform", ctx.Get("x-orchdio-platform")).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)

	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	linkBody := ctx.Body()
	// todo move this to a real type in blueprint. the keys are url and target_platform
	conversionBody := map[string]string{}

	err := json.Unmarshal(linkBody, &conversionBody)
	if err != nil {
		orchdioLogger.Error("[middleware][ExtractLinkInfoFromBody] error - Could not unmarshal conversionBody body: %v\n", zap.Error(err))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}
	url := conversionBody["url"]

	if url == "" {
		orchdioLogger.Warn("[middleware][ExtractLinkInfoFromBody] warning - URL not detected. Skipping...")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Check you're using the '?conversionBody' query string")
	}
	linkInfo, err := services.ExtractLinkInfo(url)
	linkInfo.App = app.UID.String()
	linkInfo.Developer = app.Developer.String()

	if err != nil {
		if errors.Is(err, blueprint.EHOSTUNSUPPORTED) {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented.")
		}

		if errors.Is(err, blueprint.EINVALIDLINK) {
			orchdioLogger.Warn("[middleware][ExtractLinkInfoFromBody] warning - Invalid conversionBody. Might not be a valid body.", zap.Any("body", conversionBody))
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request body. Please make sure you pass a valid conversionBody")
		}

		orchdioLogger.Error("[middleware][ExtractLinkInfoFromBody] error - Could not extract conversionBody info.", zap.Error(err), zap.Any("body", conversionBody))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}

	if linkInfo == nil {
		orchdioLogger.Warn("[middleware][ExtractLinkInfoFromBody] warning - No linkInfo retrieved for conversionBody.", zap.Any("body", conversionBody))
		return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "URL info not found.")
	}

	if conversionBody["target_platform"] == "" || conversionBody["target_platform"] == "all" {
		orchdioLogger.Warn("[middleware][ExtractLinkInfoFromBody] warning - Track conversion but no target platform specified. Setting to all.", zap.Any("body", conversionBody))
		conversionBody["target_platform"] = "all"
	}

	if !lo.Contains(platforms, conversionBody["target_platform"]) {
		orchdioLogger.Warn("[middleware][ExtractLinkInfoFromBody] warning - Track conversion but no target platform specified.", zap.Any("body", conversionBody))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request body. Please make sure you pass a valid target platform")
	}
	linkInfo.TargetPlatform = conversionBody["target_platform"]
	// set ctx local called "linkInfo" to the linkInfo type. this is for a track conversion.
	// it looks like: {TargetLink: "https://music.youtube.com/watch?v=Z2X4uZL2o8Q", TargetPlatform: "spotify"}
	ctx.Locals("linkInfo", linkInfo)

	if strings.Contains(linkInfo.TargetLink, "playlist") {
		orchdioLogger.Info("[middleware][ExtractLinkInfoFromBody] method - Playlist detected. Checking for target platform")
		// if the target platform is not set, we'll exit here. keep in mind in case of testing and it doesnt work as before.
		if conversionBody["target_platform"] == "" {
			orchdioLogger.Warn("[middleware][ExtractLinkInfoFromBody] warning - Target platform not detected. Skipping...")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "You are trying to convert a playlist. Please specify a target platform.")
		}

		log.Printf("\n[middleware][ExtractLinkInfoFromBody] method - converting from '%s' to '%s'\n", strings.ToUpper(linkInfo.Platform), strings.ToUpper(conversionBody["target_platform"]))
		// if the target platform is set, we'll check if it's valid. if it's not, we'll exit here.
		playlistPlatforms := []string{"spotify", "deezer", "applemusic", "tidal"}
		if !lo.Contains(playlistPlatforms, conversionBody["target_platform"]) {
			orchdioLogger.Warn("[middleware][ExtractLinkInfoFromBody] warning - track platform is invalid. please pass a valid platform value.", zap.Any("body", conversionBody))
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
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", ctx.Get("x-orchdio-app-pub-key")).String,
		Platform:             zap.String("platform", ctx.Get("x-orchdio-platform")).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)

	if link == "" {
		orchdioLogger.Warn("[middleware][ExtractLinkInfo] warning - URL not detected. Skipping...")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Bad request. Check you're using the '?link' query string")
	}
	linkInfo, err := services.ExtractLinkInfo(link)
	if err != nil {
		if errors.Is(err, blueprint.EHOSTUNSUPPORTED) {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
		}

		if errors.Is(err, blueprint.EINVALIDLINK) {
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body. The link is invalid")
		}

		orchdioLogger.Error("[middleware][ExtractLinkInfo] error - Could not extract link info.", zap.Error(err), zap.String("link", link))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}

	if linkInfo == nil {
		orchdioLogger.Warn("[middleware][ExtractLinkInfo] warning - No linkInfo retrieved for link.", zap.String("link", link))
		return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "URL info not found.")
	}

	orchdioLogger.Info("[middleware][ExtractLinkInfo] method - Extracted link info is: %v\n", zap.Any("linkInfo", linkInfo))
	ctx.Locals("linkInfo", linkInfo)
	return ctx.Next()
}
