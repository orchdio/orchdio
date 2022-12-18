package developer

import (
	"github.com/gofiber/fiber/v2"
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

func (d *DeveloperController) CreateApp(ctx *fiber.Ctx) error {
	log.Printf("[controllers][CreateApp] developer -  creating new app\n")
	// deserialize the request body
	var body blueprint.CreateNewDeveloperAppData
	if err := ctx.BodyParser(&body); err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not deserialize request body: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "Could not deserialize request body")
	}

	// create new developer
	database := db.NewDB{DB: d.DB}
	uid, err := database.CreateNewApp(body.Name, body.Description, body.RedirectURL, body.WebhookURL)
	if err != nil {
		log.Printf("[controllers][CreateApp] developer -  error: could not create new developer app: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, err)
	}

	log.Printf("[controllers][CreateApp] developer -  new app created: %s\n", body.Name)
	return util.SuccessResponse(ctx, fiber.StatusCreated, uid)
}
