package developer

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"log"
	"orchdio/blueprint"
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
	DB *sqlx.DB
}

func NewDeveloperController(db *sqlx.DB) *Controller {
	return &Controller{DB: db}
}

// CreateApp creates a new app for the developer. An app is a way to access the API, there can be multiple apps per developer.
func (d *Controller) CreateApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][CreateApp] developer -  creating new app\n")
	platforms := []string{applemusic.IDENTIFIER, deezer.IDENTIFIER, spotify.IDENTIFIER, tidal.IDENTIFIER, ytmusic.IDENTIFIER}

	// get the claims from the context
	// FIXME: perhaps use reflection to check type and gracefully handle
	//claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)
	// deserialize the request body
	var body blueprint.CreateNewDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not deserialize request body: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Could not deserialize request body")
	}
	orgId := ctx.Params("orgId")
	if orgId == "" {
		log.Printf("[controllers][CreateApp] developer -  error: organization id is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Organization ID is empty. Please pass a valid organization id")
	}

	pubKey := uuid.NewString()
	secretKey := uuid.NewString()
	verifySecret := uuid.NewString()
	deezerState := string(util.GenerateShortID())

	if body.Organization == "" {
		log.Printf("[controllers][CreateApp] developer -  error: organization is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Organization is empty. Please pass a valid organization")
	}

	if body.IntegrationAppId == "" {
		log.Printf("[controllers][CreateApp] developer -  error: app id is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Integration App ID is empty. Please pass a valid app id")
	}

	if body.IntegrationAppSecret == "" {
		log.Printf("[controllers][CreateApp] developer -  error: app secret is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Integration App Secret is empty. Please pass a valid app secret")
	}

	if body.IntegrationPlatform == "" {
		log.Printf("[controllers][CreateApp] developer -  error: platform is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is empty. Please pass a valid platform")
	}

	if body.WebhookURL == "" {
		log.Printf("[controllers][CreateApp] developer -  error: webhook url is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Webhook URL is empty. Please pass a valid webhook url")
	}

	if !util.IsValidUUID(body.Organization) {
		log.Printf("[controllers][CreateApp] developer -  error: organization is not a valid UUID\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Organization is not a valid UUID. Please pass a valid organization")
	}

	if !lo.Contains(platforms, body.IntegrationPlatform) {
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is not valid. Please pass a valid platform")
	}

	webhookURL := body.WebhookURL
	redirectURL := body.RedirectURL

	log.Printf("Webhook and redirect")
	spew.Dump(webhookURL, redirectURL)
	//redirectURL := fmt.Sprintf("%s/v1/auth/%s/callback", os.Getenv("APP_URL"), body.IntegrationPlatform)
	//// if the platform to connect to is deezer, we want to add the deezer state as query params.
	//// this is because deezer does not preserver state url upon redirect (see rest of code)
	//if body.IntegrationPlatform == deezer.IDENTIFIER {
	//	redirectURL = fmt.Sprintf("%s&state=%s", redirectURL, deezerState)
	//} else{
	//	redirectURL
	//}

	log.Printf("Incoming creaation data is")
	spew.Dump(body)

	// TODO: check if the app id and app secret are valid for the platform

	// create a json object to store the app data. for now, we assume all apps have app id and app secret
	appData := blueprint.IntegrationCredentials{
		AppID:     body.IntegrationAppId,
		AppSecret: body.IntegrationAppSecret,
		Platform:  body.IntegrationPlatform,
	}

	ser, err := json.Marshal(appData)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not marshal app data: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	//encrypt the app data
	encryptedAppData, err := util.Encrypt(ser, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not encrypt app data: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	// create new developer app
	database := db.NewDB{DB: d.DB}
	uid, err := database.CreateNewApp(body.Name, body.Description, body.RedirectURL, body.WebhookURL, pubKey, claims.DeveloperID, secretKey, verifySecret, body.Organization, deezerState)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not create new developer app: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	// update the deezer state in the database. this is unique
	// to deezer so it should be fine to update instead of add during app creation
	//if body.IntegrationPlatform == deezer.IDENTIFIER {
	//	_, err = d.DB.Exec(queries.UpdateDeezerState, deezerState, string(uid))
	//	if err != nil {
	//		log.Printf("[controllers][CreateApp] developer -  error: could not update deezer state: %v\n", err)
	//		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	//	}
	//}

	// update the app credentials
	err = database.UpdateIntegrationCredentials(encryptedAppData, string(uid), body.IntegrationPlatform, body.RedirectURL, body.WebhookURL)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not update app credentials: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	log.Printf("[controllers][CreateApp] developer -  new app created: %s\n", body.Name)
	res := map[string]string{
		"app_id": string(uid),
	}

	return util.SuccessResponse(ctx, fiber.StatusCreated, res)
}

