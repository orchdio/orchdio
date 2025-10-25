package middleware

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	logger2 "orchdio/logger"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/ytmusic"
	"orchdio/util"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type AuthMiddleware struct {
	DB *sqlx.DB
}

func NewAuthMiddleware(db *sqlx.DB) *AuthMiddleware {
	return &AuthMiddleware{DB: db}
}

// ValidateKey validates that the key is valid
func (a *AuthMiddleware) ValidateKey(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")

	if len([]byte(apiKey)) > 36 {
		log.Printf("[middleware][ValidateKey] key is too long. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "Key too long")
	}

	isValid := util.IsValidUUID(apiKey)

	if !isValid {
		log.Printf("[controller][user][Revoke] invalid key. Bad request %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "Invalid apikey")
	}

	// fetch the user from the database
	database := db.NewDB{DB: a.DB}

	user, err := database.FetchUserWithApiKey(apiKey)
	if err != nil {

		if err == sql.ErrNoRows {
			log.Printf("[middleware][ValidateKey] key not found. %s\n", apiKey)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "Invalid apikey")
		}

		log.Printf("[middleware][ValidateKey] error - Could not fetch user with api key: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An internal error occurred")
	}
	ctx.Locals("user", user)
	log.Printf("[middleware][ValidateKey] API key is valid")
	return ctx.Next()
}

func (a *AuthMiddleware) LogIncomingRequest(ctx *fiber.Ctx) error {
	// in order to suppress the health monitor from logging the request, we check if the path is /health
	if ctx.Path() == "/vermont/info" {
		return ctx.Next()
	}
	log.Printf("[middleware][LogIncomingRequest] incoming request: %s  %s: %s\n", ctx.IP(), ctx.Method(), ctx.Path())
	return ctx.Next()
}

// AddReadOnlyDeveloperToContext gets the developer using the public key which is read only and attach the developer to context.
func (a *AuthMiddleware) AddReadOnlyDeveloperToContext(ctx *fiber.Ctx) error {
	log.Printf("[db][middleware][AddReadOnlyDevAccessToContext] developer -  fetching app developer with public key\n")
	pubKey := ctx.Get("x-orchdio-public-key")
	if pubKey == "" {
		log.Printf("[db][AddReadOnlyDevAccessToContext] developer -  error: could not fetch app developer with public key. No header passed")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "missing x-orchdio-public-key header")
	}

	// check if the key is valid
	isValid := util.IsValidUUID(pubKey)
	if !isValid {
		log.Printf("[db][AddReadOnlyDevAccessToContext] developer -  error: could not fetch app developer with public key. Header passed is %s\n", pubKey)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid x-orchdio-public-key header")
	}

	// fetch the developer from the database
	database := db.NewDB{DB: a.DB}
	developer, err := database.FetchDeveloperAppWithPublicKey(pubKey)
	if err != nil {
		log.Printf("[db][AddReadOnlyDevAccessToContext] developer -  error: could not fetch app developer with public key")
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "invalid x-orchdio-public-key header. App does not exist")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}
	log.Printf("[db][AddReadOnlyDevAccessToContext] developer - making read only request")
	ctx.Locals("developer", developer)

	// fetch the app with the public key
	app, err := database.FetchAppByPublicKey(pubKey, developer.UUID.String())
	if err != nil {
		log.Printf("[db][AddReadOnlyDevAccessToContext] developer -  error: could not fetch app with public key")
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}
	// set the app to the context
	ctx.Locals("app", app)
	return ctx.Next()
}

// AddReadWriteDeveloperToContext gets the developer using the secret key which is read and write and attach the developer to context.
func (a *AuthMiddleware) AddReadWriteDeveloperToContext(ctx *fiber.Ctx) error {
	log.Printf("[db][middleware][FetchAppDeveloperWithSecretKey] developer -  fetching app developer with secret key\n")
	key := ctx.Get("x-orchdio-key")
	if key == "" {
		log.Printf("[db][FetchAppDeveloperWithSecretKey] developer -  error: could not fetch app developer with secret")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "missing x-orchdio-key header")
	}

	// check if the key is valid
	isValid := util.IsValidUUID(key)
	if !isValid {
		log.Printf("[db][FetchAppDeveloperWithSecretKey] developer -  error: could not fetch app developer with secret")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid x-orchdio-key header")
	}

	var developer blueprint.User
	err := a.DB.QueryRowx(queries.FetchAuthorizedAppDeveloperBySecretKey, key).StructScan(&developer)
	if err != nil {
		log.Printf("[db][FetchAppDeveloperWithSecretKey] developer -  error: could not fetch app developer with the secret: %s. Error is %v\n", key, err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "app not found")
		}
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid x-orchdio-key header")
	}
	ctx.Locals("developer", &developer)

	database := db.NewDB{DB: a.DB}
	app, err := database.FetchAppBySecretKey([]byte(key))
	if err != nil {
		log.Printf("[db][AddReadWriteDevAccessToContext] developer -  error: could not fetch app with private key")
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "invalid x-orchdio-key header. App does not exist")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}
	// set the app to the context
	ctx.Locals("app", app)
	return ctx.Next()
}

