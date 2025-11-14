package developer

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type Controller struct {
	DB          *sqlx.DB
	SvixService svixwebhook.SvixInterface
}

func NewDeveloperController(db *sqlx.DB, webhookInterface svixwebhook.SvixInterface) *Controller {
	return &Controller{DB: db, SvixService: webhookInterface}
}

// CreateApp creates a new app for the developer. An app is a way to access the API, there can be multiple apps per developer.
func (d *Controller) CreateApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][CreateApp] developer -  creating new app\n")
	platforms := []string{applemusic.IDENTIFIER, deezer.IDENTIFIER, spotify.IDENTIFIER, tidal.IDENTIFIER, ytmusic.IDENTIFIER}
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
	// TODO: check if the app id and app secret are valid for the platform
	// create a json object to store the app data. for now, we assume all apps have app id and app secret
	appData := blueprint.IntegrationCredentials{
		AppID:     body.IntegrationAppId,
		AppSecret: body.IntegrationAppSecret,
		Platform:  body.IntegrationPlatform,
	}

	serializedAppData, err := json.Marshal(appData)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not marshal app data: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	//encrypt the app data
	encryptedAppData, err := util.Encrypt(serializedAppData, []byte(os.Getenv("ENCRYPTION_SECRET")))
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

	webhookName := fmt.Sprintf("%s:%s", body.Name, string(uid))
	webhookAppUID := svixwebhook.FormatSvixAppUID(string(uid))
	whResponse, _, whErr := d.SvixService.CreateApp(webhookName, webhookAppUID)

	if whErr != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not create developer app: %v\n", whErr)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, whErr, "An internal error occurred and could not create developer app.")
	}

	endpointUniqID := svixwebhook.FormatSvixEndpointUID(string(uid))
	_, epErr := d.SvixService.CreateEndpoint(whResponse.Id, endpointUniqID, body.WebhookURL)
	if epErr != nil {
		log.Printf("[controller][CreateApp] developer -  error: could not create developer app: %v\n", epErr)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, epErr, "An internal error occurred and could not create developer app.")
	}

	updErr := database.UpdateWebhookAppID(string(uid), whResponse.Id)
	if updErr != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not update developer app: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, updErr, "An internal error occurred and could not update developer app.")
	}

	err = database.UpdateIntegrationCredentials(encryptedAppData, string(uid), body.IntegrationPlatform, body.RedirectURL, body.WebhookURL, whResponse.Id)
	log.Printf("[controllers][CreateApp] developer -  new app created: %s\n", body.Name)

	res := &blueprint.CreateNewDevAppResponse{
		AppId: string(uid),
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
	updatedApp, err := database.UpdateApp(ctx.Params("appId"), body.IntegrationPlatform, claims.DeveloperID, body)
	if err != nil {
		log.Printf("[controllers][UpdateApp] developer -  error: could not update app in Database: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", fmt.Sprintf("App with ID %s does not exist", ctx.Params("appId")))
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update developer app")
	}

	svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	// todo: finalize webhook name format standard.
	webhookName := fmt.Sprintf("%s-orchdio-%s", updatedApp.Name, updatedApp.UID.String())
	whAppName := fmt.Sprintf("%s:%s", updatedApp.Name, updatedApp.UID.String())
	endpointUniqID := svixwebhook.FormatSvixEndpointUID(updatedApp.UID.String())

	webhookAppUID := svixwebhook.FormatSvixAppUID(updatedApp.UID.String())

	// if webhook app id is empty, then that means the application was not created on Svix.
	// this is largely due to backwards compatibility.
	if updatedApp.WebhookAppID == "" {
		// check if app already exists
		// hack: not sure how to get the correct error object from the svix go module. the error returned seems to be a string
		// so the solution below uses keyword detection.
		log.Printf("[controllers][updateApp] developer -  error: app does not exist\n")

		// create new app on svix
		svixApp, _, appErr := svixInstance.CreateApp(whAppName, webhookAppUID)
		if appErr != nil {
			log.Printf("[controllers][UpdateApp] developer -  error: could not create app in Database: %v\n", appErr)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, appErr, "Could not create developer app")
		}

		_, whErr := svixInstance.CreateEndpoint(svixApp.Id, endpointUniqID, updatedApp.WebhookURL)
		if whErr != nil {
			log.Printf("[controllers][UpdateApp] developer -  error: could not create webhook: %v\n", whErr)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, whErr, "Could not create webhook")
		}

		err = database.UpdateWebhookAppID(updatedApp.UID.String(), svixApp.Id)
		if err != nil {
			log.Printf("[controllers][UpdateApp] developer -  error: could not update webhook: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update webhook")
		}
		updatedApp.WebhookAppID = svixApp.Id
		updatedApp.Name = webhookName
		return util.SuccessResponse(ctx, fiber.StatusCreated, "App updated successfully")
	}

	// note: here we are checking if the endpoint exists for an app. This is because at the moment, we want to make sure
	// an app has only one webhook url (equivalent to endpoint in Svix). We're doing this in order to reduce the cognitive
	// load of thinking about multiple endpoints for a single application and how to handle making sure events
	// will be sent only to appropriate endpoints.
	//
	// we could ideally do this by simply making sure that when we create a new endpoint, we subscribe them to only the
	// events we expect them to be subscribed to. But this would require having a way to specify which events to support
	// for each application on Svix. Svix offers Portal to do this (last i checked) but the thing is that this is exactly
	// (part of) the cognitive load — we dont want to bother about thinking  3rd party integration docs and architecture
	// right now
	_, epErr := svixInstance.GetEndpoint(updatedApp.WebhookAppID, endpointUniqID)
	if epErr != nil {
		log.Printf("[controllers][updateApp] developer -  error: could not get existing svix app: %v\n", epErr)
		log.Printf("WH error: %v\n", epErr.Error())
		if strings.Contains(epErr.Error(), "404") {
			// create a new endpoint.
			_, cErr := svixInstance.CreateEndpoint(updatedApp.WebhookAppID, endpointUniqID, updatedApp.WebhookURL)
			if cErr != nil {
				log.Printf("[controllers][UpdateApp] developer -  error: could not create endpoint: %v\n", cErr)
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, cErr, "Could not create endpoint")
			}
			return util.SuccessResponse(ctx, fiber.StatusCreated, "App updated successfully")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, epErr, "Could not get existing svix app")
	}

	// update Svix endpoint
	_, uErr := svixInstance.UpdateEndpoint(updatedApp.WebhookAppID, endpointUniqID, updatedApp.WebhookURL)
	if uErr != nil {
		log.Printf("[controllers][updateApp] developer -  error: could not update endpoint: %v\n", uErr)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, uErr, "Could not update endpoint")
	}

	log.Println("[controllers][UpdateApp] developer -  app updated", zap.String("app_id", ctx.Params("appId")))
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
		spotify.IDENTIFIER:    app.SpotifyCredentials,
		deezer.IDENTIFIER:     app.DeezerCredentials,
		applemusic.IDENTIFIER: app.AppleMusicCredentials,
		tidal.IDENTIFIER:      app.TidalCredentials,
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

	// create a new app portal that lives long enough
	svixWebhook := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	// get app portal
	portalAccess, err := svixWebhook.CreateAppPortal(app.WebhookAppID)

	if err != nil {
		log.Print("Error fetching app portal...", err)
		return util.SuccessResponse(ctx, fiber.StatusInternalServerError, "Could not fetch Dev app due to internal errors")
	}

	info := &blueprint.AppInfo{
		AppID:            app.UID.String(),
		Name:             app.Name,
		Description:      app.Description,
		RedirectURL:      app.RedirectURL,
		WebhookURL:       app.WebhookURL,
		PublicKey:        app.PublicKey.String(),
		Authorized:       app.Authorized,
		Credentials:      creds,
		DeezerState:      app.DeezerState,
		WebhookPortalURL: portalAccess.Url,
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
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}
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
	return util.SuccessResponse(ctx, fiber.StatusOK, apps)
}

func (d *Controller) RevokeAppKeys(ctx *fiber.Ctx) error {
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][RevokeAppKeys] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	// todo: move this into a proper type
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

	if reqBody.KeyType == blueprint.SecretKeyType {
		privateKey := uuid.NewString()
		err := database.RevokeSecretKey(appId, privateKey)
		if err != nil {
			log.Printf("-  error: could not revoke secret key in Database: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke secret key")
		}
		updatedKeys.SecretKey = privateKey
	}

	if reqBody.KeyType == blueprint.VerifyKeyType {
		verifySecret := uuid.NewString()
		err := database.RevokeVerifySecret(appId, verifySecret)
		if err != nil {
			log.Printf("-  error: could not revoke verify secret in Database: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke verify secret")
		}
		updatedKeys.VerifySecret = verifySecret
	}

	if reqBody.KeyType == blueprint.PublicKeyType {
		publicKey := uuid.NewString()
		err := database.RevokePublicKey(appId, publicKey)
		if err != nil {
			log.Printf("-  error: could not revoke public key in Database: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not revoke public key")
		}
	}

	if reqBody.KeyType == blueprint.DeezerStateType {
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
