package webhook

import (
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/util"
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

func (c *Controller) Handle(ctx *fiber.Ctx) error {
	log.Printf("==========================================================")
	log.Printf("[controller][webhook][Handle] - webhook event")
	// TODO: implement HMAC verification

	// get the type of webhook it is
	body := ctx.Body()
	var webhookMessage blueprint.WebhookMessage
	var err = json.Unmarshal(body, &webhookMessage)
	if err != nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid JSON")
	}

	log.Printf("[controller][webhook][Handle] - webhook message: %+v", webhookMessage)

	// get the webhook type
	webhookType := webhookMessage.Event
	log.Printf("[controller][webhook][Handle] - webhook type: %s", webhookType)
	switch webhookType {
	case blueprint.EEPLAYLISTCONVERSION:
		log.Printf("[controller][webhook][Handle] - Playlist converted")
		log.Printf("[controller][webhook][Handle] Message is - %+v\n", webhookMessage)
		log.Printf("==========================================================")
		return util.SuccessResponse(ctx, http.StatusOK, "Playlist converted")
	}
	log.Printf("==========================================================")
	return util.SuccessResponse(ctx, http.StatusOK, "webhook")
}
