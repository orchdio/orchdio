package account

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"github.com/vicanso/go-axios"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/util"
)

type WebhookController struct {
	DB *sqlx.DB
}

func NewAccountWebhookController(db *sqlx.DB) *WebhookController {
	return &WebhookController{DB: db}
}

func (w *WebhookController) FetchWebhookUrl(c *fiber.Ctx) error {
	log.Printf("[controller][user][FetchWebhookUrl] - fetching webhook url")
	user := c.Locals("user").(*blueprint.User)

	database := db.NewDB{DB: w.DB}
	webhook, err := database.FetchWebhook(user.UUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[controller][user][FetchWebhookUrl] - error - no webhook url found for user %v\n", user.UUID)
			return util.ErrorResponse(c, http.StatusNotFound, "not found", "No webhook information found for user")
		}
		log.Printf("[controller][user][FetchWebhookUrl] - error fetching webhook url %s\n", err.Error())
		return util.ErrorResponse(c, http.StatusInternalServerError, err.Error(), "An unexpected error")
	}
	response := map[string]string{
		"url": webhook.Url,
	}
	log.Printf("[controller][user][FetchWebhookUrl] - fetched webhook url: '%s' for user %v\n", webhook.Url, user)
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

	webhoookBody := struct {
		Url         string `json:"url"`
		VerifyToken string `json:"verify_token"`
	}{}

	err := json.Unmarshal(bod, &webhoookBody)
	if err != nil {
		log.Printf("[controller][user][CreateWebhookUrl] - error unmarshalling webhook body %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err.Error(), "An unexpected error")
	}

	webhookUrl := webhoookBody.Url
	if webhookUrl == "" {
		log.Printf("[controller][user][CreateWebhookUrl] - error - webhook url is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Webhook url is empty")
	}

	if webhoookBody.VerifyToken == "" {
		log.Printf("[controller][user][CreateWebhookUrl] - error - verify token is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Verify token is empty")
	}

	if len(webhookUrl) > 100 {
		log.Printf("[controller][user][CreateWebhookUrl] - error - webhook url is too long")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Webhook url is too long")
	}

	if len(webhoookBody.VerifyToken) > 500 {
		log.Printf("[controller][user][CreateWebhookUrl] - error - verify token is too long")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Verify token is too long")
	}

	log.Printf("[controller][user][CreateWebhookUrl] - webhook passed is: '%s' \n", webhookUrl)

	database := db.NewDB{DB: w.DB}

	// check if user already has a webhook
	_, whErr := database.FetchWebhook(user.UUID.String())

	log.Printf("\n[controller][user][CreateWebhookUrl] - webhook err: '%s' \n", whErr)

	if whErr != nil && !errors.Is(whErr, sql.ErrNoRows) {
		// TODO: handle other possible fetch webhook errors. for now just say "error fetching webhook"
		log.Printf("[controller][user][CreateWebhookUrl][error] - could not fetch webhook url. Something unexpected happened : %v \n", whErr)
		return util.ErrorResponse(ctx, http.StatusConflict, whErr.Error(), "An unexpected error")
	}

	webhook, err := axios.Get(webhookUrl)
	if err != nil {
		log.Printf("[controller][user][CreateWebhookUrl] - error - webhook url is invalid: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Webhook url is invalid")
	}

	if webhook.Status != http.StatusOK {
		log.Printf("[controller][user][Create WebhookUrl] - error - response not ok: %v\n", webhook.Status)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Webhook url is invalid")
	}

	var VerifyWebhookResponse blueprint.WebhookVerificationResponse

	err = json.Unmarshal(webhook.Data, &VerifyWebhookResponse)

	log.Printf("[controller][user][CreateWebhookUrl] - webhook url is valid %s\n", string(webhook.Data))

	err = database.CreateUserWebhook(user.UUID.String(), webhookUrl, webhoookBody.VerifyToken)
	if err != nil {
		if errors.Is(err, blueprint.EALREADY_EXISTS) {
			log.Printf("[controller][user][CreateWebhookUrl] - error - user already has a webhook url")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "App already has a webhook url")
		}
		log.Printf("[controller][user][CreateWebhookUrl] - error creating webhook url %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err.Error(), "An unexpected error")
	}

	var response = map[string]interface{}{
		"url": webhookUrl,
	}

	log.Printf("[controller][user][CreateWebhookUrl] - created webhook url: '%s' for user %s.\n", webhookUrl, user.UUID.String())
	return util.SuccessResponse(ctx, http.StatusCreated, response)
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
		  "verify_token": "1234567890"
		}
	*/

	webhoookBody := struct {
		Url         string `json:"url"`
		VerifyToken string `json:"verify_token"`
	}{}
	err := json.Unmarshal(bod, &webhoookBody)
	if err != nil {
		log.Printf("[controller][user][UpdateWebhookUrl] - error unmarshalling webhook body %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "bad request", "The body of the request is invalid. Please check and try again.")
	}

	webhookUrl := webhoookBody.Url
	if webhookUrl == "" {
		log.Printf("[controller][user][UpdateWebhookUrl] - error - webhook url is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Webhook url is empty")
	}

	if webhoookBody.VerifyToken == "" {
		log.Printf("[controller][user][UpdateWebhookUrl] - error - webhook verify token is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Webhook verify token is empty")
	}

	log.Printf("[controller][user][UpdateWebhookUrl] - webhook passed is: '%s' \n", webhookUrl)

	database := db.NewDB{DB: w.DB}
	upErr := database.UpdateUserWebhook(user.UUID.String(), webhookUrl, webhoookBody.VerifyToken)
	if upErr != nil {
		if upErr == sql.ErrNoRows {
			log.Printf("[controller][user][UpdateWebhookUrl] - error - user does not have a webhook url")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "Webhook not found")
		}
		log.Printf("[controller][user][UpdateWebhookUrl] - error updating webhook url %s\n", upErr.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, upErr.Error(), "An unexpected error")
	}
	log.Printf("[controller][user][UpdateWebhookUrl] - updated webhook url: '%s' for user %s\n", webhookUrl, user.UUID)

	return util.SuccessResponse(ctx, http.StatusOK, webhoookBody)
}

