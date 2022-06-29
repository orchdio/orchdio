package account

import (
	"database/sql"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/util"
)

type WebhookController struct {
	DB *sqlx.DB
}

func NewWebhookController(db *sqlx.DB) *WebhookController {
	return &WebhookController{DB: db}
}

//func (w *WebhookController)

func (w *WebhookController) FetchWebhookUrl(c *fiber.Ctx) error {
	log.Printf("[controller][user][FetchWebhookUrl] - fetching webhook url")
	user := c.Locals("user").(*blueprint.User)

	database := db.NewDB{DB: w.DB}
	webhookUrl, err := database.FetchWebhook(user.UUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[controller][user][FetchWebhookUrl] - error - no webhook url found for user %v\n", user.UUID)
			return util.ErrorResponse(c, http.StatusNotFound, "Webhook not found")
		}
		log.Printf("[controller][user][FetchWebhookUrl] - error fetching webhook url %s\n", err.Error())
		return util.ErrorResponse(c, http.StatusInternalServerError, "An unexpected error")
	}
	response := map[string]string{
		"url": string(webhookUrl),
	}
	log.Printf("[controller][user][FetchWebhookUrl] - fetched webhook url: '%s' for user %v\n", string(webhookUrl), user)
	return util.SuccessResponse(c, http.StatusCreated, response)
}

// CreateWebhookUrl creates a webhook for a user
func (w *WebhookController) CreateWebhookUrl(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][CreateWebhookUrl] - creating webhook url")
	user := ctx.Locals("user").(*blueprint.User)
	//database := c.DB
	bod := ctx.Body()

	/**
	it'll look like:
		{
	      "url": "https://www.example.com/webhook",
		}
	*/

	webhoookBody := map[string]string{}
	err := json.Unmarshal(bod, &webhoookBody)
	if err != nil {
		log.Printf("[controller][user][CreateWebhookUrl] - error unmarshalling webhook body %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}

	webhookUrl := webhoookBody["url"]
	if webhookUrl == "" {
		log.Printf("[controller][user][CreateWebhookUrl] - error - webhook url is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Webhook url is empty")
	}

	log.Printf("[controller][user][CreateWebhookUrl] - webhook passed is: '%s' \n", webhookUrl)

	database := db.NewDB{DB: w.DB}

	// check if user already has a webhook
	_, whErr := database.FetchWebhook(user.UUID.String())

	log.Printf("\n[controller][user][CreateWebhookUrl] - webhook err: '%s' \n", whErr)

	if whErr == nil {
		// TODO: handle other possible fetchwebhook errors. for now just say "error fetching webhook"
		log.Printf("[controller][user][CreateWebhookUrl] - error fetching webhook url \n")
		return util.ErrorResponse(ctx, http.StatusConflict, "An unexpected error")
	}

	//if len(webhookUrlByte) > 0 {
	//	log.Printf("[controller][user][CreateWebhookUrl] - error - user already has a webhook url")
	//	return util.ErrorResponse(ctx, http.StatusBadRequest, "User already has a webhook url")
	//}

	// save into the database
	//res, err := database.D(webhookUrl, claims.UUID.String())

	err = database.CreateUserWebhook(user.UUID.String(), webhookUrl)
	if err != nil {
		if err == blueprint.EALREADY_EXISTS {
			log.Printf("[controller][user][CreateWebhookUrl] - error - user already has a webhook url")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "User already has a webhook url")
		}
		log.Printf("[controller][user][CreateWebhookUrl] - error creating webhook url %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}
	log.Printf("[controller][user][CreateWebhookUrl] - created webhook url: '%s' for user %v\n", webhookUrl, user)
	return util.SuccessResponse(ctx, http.StatusCreated, nil)
}

// UpdateUserWebhookUrl updates a webhook for a user
func (w *WebhookController) UpdateUserWebhookUrl(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][UpdateWebhookUrl] - updating webhook url")
	user := ctx.Locals("user").(*blueprint.User)
	//database := c.DB
	bod := ctx.Body()

	/**
	it'll look like:
		{
	      "url": "https://www.example.com/webhook",
		}
	*/

	webhoookBody := map[string]string{}
	err := json.Unmarshal(bod, &webhoookBody)
	if err != nil {
		log.Printf("[controller][user][UpdateWebhookUrl] - error unmarshalling webhook body %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}

	webhookUrl := webhoookBody["url"]
	if webhookUrl == "" {
		log.Printf("[controller][user][UpdateWebhookUrl] - error - webhook url is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Webhook url is empty")
	}

	log.Printf("[controller][user][UpdateWebhookUrl] - webhook passed is: '%s' \n", webhookUrl)

	database := db.NewDB{DB: w.DB}
	upErr := database.UpdateUserWebhook(user.UUID.String(), webhookUrl)
	if upErr != nil {
		if upErr == sql.ErrNoRows {
			log.Printf("[controller][user][UpdateWebhookUrl] - error - user does not have a webhook url")
			return util.ErrorResponse(ctx, http.StatusNotFound, "Webhook not found")
		}
		log.Printf("[controller][user][UpdateWebhookUrl] - error updating webhook url %s\n", upErr.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}
	log.Printf("[controller][user][UpdateWebhookUrl] - updated webhook url: '%s' for user %v\n", webhookUrl, user)
	res := map[string]string{
		"url": string(webhookUrl),
	}
	return util.SuccessResponse(ctx, http.StatusOK, res)
}

// DeleteUserWebhookUrl deletes a webhook for a user
func (w *WebhookController) DeleteUserWebhookUrl(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][DeleteUserWebhookUrl] - deleting webhook url")
	user := ctx.Locals("user").(*blueprint.User)

	database := db.NewDB{DB: w.DB}
	upErr := database.DeleteUserWebhook(user.UUID.String())

	if upErr != nil {
		if upErr == sql.ErrNoRows {
			log.Printf("[controller][user][DeleteUserWebhookUrl] - error - user does not have a webhook url")
			return util.ErrorResponse(ctx, http.StatusNotFound, "Webhook not found")
		}
		log.Printf("[controller][user][DeleteUserWebhookUrl] - error deleting webhook url %s\n", upErr.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}

	log.Printf("[controller][user][DeleteUserWebhookUrl] - deleted webhook url for user %v\n", user)
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}