// UpdateApp updates an existing app for the developer.
func (d *Controller) UpdateApp(ctx *fiber.Ctx) error {
	// TODO: implement transferring organizations
	log.Printf("[controllers][UpdateApp] developer -  updating app\n")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH

	if ctx.Params("appId") == "" {
		log.Printf("[controllers][UpdateApp] developer -  error: App ID is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid App ID")
	}

	// deserialize the request body into blueprint.DeveloperApp
	var body blueprint.UpdateDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		log.Printf("[controllers][UpdateApp] developer -  error: could not deserialize update request body: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Could not deserialize request body. Please make sure you pass the correct data")
	}

	//if body.IntegrationPlatform == "" && (body.IntegrationAppID != "" || body.IntegrationAppSecret != "" || body.IntegrationRefreshToken != "") {
	//	log.Printf("[controllers][UpdateApp] developer -  error: platform is empty\n")
	//	return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is empty. Please pass a valid platform")
	//}

	if body.IntegrationPlatform == "" {
		if body.IntegrationAppID != "" {
			log.Printf("[controllers][UpdateApp] developer -  error: app id is empty\n")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "no platform", "Please pass a valid platform alongside the app id")
		}
		if body.IntegrationAppSecret != "" {
			log.Printf("[controllers][UpdateApp] developer -  error: app secret is empty\n")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "no platform", "Please pass a valid platform alongside the app secret")
		}
		if body.IntegrationRefreshToken != "" {
			log.Printf("[controllers][UpdateApp] developer -  error: refresh token is empty\n")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "no platform", "Please pass a valid platform alongside the refresh token")
		}
	}

	// update the app
	database := db.NewDB{DB: d.DB}
	err := database.UpdateApp(ctx.Params("appId"), body.IntegrationPlatform, claims.DeveloperID, body)
	if err != nil {
		log.Printf("[controllers][UpdateApp] developer -  error: could not update app in Database: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", fmt.Sprintf("App with ID %s does not exist", ctx.Params("appId")))
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update developer app")
	}

	log.Printf("[controllers][UpdateApp] developer -  app updated: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App updated successfully")
}

func (d *Controller) DeletePlatformIntegrationCredentials(ctx *fiber.Ctx) error {
	log.Printf("[controllers][DeletePlatformIntegrationCredentials] developer -  deleting platform integration credentials\n")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH

	if ctx.Params("appId") == "" {
		log.Printf("[controllers][DeletePlatformIntegrationCredentials] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID")
	}

	if ctx.Params("platform") == "" {
		log.Printf("[controllers][DeletePlatformIntegrationCredentials] developer -  error: platform is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is empty. Please pass a valid platform")
	}

	// delete the app
	database := db.NewDB{DB: d.DB}
	err := database.DeletePlatformIntegrationCredentials(ctx.Params("appId"), ctx.Params("platform"), claims.DeveloperID)
	if err != nil {
		log.Printf("[controllers][DeletePlatformIntegrationCredentials] developer -  error: could not delete platform integration credentials in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not delete platform integration credentials")
	}

	log.Printf("[controllers][DeletePlatformIntegrationCredentials] developer -  platform integration credentials deleted: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "Platform integration credentials deleted successfully")
}

func (d *Controller) DeleteApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][DeleteApp] developer -  deleting app\n")

	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH
	if ctx.Params("appId") == "" {
		log.Printf("[controllers][DeleteApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID")
	}

	// delete the app
	database := db.NewDB{DB: d.DB}
	err := database.DeleteApp(ctx.Params("appId"), claims.DeveloperID)
	if err != nil {
		log.Printf("[controllers][DeleteApp] developer -  error: could not delete app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occured")
	}
	log.Printf("[controllers][DeleteApp] developer -  app deleted: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App deleted successfully")
}