// DeleteUserWebhookUrl deletes a webhook for a user
func (w *WebhookController) DeleteUserWebhookUrl(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][DeleteUserWebhookUrl] - deleting webhook url")
	user := ctx.Locals("user").(*blueprint.User)

	database := db.NewDB{DB: w.DB}
	upErr := database.DeleteUserWebhook(user.UUID.String())

	if upErr != nil {
		if errors.Is(upErr, sql.ErrNoRows) {
			log.Printf("[controller][user][DeleteUserWebhookUrl] - error - user does not have a webhook url")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "Webhook not found")
		}
		log.Printf("[controller][user][DeleteUserWebhookUrl] - error deleting webhook url %s\n", upErr.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, upErr.Error(), "An unexpected error")
	}

	log.Printf("[controller][user][DeleteUserWebhookUrl] - deleted webhook url for user %v\n", user)
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}

func (w *WebhookController) Verify(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][Verify] - verifying webhook")
	user := ctx.Locals("user").(*blueprint.User)
	// in order to verify a webhook, we send a request to the webhook url.
	//we expect a response with a status code of 200.
	// we expect the URL to contain the verify_token, which is the token they set in the webhook settings.

	database := db.NewDB{DB: w.DB}
	webhook, err := database.FetchWebhook(user.UUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[controller][user][Verify] - error - user does not have a webhook url")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "Webhook not found")
		}
		log.Printf("[controller][user][Verify] - error fetching webhook url %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err.Error(), "An unexpected error")
	}

	log.Printf("[controller][user][Verify] - webhook url is: '%s' \n", webhook.Url)
	// make a GET request to the webhook url
	res, err := axios.Get(webhook.Url)
	if err != nil {
		log.Printf("[controller][user][Verify] - error making GET request to webhook url %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err.Error(), "An unexpected error")
	}

	if res.Status != 200 {
		log.Printf("[controller][user][Verify] - error - webhook url returned status code %d\n", res.Status)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "unknown error", "An unexpected error")
	}

	log.Printf("[controller][user][Verify] - webhook response is %v\n", string(res.Data))
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}
