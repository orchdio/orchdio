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
	"orchdio/db/queries"
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
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
	// deserialize the request body
	var body blueprint.CreateNewDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not deserialize request body: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Could not deserialize request body")
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

	if !util.IsValidUUID(body.Organization) {
		log.Printf("[controllers][CreateApp] developer -  error: organization is not a valid UUID\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Organization is not a valid UUID. Please pass a valid organization")
	}

	if !lo.Contains(platforms, body.IntegrationPlatform) {
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is not valid. Please pass a valid platform")
	}

	redirectURL := fmt.Sprintf("%s/v1/auth/%s/callback", os.Getenv("APP_URL"), body.IntegrationPlatform)
	// if the platform to connect to is deezer, we want to add the deezer state as query params.
	// this is because deezer does not preserver state url upon redirect (see rest of code)
	if body.IntegrationPlatform == deezer.IDENTIFIER {
		redirectURL = fmt.Sprintf("%s&state=%s", redirectURL, deezerState)
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
	uid, err := database.CreateNewApp(body.Name, body.Description, redirectURL, body.WebhookURL, pubKey, claims.UUID.String(), secretKey, verifySecret, body.Organization)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not create new developer app: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	// update the deezer state in the database. this is unique
	// to deezer so it should be fine to update instead of add during app creation
	if body.IntegrationPlatform == deezer.IDENTIFIER {
		_, err = d.DB.Exec(queries.UpdateDeezerState, deezerState, string(uid))
		if err != nil {
			log.Printf("[controllers][CreateApp] developer -  error: could not update deezer state: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
		}
	}

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
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH

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

	if body.IntegrationPlatform == "" && (body.IntegrationAppID != "" || body.IntegrationAppSecret != "" || body.IntegrationRefreshToken != "") {
		log.Printf("[controllers][UpdateApp] developer -  error: platform is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Platform is empty. Please pass a valid platform")
	}

	// update the app
	database := db.NewDB{DB: d.DB}
	err := database.UpdateApp(ctx.Params("appId"), body.IntegrationPlatform, claims.UUID.String(), body)
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

func (d *Controller) DeleteApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][DeleteApp] developer -  deleting app\n")

	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
	if ctx.Params("appId") == "" {
		log.Printf("[controllers][DeleteApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID")
	}

	// delete the app
	database := db.NewDB{DB: d.DB}
	err := database.DeleteApp(ctx.Params("appId"), claims.UUID.String())
	if err != nil {
		log.Printf("[controllers][DeleteApp] developer -  error: could not delete app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occured")
	}
	log.Printf("[controllers][DeleteApp] developer -  app deleted: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App deleted successfully")
}

func (d *Controller) FetchApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchApp] developer -  fetching app\n")

	//claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
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

	var deezerOutBytes []byte
	var spotifyOutBytes []byte
	//var appleOutBytes []byte

	if string(app.DeezerCredentials) != "" {
		log.Printf("[controllers][FetchApp] developer -  decrypting deezer credentials\n")
		deezerOutBytes, err = util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[controllers][FetchApp] developer -  error: could not decrypt deezer credentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred. Could not decrypt deezer credentials")
		}

		log.Printf("[controllers][FetchApp] developer -  deezer credentials decrypted\n")
		spew.Dump(string(deezerOutBytes))
	}

	if string(app.SpotifyCredentials) != "" {
		log.Printf("[controllers][FetchApp] developer -  decrypting spotify credentials\n")
		spotifyOutBytes, err = util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[controllers][FetchApp] developer -  error: could not decrypt spotify credentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred. Could not decrypt spotify credentials")
		}

		log.Printf("[controllers][FetchApp] developer -  spotify credentials decrypted\n")
		spew.Dump(string(spotifyOutBytes))
	}

	info := &blueprint.AppInfo{
		AppID:       app.UID.String(),
		Name:        app.Name,
		Description: app.Description,
		RedirectURL: app.RedirectURL,
		WebhookURL:  app.WebhookURL,
		PublicKey:   app.PublicKey.String(),
		Authorized:  false,
	}

	return util.SuccessResponse(ctx, fiber.StatusOK, info)
}

func (d *Controller) DisableApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][DisableApp] developer -  disabling app\n")
	appId := ctx.Params("appId")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
	if appId == "" {
		log.Printf("[controllers][DisableApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "appId is empty")
	}

	// disable the app
	database := db.NewDB{DB: d.DB}
	err := database.DisableApp(appId, claims.UUID.String())
	if err != nil {
		log.Printf("[controllers][DisableApp] developer -  error: could not disable app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}

	log.Printf("[controllers][DisableApp] developer -  app disabled: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, "App disabled successfully")
}

func (d *Controller) EnableApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][EnableApp] developer -  enabling app\n")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][EnableApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "AppId is empty. Please pass a valid app ID.")
	}

	// enable the app
	database := db.NewDB{DB: d.DB}
	err := database.EnableApp(appId, claims.UUID.String())
	if err != nil {
		log.Printf("[controllers][EnableApp] developer -  error: could not enable app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}

	log.Printf("[controllers][EnableApp] developer -  app enabled: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, "App enabled successfully")
}

func (d *Controller) FetchKeys(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchKeys] developer -  fetching keys\n")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH

	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][FetchKeys] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	// fetch the app
	database := db.NewDB{DB: d.DB}
	keys, err := database.FetchAppKeys(appId, claims.UUID.String())
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
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
	// fetch the apps
	database := db.NewDB{DB: d.DB}
	apps, err := database.FetchApps(claims.UUID.String())
	if err != nil {
		log.Printf("[controllers][FetchAllDeveloperApps] developer -  error: could not fetch apps in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occured.")
	}

	log.Printf("[controllers][FetchAllDeveloperApps] developer -  apps fetched: %s\n", claims.UUID.String())
	return util.SuccessResponse(ctx, fiber.StatusOK, apps)
}

func (d *Controller) RevokeAppKeys(ctx *fiber.Ctx) error {
	log.Printf("[controllers][RevokeAppKeys] developer -  revoking app keys\n")
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][RevokeAppKeys] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	// revoke the app keys
	database := db.NewDB{DB: d.DB}
	publicKey := uuid.NewString()
	privateKey := uuid.NewString()
	verifySecret := uuid.NewString()
	deezerState := string(util.GenerateShortID())
	err := database.UpdateAppKeys(publicKey,
		privateKey, verifySecret, deezerState, appId)
	if err != nil {
		log.Printf("[controllers][RevokeAppKeys] developer -  error: could not revoke app keys in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}

	updatedKeys := &blueprint.AppKeys{
		PublicKey:    publicKey,
		SecretKey:    privateKey,
		VerifySecret: verifySecret,
	}

	log.Printf("[controllers][RevokeAppKeys] developer -  app keys revoked and new credentials generated: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, updatedKeys)
}
