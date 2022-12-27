package developer

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/util"
	"os"
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
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken) // THIS WILL MOST LIKELY CRASH
	// deserialize the request body
	var body blueprint.CreateNewDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not deserialize request body: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "Could not deserialize request body")
	}
	pubKey := uuid.NewString()
	secretKey, err := util.Encrypt([]byte(uuid.NewString()), []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not encrypt secret key during app creation: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "Could not encrypt secret key during app creation")
	}

	// create new developer
	database := db.NewDB{DB: d.DB}
	uid, err := database.CreateNewApp(body.Name, body.Description, body.RedirectURL, body.WebhookURL, pubKey, claims.UUID.String(), secretKey)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not create new developer app: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err)
	}

	log.Printf("[controllers][CreateApp] developer -  new app created: %s\n", body.Name)
	return util.SuccessResponse(ctx, fiber.StatusCreated, string(uid))
}

func (d *DeveloperController) UpdateApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][UpdateApp] developer -  updating app\n")

	if ctx.Params("appId") == "" {
		log.Printf("[controllers][UpdateApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "appId is empty")
	}

	// deserialize the request body into blueprint.DeveloperApp
	var body blueprint.UpdateDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		log.Printf("[controllers][UpdateApp] developer -  error: could not deserialize update request body: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "Could not deserialize request body. Please make sure you pass the correct data")
	}

	// update the app
	database := db.NewDB{DB: d.DB}
	err := database.UpdateApp(ctx.Params("appId"), body)
	if err != nil {
		log.Printf("[controllers][UpdateApp] developer -  error: could not update app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err)
	}

	log.Printf("[controllers][UpdateApp] developer -  app updated: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App updated successfully")
}

func (d *DeveloperController) DeleteApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][DeleteApp] developer -  deleting app\n")

	if ctx.Params("appId") == "" {
		log.Printf("[controllers][DeleteApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "appId is empty")
	}

	// delete the app
	database := db.NewDB{DB: d.DB}
	err := database.DeleteApp(ctx.Params("appId"))
	if err != nil {
		log.Printf("[controllers][DeleteApp] developer -  error: could not delete app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err)
	}
	log.Printf("[controllers][DeleteApp] developer -  app deleted: %s\n", ctx.Params("appId"))
	return util.SuccessResponse(ctx, fiber.StatusOK, "App deleted successfully")
}

func (d *DeveloperController) FetchApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][FetchApp] developer -  fetching app\n")

	appId := ctx.Params("appId")
	if appId == "" {
		log.Printf("[controllers][FetchApp] developer -  error: appId is empty\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "appId is empty")
	}

	// fetch the app
	database := db.NewDB{DB: d.DB}
	app, err := database.FetchAppByAppId(appId)
	if err != nil {
		log.Printf("[controllers][FetchApp] developer -  error: could not fetch app in Database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err)
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
