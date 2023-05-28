package webhook

import (
	hmac2 "crypto/hmac"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/util"
	"os"
)

type Controller struct {
	DB  *sqlx.DB
	Red *redis.Client
}

// NewWebhookController creates a new webhook controller.
func NewWebhookController(db *sqlx.DB, red *redis.Client) *Controller {
	return &Controller{
		DB:  db,
		Red: red,
	}
}

func (c *Controller) AuthenticateWebhook(ctx *fiber.Ctx) error {
	log.Printf("[controller][webhook][Handle] - GET request to check webhook is valid")
	return ctx.SendStatus(http.StatusOK)
}

func (c *Controller) Handle(ctx *fiber.Ctx) error {
	//claims := ctx.Locals("developer").(*blueprint.OrchdioUserToken)
	if ctx.Method() == "GET" {
		log.Printf("[controller][webhook][Handle] - GET request. Might be webhook verification")
		return ctx.SendStatus(http.StatusOK)
	}

	log.Printf("==========================================================")
	log.Printf("[controller][webhook][Handle] - webhook event")
	orchdioHmac := ctx.Get("x-orchdio-hmac")
	database := db.NewDB{DB: c.DB}

	// get the type of webhook it is
	body := ctx.Body()
	var webhookMessage blueprint.WebhookMessage
	var err = json.Unmarshal(body, &webhookMessage)
	if err != nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid JSON")
	}

	user, uErr := database.FindUserByEmail(os.Getenv("ZOOVE_ADMIN_EMAIL"))
	if uErr != nil {
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error")
	}

	apiKey, aErr := database.FetchUserApikey(user.Email)
	log.Printf("[controller][webhook][Handle] - user apikey: %+v", apiKey.Key.String())
	if aErr != nil {
		log.Printf("[controller][webhook][Handle] - error fetching user apikey %s\n", aErr.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error")
	}
	hash := util.GenerateHMAC(webhookMessage, apiKey.Key.String())

	if hmac2.Equal([]byte(orchdioHmac), hash) {
		log.Printf("[controller][webhook][Handle] - error - hmac verification failed")
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "Unauthorized. Payload tampered with")
	}

	// get the webhook type
	webhookType := webhookMessage.Event
	log.Printf("[controller][webhook][Handle] - webhook type: %s", webhookType)
	switch webhookType {
	case blueprint.EEPLAYLISTCONVERSION:
		log.Printf("[controller][webhook][Handle] - Playlist converted")
		log.Printf("==========================================================")
		return util.SuccessResponse(ctx, http.StatusOK, "Playlist converted")
	}
	log.Printf("==========================================================")
	return util.SuccessResponse(ctx, http.StatusOK, "webhook")
}