func (d *Controller) FetchApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchApp] developer -  fetching app\n")

	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][FetchApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Could not create app ID")
	}

	// fetch the app
	database := db.NewDB{DB: d.DB}
	app, err := database.FetchAppByAppId(appId)
	if err != nil {
		log.Printf("[controllers][FetchApp] developer -  error: could not fetch app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred. Could not fetch database record")
	}

	log.Printf("[controllers][FetchApp] developer -  app fetched: %s\n", appId)

	var creds []blueprint.IntegrationCredentials

	var credK = map[string][]byte{
		"spotify":    app.SpotifyCredentials,
		"deezer":     app.DeezerCredentials,
		"applemusic": app.AppleMusicCredentials,
		"tidal":      app.TidalCredentials,
	}

	for k, v := range credK {
		log.Printf("[controllers][FetchApp] developer -  decrypting %s credentials\n", k)
		if len(v) > 0 {
			outBytes, decErr := util.Decrypt(v, []byte(os.Getenv("ENCRYPTION_SECRET")))
			if decErr != nil {
				log.Printf("[controllers][FetchApp] developer -  error: could not decrypt %s credentials: %v\n", k, decErr)
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, decErr, "An internal error occurred. Could not decrypt credentials")
			}

			log.Printf("[controllers][FetchApp] developer -  %s credentials decrypted\n", k)
			var cred blueprint.IntegrationCredentials
			err = json.Unmarshal(outBytes, &cred)
			if err != nil {
				log.Printf("[controllers][FetchApp] developer -  error: could not deserialize %s credentials: %v\n", k, err)
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
	log.Printf("[controllers][DisableApp] developer -  disabling app\n")
	appId := ctx.Params("appId")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH
	if appId == "" {
		log.Printf("[controllers][DisableApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "appId is empty")
	}

	// disable the app
	database := db.NewDB{DB: d.DB}
	err := database.DisableApp(appId, claims.DeveloperID)
	if err != nil {
		log.Printf("[controllers][DisableApp] developer -  error: could not disable app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}

	log.Printf("[controllers][DisableApp] developer -  app disabled: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, "App disabled successfully")
}

func (d *Controller) EnableApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][EnableApp] developer -  enabling app\n")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][EnableApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "AppId is empty. Please pass a valid app ID.")
	}

	// enable the app
	database := db.NewDB{DB: d.DB}
	err := database.EnableApp(appId, claims.DeveloperID)
	if err != nil {
		log.Printf("[controllers][EnableApp] developer -  error: could not enable app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}

	log.Printf("[controllers][EnableApp] developer -  app enabled: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, "App enabled successfully")
}

func (d *Controller) FetchKeys(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchKeys] developer -  fetching keys\n")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT) // THIS WILL MOST LIKELY CRASH

	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][FetchKeys] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	// fetch the app
	database := db.NewDB{DB: d.DB}
	keys, err := database.FetchAppKeys(appId, claims.DeveloperID)
	if err != nil {
		log.Printf("[controllers][FetchKeys] developer -  error: could not fetch app keys from the Database: %v\n", err)
		if err == sql.ErrNoRows {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}

	log.Printf("[controllers][FetchKeys] developer -  app keys fetched: %s\n", appId)

	// parse spotify credentials
	return util.SuccessResponse(ctx, fiber.StatusOK, keys)
}

func (d *Controller) FetchAllDeveloperApps(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchAllDeveloperApps] developer -  fetching all apps\n")

	// get the developer from the context
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)
	orgID := ctx.Params("orgId")

	// fetch the apps
	database := db.NewDB{DB: d.DB}
	apps, err := database.FetchApps(claims.DeveloperID, orgID)
	if err != nil {
		log.Printf("[controllers][FetchAllDeveloperApps] developer -  error: could not fetch apps in Database: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "No apps found for this organization.")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occured.")
	}

	log.Printf("[controllers][FetchAllDeveloperApps] developer -  apps fetched: %s\n", claims.DeveloperID)
	return util.SuccessResponse(ctx, fiber.StatusOK, apps)
}

func (d *Controller) RevokeAppKeys(ctx *fiber.Ctx) error {
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][RevokeAppKeys] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	reqBody := struct {
		KeyType string `json:"key_type"`
	}{}

	if err := ctx.BodyParser(&reqBody); err != nil {
		log.Printf("[controllers][RevokeAppKeys] developer -  error: could not deserialize request body: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Could not deserialize request body")
	}

	if reqBody.KeyType == "" {
		log.Printf("[controllers][RevokeAppKeys] developer -  error: key type is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Key type is empty. Please pass a valid key type")
	}

	database := db.NewDB{DB: d.DB}
	updatedKeys := &blueprint.AppKeys{}

	if reqBody.KeyType == "secret" {
		privateKey := uuid.NewString()
		err := database.RevokeSecretKey(appId, privateKey)
		if err != nil {
			log.Printf("-  error: could not revoke secret key in Database: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke secret key")
		}
		updatedKeys.SecretKey = privateKey
	}

	if reqBody.KeyType == "verify" {
		verifySecret := uuid.NewString()
		err := database.RevokeVerifySecret(appId, verifySecret)
		if err != nil {
			log.Printf("-  error: could not revoke verify secret in Database: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke verify secret")
		}
		updatedKeys.VerifySecret = verifySecret
	}

	if reqBody.KeyType == "public" {
		publicKey := uuid.NewString()
		err := database.RevokePublicKey(appId, publicKey)
		if err != nil {
			log.Printf("-  error: could not revoke public key in Database: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke public key")
		}
	}

	if reqBody.KeyType == "deezer_state" {
		deezerState := string(util.GenerateShortID())
		err := database.RevokeDeezerState(appId, deezerState)
		if err != nil {
			log.Printf("-  error: could not revoke deezer state in Database: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke deezer state")
		}
		updatedKeys.DeezerState = deezerState
	}

	log.Printf("-  app keys revoked and new credentials generated: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, updatedKeys)
}
