package developer

import (
	"database/sql"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/util"
)

type DeveloperController struct {
	DB *sqlx.DB
}

func NewDeveloperController(db *sqlx.DB) *DeveloperController {
	return &DeveloperController{DB: db}
}

// CreateApp creates a new app for the developer. An app is a way to access the API, there can be multiple apps per developer.
func (d *DeveloperController) CreateApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][CreateApp] developer -  creating new app\n")

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

	// create new developer app
	database := db.NewDB{DB: d.DB}
	uid, err := database.CreateNewApp(body.Name, body.Description, body.RedirectURL, body.WebhookURL, pubKey, claims.UUID.String(), secretKey)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not create new developer app: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred and could not create developer app.")
	}

	log.Printf("[controllers][CreateApp] developer -  new app created: %s\n", body.Name)
	return util.SuccessResponse(ctx, fiber.StatusCreated, string(uid))
}

// UpdateApp updates an existing app for the developer.
func (d *DeveloperController) UpdateApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][UpdateApp] developer -  updating app\n")

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

	// update the app
	database := db.NewDB{DB: d.DB}
	err := database.UpdateApp(ctx.Params("appId"), body)
	if err != nil {
		log.Printf("[controllers][UpdateApp] developer -  error: could not update app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "Could not update developer app")
	}

	log.Printf("[controllers][UpdateApp] developer -  app updated: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App updated successfully")
}

func (d *DeveloperController) DeleteApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][DeleteApp] developer -  deleting app\n")

	if ctx.Params("appId") == "" {
		log.Printf("[controllers][DeleteApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID")
	}

	// delete the app
	database := db.NewDB{DB: d.DB}
	err := database.DeleteApp(ctx.Params("appId"))
	if err != nil {
		log.Printf("[controllers][DeleteApp] developer -  error: could not delete app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occured")
	}
	log.Printf("[controllers][DeleteApp] developer -  app deleted: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App deleted successfully")
}

func (d *DeveloperController) FetchApp(ctx *fiber.Ctx) error {
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
	//secK, err := util.Decrypt(app.SecretKey, []byte(os.Getenv("ENCRYPTION_SECRET")))
	//if err != nil {
	//	log.Printf("[controllers][FetchApp] developer -  error: could not decrypt secret key: %v\n", err)
	//	return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err)
	//}
	//
	//log.Printf("private key: %s", string(secK))
	info := map[string]interface{}{
		"app_id":       app.UID,
		"name":         app.Name,
		"description":  app.Description,
		"redirect_url": app.RedirectURL,
		"webhook_url":  app.WebhookURL,
		"public_key":   app.PublicKey,
		"authorized":   app.Authorized,
	}
	return util.SuccessResponse(ctx, fiber.StatusOK, info)
}

func (d *DeveloperController) DisableApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][DisableApp] developer -  disabling app\n")
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][DisableApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "appId is empty")
	}

	// disable the app
	database := db.NewDB{DB: d.DB}
	err := database.DisableApp(appId)
	if err != nil {
		log.Printf("[controllers][DisableApp] developer -  error: could not disable app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}

	log.Printf("[controllers][DisableApp] developer -  app disabled: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, "App disabled successfully")
}

func (d *DeveloperController) EnableApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][EnableApp] developer -  enabling app\n")
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][EnableApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "AppId is empty. Please pass a valid app ID.")
	}

	// enable the app
	database := db.NewDB{DB: d.DB}
	err := database.EnableApp(appId)
	if err != nil {
		log.Printf("[controllers][EnableApp] developer -  error: could not enable app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred")
	}

	log.Printf("[controllers][EnableApp] developer -  app enabled: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, "App enabled successfully")
}

func (d *DeveloperController) FetchKeys(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchKeys] developer -  fetching keys\n")
	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][FetchKeys] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is empty. Please pass a valid app ID.")
	}

	// fetch the app
	database := db.NewDB{DB: d.DB}
	keys, err := database.FetchAppKeys(appId)
	if err != nil {
		log.Printf("[controllers][FetchKeys] developer -  error: could not fetch app keys from the Database: %v\n", err)
		if err == sql.ErrNoRows {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err, "An internal error occurred.")
	}

	log.Printf("[controllers][FetchKeys] developer -  app keys fetched: %s\n", appId)
	return util.SuccessResponse(ctx, fiber.StatusOK, keys)
}

func (d *DeveloperController) FetchAllDeveloperApps(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchAllDeveloperApps] developer -  fetching all apps\n")

	// get the developer from the context
	claims := ctx.Locals("claims").(blueprint.OrchdioUserToken)
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
