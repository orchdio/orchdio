package developer

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"log"
	"orchdio/blueprint"
	webhook "orchdio/convoy.go"
	"orchdio/db"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	"os"
)

type Controller struct {
	DB     *sqlx.DB
	Logger *zap.Logger
}

func NewDeveloperController(db *sqlx.DB, logger *zap.Logger) *Controller {
	return &Controller{DB: db, Logger: logger}
}

// CreateApp creates a new app for the developer. An app is a way to access the API, there can be multiple apps per developer.
func (d *Controller) CreateApp(ctx *fiber.Ctx) error {

	d.Logger.Info("[controllers][CreateApp] developer -  creating new app")
	platforms := []string{applemusic.IDENTIFIER, deezer.IDENTIFIER, spotify.IDENTIFIER, tidal.IDENTIFIER, ytmusic.IDENTIFIER}

	// get the claims from the context
	// FIXME: perhaps use reflection to check type and gracefully handle
	//claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)
	// deserialize the request body
	var body blueprint.CreateNewDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		d.Logger.Error("[controllers][CreateApp] developer -  error: could not deserialize request body", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Could not deserialize request body")
	}

	orgId := ctx.Params("orgId")
	if orgId == "" {
		d.Logger.Error("[controllers][CreateApp] developer -  error: organization id is empty", zap.Any("body", body))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Organization ID is empty. Please pass a valid organization id")
	}

	pubKey := uuid.NewString()
	secretKey := uuid.NewString()
	verifySecret := uuid.NewString()
	deezerState := string(util.GenerateShortID())

	if body.Organization == "" {
		d.Logger.Error("[controllers][CreateApp] developer -  error: organization is empty", zap.Any("body", body))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Organization is empty. Please pass a valid organization")
	}

	if body.IntegrationAppId == "" {
		d.Logger.Error("[controllers][CreateApp] developer -  error: app id is empty", zap.Any("body", body))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Integration App ID is empty. Please pass a valid app id")
	}

	if body.IntegrationAppSecret == "" {
		d.Logger.Error("[controllers][CreateApp] developer -  error: app secret is empty", zap.Any("body", body))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Integration App Secret is empty. Please pass a valid app secret")
	}

	if body.IntegrationPlatform == "" {
		d.Logger.Error("[controllers][CreateApp] developer -  error: platform is empty", zap.Any("body", body))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is empty. Please pass a valid platform")
	}

	if body.WebhookURL == "" {
		d.Logger.Error("[controllers][CreateApp] developer -  error: webhook url is empty", zap.Any("body", body))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Webhook URL is empty. Please pass a valid webhook url")
	}

	if !util.IsValidUUID(body.Organization) {
		d.Logger.Error("[controllers][CreateApp] developer -  error: organization is not a valid UUID", zap.Any("body", body))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Organization is not a valid UUID. Please pass a valid organization")
	}

	if !lo.Contains(platforms, body.IntegrationPlatform) {
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is not valid. Please pass a valid platform")
	}
	// TODO: check if the app id and app secret are valid for the platform

	// create a json object to store the app data. for now, we assume all apps have app id and app secret
	appData := blueprint.IntegrationCredentials{
		AppID:     body.IntegrationAppId,
		AppSecret: body.IntegrationAppSecret,
		Platform:  body.IntegrationPlatform,
	}

	ser, err := json.Marshal(appData)
	if err != nil {
		d.Logger.Error("[controllers][CreateApp] developer -  error: could not marshal app data", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	//encrypt the app data
	encryptedAppData, err := util.Encrypt(ser, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		d.Logger.Error("[controllers][CreateApp] developer -  error: could not encrypt app data", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	// create new developer app
	database := db.NewDB{DB: d.DB}
	uid, err := database.CreateNewApp(body.Name, body.Description, body.RedirectURL, body.WebhookURL, pubKey, claims.DeveloperID, secretKey, verifySecret, body.Organization, deezerState)
	if err != nil {
		d.Logger.Error("[controllers][CreateApp] developer -  error: could not create new developer app", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}
	// create a new convoy webhook endpoint
	convoyInst := webhook.NewConvoy()

	defer func() {
		log.Printf("Some ordinary defer")
	}()
	webhookName := fmt.Sprintf("%s-%s", body.Name, uid)
	whResponse, err := convoyInst.CreateEndpoint(body.WebhookURL, body.Description, webhookName)
	if err != nil {
		d.Logger.Error("[controllers][CreateApp] developer -  error: could not create webhook endpoint", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	// update the app credentials
	err = database.UpdateIntegrationCredentials(encryptedAppData, string(uid), body.IntegrationPlatform, body.RedirectURL, body.WebhookURL, whResponse.ID)
	if err != nil {
		d.Logger.Error("[controllers][CreateApp] developer -  error: could not update app credentials", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}
	d.Logger.Info("[controllers][CreateApp] developer -  new app created", zap.String("app_id", string(uid)))

	res := map[string]string{
		"app_id": string(uid),
	}

	return util.SuccessResponse(ctx, fiber.StatusCreated, res)
}

// UpdateApp updates an existing app for the developer.
func (d *Controller) UpdateApp(ctx *fiber.Ctx) error {
	// TODO: implement transferring organizations
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH

	if ctx.Params("appId") == "" {
		d.Logger.Error("[controllers][UpdateApp] developer -  error: App ID is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid App ID")
	}

	// deserialize the request body into blueprint.DeveloperApp
	var body blueprint.UpdateDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		d.Logger.Error("[controllers][UpdateApp] developer -  error: could not deserialize update request body", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Could not deserialize request body. Please make sure you pass the correct data")
	}

	if body.IntegrationPlatform == "" {
		if body.IntegrationAppID != "" {
			d.Logger.Error("[controllers][UpdateApp] developer -  error: app id is empty")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "no platform", "Please pass a valid platform alongside the app id")
		}

		if body.IntegrationAppSecret != "" {
			d.Logger.Error("[controllers][UpdateApp] developer -  error: app secret is empty")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "no platform", "Please pass a valid platform alongside the app secret")
		}
		if body.IntegrationRefreshToken != "" {
			d.Logger.Error("[controllers][UpdateApp] developer -  error: refresh token is empty")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "no platform", "Please pass a valid platform alongside the refresh token")
		}
	}

	// update the app
	database := db.NewDB{DB: d.DB}
	err := database.UpdateApp(ctx.Params("appId"), body.IntegrationPlatform, claims.DeveloperID, body)
	if err != nil {
		d.Logger.Error("[controllers][UpdateApp] developer -  error: could not update app in Database", zap.Error(err))
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", fmt.Sprintf("App with ID %s does not exist", ctx.Params("appId")))
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update developer app")
	}

	devApp, dErr := database.FetchAppByAppId(ctx.Params("appId"))
	if dErr != nil {
		d.Logger.Error("[controllers][UpdateApp] developer -  error: could not fetch app in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not fetch developer app")
	}
	convoyInstance := webhook.NewConvoy()

	if devApp.ConvoyEndpointID == "" {
		d.Logger.Warn("[controllers][UpdateApp] developer -  error: convoy endpoint id is empty")
		webhookName := fmt.Sprintf("%s-%s", devApp.Name, devApp.UID.String())
		whResponse, cErr := convoyInstance.CreateEndpoint(devApp.WebhookURL, devApp.Description, webhookName)
		if cErr != nil {
			d.Logger.Error("[controllers][UpdateApp] developer -  error: could not update convoy endpoint", zap.Error(cErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update developer app")
		}

		err = database.UpdateConvoyWebhookID(devApp.UID.String(), whResponse.ID)
		if err != nil {
			d.Logger.Error("[controllers][UpdateApp] developer -  error: could not update convoy endpoint id in Database", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update developer app")
		}

		d.Logger.Info("[controllers][UpdateApp] developer -  convoy endpoint updated", zap.String("app_id", ctx.Params("appId")))
	}

	// update convoy endpoint
	err = convoyInstance.UpdateEndpoint(devApp.ConvoyEndpointID, devApp.WebhookURL, devApp.Description, devApp.Name)
	if err != nil {
		d.Logger.Error("[controllers][UpdateApp] developer -  error: could not update convoy endpoint", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update developer app")
	}

	d.Logger.Info("[controllers][UpdateApp] developer -  app updated", zap.String("app_id", ctx.Params("appId")))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App updated successfully")
}

func (d *Controller) DeletePlatformIntegrationCredentials(ctx *fiber.Ctx) error {
	d.Logger.Info("[controllers][DeletePlatformIntegrationCredentials] developer -  deleting platform integration credentials")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH

	if ctx.Params("appId") == "" {
		d.Logger.Error("[controllers][DeletePlatformIntegrationCredentials] developer -  error: appId is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID")
	}

	if ctx.Params("platform") == "" {
		d.Logger.Error("[controllers][DeletePlatformIntegrationCredentials] developer -  error: platform is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is empty. Please pass a valid platform")
	}

	// delete the app
	database := db.NewDB{DB: d.DB}
	err := database.DeletePlatformIntegrationCredentials(ctx.Params("appId"), ctx.Params("platform"), claims.DeveloperID)
	if err != nil {
		log.Printf("[controllers][DeletePlatformIntegrationCredentials] developer -  error: could not delete platform integration credentials in Database: %v\n", err)
		d.Logger.Error("[controllers][DeletePlatformIntegrationCredentials] developer -  error: could not delete platform integration credentials in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not delete platform integration credentials")
	}

	d.Logger.Info("[controllers][DeletePlatformIntegrationCredentials] developer -  platform integration credentials deleted", zap.String("app_id", ctx.Params("appId")))
	return util.SuccessResponse(ctx, fiber.StatusOK, "Platform integration credentials deleted successfully")
}

func (d *Controller) DeleteApp(ctx *fiber.Ctx) error {
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH
	if ctx.Params("appId") == "" {
		d.Logger.Error("[controllers][DeleteApp] developer -  error: appId is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID")
	}
	convoyInstance := webhook.NewConvoy()
	database := db.NewDB{DB: d.DB}

	devApp, err := database.FetchAppByAppIdWithoutDevId(ctx.Params("appId"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Error("[controllers][DeleteApp] developer -  error: could not fetch app in Database. App does not exist", zap.Error(err), zap.String("app_id", ctx.Params("appId")))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not fetch developer app")
		}
	}

	if devApp.ConvoyEndpointID != "" {
		err = convoyInstance.DeleteEndpoint(devApp.ConvoyEndpointID)
		if err != nil {
			d.Logger.Error("[controllers][DeleteApp] developer -  error: could not delete convoy endpoint", zap.Error(err), zap.String("app_id", ctx.Params("appId")))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not delete developer app")
		}
	}

	// delete the app
	err = database.DeleteApp(ctx.Params("appId"), claims.DeveloperID)
	if err != nil {
		d.Logger.Error("[controllers][DeleteApp] developer -  error: could not delete app in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occured")
	}

	//err = convoyInstance.DeleteEndpoint(ctx.Params("webhookId"))
	d.Logger.Info("[controllers][DeleteApp] developer -  app deleted", zap.String("app_id", ctx.Params("appId")))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App deleted successfully")
}

func (d *Controller) FetchApp(ctx *fiber.Ctx) error {
	appId := ctx.Params("appId")
	if appId == "" {
		d.Logger.Error("[controllers][FetchApp] developer -  error: appId is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Could not create app ID")
	}

	// fetch the app
	database := db.NewDB{DB: d.DB}
	app, err := database.FetchAppByAppId(appId)
	if err != nil {
		d.Logger.Error("[controllers][FetchApp] developer -  error: could not fetch app in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred. Could not fetch database record")
	}

	var creds []blueprint.IntegrationCredentials
	var credK = map[string][]byte{
		"spotify":    app.SpotifyCredentials,
		"deezer":     app.DeezerCredentials,
		"applemusic": app.AppleMusicCredentials,
		"tidal":      app.TidalCredentials,
	}

	for k, v := range credK {
		d.Logger.Info("[controllers][FetchApp] developer -  decrypting credentials", zap.String("platform", k))
		if len(v) > 0 {
			outBytes, decErr := util.Decrypt(v, []byte(os.Getenv("ENCRYPTION_SECRET")))
			if decErr != nil {
				d.Logger.Error("[controllers][FetchApp] developer -  error: could not decrypt credentials", zap.Error(decErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, decErr, "An internal error occurred. Could not decrypt credentials")
			}

			d.Logger.Info("[controllers][FetchApp] developer -  credentials decrypted", zap.String("platform", k))
			var cred blueprint.IntegrationCredentials
			err = json.Unmarshal(outBytes, &cred)
			if err != nil {
				d.Logger.Error("[controllers][FetchApp] developer -  error: could not deserialize credentials", zap.Error(err))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred. Could not deserialize credentials")
			}
			creds = append(creds, cred)
		}
	}

	info := &blueprint.AppInfo{
		AppID:       app.UID.String(),
		Name:        app.Name,
		Description: app.Description,
		RedirectURL: app.RedirectURL,
		WebhookURL:  app.WebhookURL,
		PublicKey:   app.PublicKey.String(),
		Authorized:  app.Authorized,
		Credentials: creds,
		DeezerState: app.DeezerState,
	}
	return util.SuccessResponse(ctx, fiber.StatusOK, info)
}

func (d *Controller) DisableApp(ctx *fiber.Ctx) error {
	appId := ctx.Params("appId")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH
	if appId == "" {
		d.Logger.Error("[controllers][DisableApp] developer -  error: appId is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "appId is empty")
	}

	// disable the app
	database := db.NewDB{DB: d.DB}
	err := database.DisableApp(appId, claims.DeveloperID)
	if err != nil {
		d.Logger.Error("[controllers][DisableApp] developer -  error: could not disable app in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}

	// pause the convoy endpoint
	convoyInstance := webhook.NewConvoy()
	err = convoyInstance.PauseEndpoint(appId)
	if err != nil {
		d.Logger.Error("[controllers][DisableApp] developer -  error: could not pause convoy endpoint", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}
	d.Logger.Info("[controllers][DisableApp] developer -  convoy endpoint paused", zap.String("app_id", appId))
	d.Logger.Info("[controllers][DisableApp] developer -  app disabled", zap.String("app_id", appId))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App disabled successfully")
}

func (d *Controller) EnableApp(ctx *fiber.Ctx) error {
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH
	appId := ctx.Params("appId")
	if appId == "" {
		d.Logger.Error("[controllers][EnableApp] developer -  error: appId is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "AppId is empty. Please pass a valid app ID.")
	}

	// enable the app
	database := db.NewDB{DB: d.DB}
	err := database.EnableApp(appId, claims.DeveloperID)
	if err != nil {
		d.Logger.Error("[controllers][EnableApp] developer -  error: could not enable app in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}
	return util.SuccessResponse(ctx, fiber.StatusOK, "App enabled successfully")
}

func (d *Controller) FetchKeys(ctx *fiber.Ctx) error {
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH
	appId := ctx.Params("appId")
	if appId == "" {
		d.Logger.Error("[controllers][FetchKeys] developer -  error: appId is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	// fetch the app
	database := db.NewDB{DB: d.DB}
	keys, err := database.FetchAppKeys(appId, claims.DeveloperID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
		}
		d.Logger.Error("[controllers][FetchKeys] developer -  error: could not fetch app keys in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}
	// parse spotify credentials
	return util.SuccessResponse(ctx, fiber.StatusOK, keys)
}

func (d *Controller) FetchAllDeveloperApps(ctx *fiber.Ctx) error {
	// get the developer from the context
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)
	orgID := ctx.Params("orgId")

	// fetch the apps
	database := db.NewDB{DB: d.DB}
	apps, err := database.FetchApps(claims.DeveloperID, orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "No apps found for this organization.")
		}
		d.Logger.Error("[controllers][FetchAllDeveloperApps] developer -  error: could not fetch apps in Database", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occured.")
	}
	return util.SuccessResponse(ctx, fiber.StatusOK, apps)
}

func (d *Controller) RevokeAppKeys(ctx *fiber.Ctx) error {
	appId := ctx.Params("appId")
	if appId == "" {
		d.Logger.Warn("[controllers][RevokeAppKeys] developer -  error: appId is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	reqBody := struct {
		KeyType string `json:"key_type"`
	}{}

	if err := ctx.BodyParser(&reqBody); err != nil {
		d.Logger.Error("[controllers][RevokeAppKeys] developer -  error: could not deserialize request body", zap.Error(err))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Could not deserialize request body")
	}

	if reqBody.KeyType == "" {
		d.Logger.Error("[controllers][RevokeAppKeys] developer -  error: key type is empty")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Key type is empty. Please pass a valid key type")
	}

	database := db.NewDB{DB: d.DB}
	updatedKeys := &blueprint.AppKeys{}

	if reqBody.KeyType == "secret" {
		privateKey := uuid.NewString()
		err := database.RevokeSecretKey(appId, privateKey)
		if err != nil {
			d.Logger.Error("[controllers][RevokeAppKeys] developer -  error: could not revoke secret key in Database", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke secret key")
		}
		updatedKeys.SecretKey = privateKey
	}

	if reqBody.KeyType == "verify" {
		verifySecret := uuid.NewString()
		err := database.RevokeVerifySecret(appId, verifySecret)
		if err != nil {
			d.Logger.Error("[controllers][RevokeAppKeys] developer -  error: could not revoke verify secret in Database", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke verify secret")
		}
		updatedKeys.VerifySecret = verifySecret
	}

	if reqBody.KeyType == "public" {
		publicKey := uuid.NewString()
		err := database.RevokePublicKey(appId, publicKey)
		if err != nil {
			d.Logger.Error("[controllers][RevokeAppKeys] developer -  error: could not revoke public key in Database", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke public key")
		}
	}

	if reqBody.KeyType == "deezer_state" {
		deezerState := string(util.GenerateShortID())
		err := database.RevokeDeezerState(appId, deezerState)
		if err != nil {
			d.Logger.Error("[controllers][RevokeAppKeys] developer -  error: could not revoke deezer state in Database", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke deezer state")
		}
		updatedKeys.DeezerState = deezerState
	}
	d.Logger.Info("[controllers][RevokeAppKeys] developer -  app keys revoked and new credentials generated", zap.String("app_id", appId))
	return util.SuccessResponse(ctx, fiber.StatusOK, updatedKeys)
}