func (a *AuthMiddleware) HandleTrolls(ctx *fiber.Ctx) error {
	var blacklists = []string{"/.env.dev", "/_profiler/phpinfo",
		"/.admin",
		"/.git",
		"/nginx_status",
		"/.htcaccess", "/robot.txt", "/admin.php"}
	for _, blacklist := range blacklists {
		if strings.Contains(ctx.Path(), blacklist) {
			log.Printf("[middleware][HandleTrolls] warning - Trolling attempt from IP: %s at path: %s at time: %s\n", ctx.IP(), ctx.Path(), time.Now().String())
			return util.ErrorResponse(ctx, http.StatusExpectationFailed, "zilch", "lol 🖕🏾")
		}
	}
	return ctx.Next()
}

// VerifyUserActionApp verifies that the developer app is valid and that the user is authorized to perform the action. It attaches the user
// context information to the locals. this is then used in controllers where user is making user action requests, for example fetching user library playlists.
func (a *AuthMiddleware) VerifyUserActionApp(ctx *fiber.Ctx) error {
	log.Printf("[middleware][auth][AuthDeveloperApp] - authenticating developer app")
	// extract the app id from the header
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	userId := ctx.Params("userId")
	platform := ctx.Params("platform")
	refreshToken := ""
	if userId == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Missing user id")
	}

	if platform == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Missing platform")
	}

	if app == nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Missing app")
	}

	database := db.NewDB{DB: a.DB}
	user, err := database.FetchPlatformAndUserInfoByIdentifier(userId, app.UID.String(), platform)
	if err != nil {
		if err == sql.ErrNoRows {
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", fmt.Sprintf("App not found. User might not have authorized this app. Please sign in with %s", FetcPlatformNameByIdentifier(platform)))
		}
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	if user.RefreshToken == nil && platform != "tidal" {
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "User not authorized")
	}

	if user.RefreshToken != nil {
		r, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[middleware][auth][AuthDeveloperApp] - could not decrypt refresh token")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		refreshToken = string(r)
	}

	userMiddlewareInfo := blueprint.AuthMiddlewareUserInfo{
		Platform:     platform,
		PlatformID:   user.PlatformID,
		RefreshToken: refreshToken,
	}

	ctx.Locals("userCtx", &userMiddlewareInfo)
	return ctx.Next()
}

func FetcPlatformNameByIdentifier(identifier string) string {
	var platforms = map[string]string{
		spotify.IDENTIFIER:    "Spotify",
		deezer.IDENTIFIER:     "Deezer",
		applemusic.IDENTIFIER: "Apple Music",
		ytmusic.IDENTIFIER:    "YouTube Music",
	}
	return platforms[identifier]
}

func (a *AuthMiddleware) AddRequestPlatformWithPrivateKeyToCtx(ctx *fiber.Ctx) error {
	platform := ctx.Params("platform")
	reqId := ctx.Get("x-orchdio-request-id")
	// todo: rename this to x-orchdio-private-key
	privKey := ctx.Get("x-orchdio-key")
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID: reqId,
		Platform:  zap.String("platform", platform).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)
	orchdioLogger.Info("Request ID", zap.String("request_id", reqId))
	if platform == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Missing platform")
	}

	platforms := []string{"spotify", "deezer", "tidal", "applemusic"}
	if !lo.Contains(platforms, platform) {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid platform")
	}

	path := ctx.Path()
	orchdioLogger.Info("Request path", zap.String("path", path))

	if privKey == "" {
		orchdioLogger.Error("[middleware][AddRequestPlatformToCtx] developer -  error: could not fetch app developer with public key. No public key passed")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "missing x-orchdio-public-key header")
	}
	ctx.Locals("app_private_key", privKey)
	ctx.Locals("platform", platform)
	return ctx.Next()
}

func (a *AuthMiddleware) AddRequestPlatformWithPubKeyToCtx(ctx *fiber.Ctx) error {
	platform := ctx.Params("platform")
	reqId := ctx.Get("x-orchdio-request-id")
	pubKey := ctx.Get("x-orchdio-public-key")
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            reqId,
		ApplicationPublicKey: zap.String("app_public_key", pubKey).String,
		Platform:             zap.String("platform", platform).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)
	orchdioLogger.Info("Request ID", zap.String("request_id", reqId))
	if platform == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Missing platform")
	}

	platforms := []string{"spotify", "deezer", "tidal", "applemusic"}
	if !lo.Contains(platforms, platform) {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid platform")
	}

	appPubKey := ctx.Get("x-orchdio-public-key")
	path := ctx.Path()
	orchdioLogger.Info("Request path", zap.String("path", path))

	// due to the fact that during auth, deezer doesn't make the request with the pubkey
	// we make sure to skip for auth paths generally
	if appPubKey == "" && !strings.Contains(path, "/callback") {
		orchdioLogger.Error("[middleware][AddRequestPlatformToCtx] developer -  error: could not fetch app developer with public key. No public key passed")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "missing x-orchdio-public-key header")
	}

	// rename to pub key
	ctx.Locals("app_pub_key", appPubKey)
	ctx.Locals("platform", platform)
	return ctx.Next()
}

func (a *AuthMiddleware) CheckOrgID(ctx *fiber.Ctx) error {
	orgID := ctx.Params("orgId")
	if orgID == "" {
		log.Printf("[middleware][CheckOrgID] developer -  error: orgId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Org ID is empty. Please pass a valid org ID.")
	}
	return ctx.Next()
}
