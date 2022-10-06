package middleware

import (
	"database/sql"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services"
	"orchdio/util"
)

// VerifyToken verifies a token and set the context local called "claim" to a type of *blueprint.OrchdioUserToken
func VerifyToken(ctx *fiber.Ctx) error {
	log.Printf("[middleware][VerifyToken] method - Verifying token...\n")
	jwtToken := ctx.Locals("authToken").(*jwt.Token)
	claims := jwtToken.Claims.(*blueprint.OrchdioUserToken)
	ctx.Locals("claims", claims)
	log.Printf("[middleware][VerifyToken] method - Token verified. Claims set: %v\n", claims)
	return ctx.Next()
}

func ExtractLinkInfoFromBody(ctx *fiber.Ctx) error {
	linkBody := ctx.Body()

	link := map[string]string{}

	err := json.Unmarshal(linkBody, &link)
	if err != nil {
		log.Printf("[middleware][ExtractLinkInfoFromBody] error - Could not unmarshal link body: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	url := link["url"]

	if url == "" {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] warning - Link not detected. Skipping...\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Bad request. Check you're using the '?link' query string")
	}
	linkInfo, err := services.ExtractLinkInfo(url)

	if err != nil {
		if err == blueprint.EHOSTUNSUPPORTED {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, err)
		}

		if err == blueprint.EINVALIDLINK {
			log.Printf("[middleware][ExtractLinkInfoFromBody][warning] invalid link. are you sure its a url? %s\n", link)
			return util.ErrorResponse(ctx, http.StatusBadRequest, err)
		}

		log.Printf("\n[middleware][ExtractLinkInfoFromBody] error - Could not extract link info: %v: for link: %v\n", err, link)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	if linkInfo == nil {
		log.Printf("\n[middleware][ExtractLinkInfoFromBody] error - No linkInfo retrieved for link: %v: \n", link)
		return util.ErrorResponse(ctx, http.StatusNotFound, "Link info not found.")
	}

	log.Printf("\n[middleware][ExtractLinkInfoFromBody] method - Extracted link info is: %v\n", linkInfo)
	ctx.Locals("linkInfo", linkInfo)
	return ctx.Next()
}

// ExtractLinkInfo fetches the extracted info about a link and save it into local context called "linkInfo"
func ExtractLinkInfo(ctx *fiber.Ctx) error {
	link := ctx.Query("link")
	if link == "" {
		log.Printf("\n[middleware][ExtractLinkInfo] warning - Link not detected. Skipping...\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Bad request. Check you're using the '?link' query string")
	}
	linkInfo, err := services.ExtractLinkInfo(link)
	if err != nil {
		if err == blueprint.EHOSTUNSUPPORTED {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, err)
		}

		if err == blueprint.EINVALIDLINK {
			log.Printf("[middleware][ExtractLinkInfo][warning] invalid link. are you sure its a url? %s\n", link)
			return util.ErrorResponse(ctx, http.StatusBadRequest, err)
		}

		log.Printf("\n[middleware][ExtractLinkInfo] error - Could not extract link info: %v: for link: %v\n", err, link)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	if linkInfo == nil {
		log.Printf("\n[middleware][ExtractLinkInfo] error - No linkInfo retrieved for link: %v: \n", link)
		return util.ErrorResponse(ctx, http.StatusNotFound, "Link info not found.")
	}

	log.Printf("\n[middleware][ExtractLinkInfo] method - Extracted link info is: %v\n", linkInfo)
	ctx.Locals("linkInfo", linkInfo)
	return ctx.Next()
}

type AuthMiddleware struct {
	DB *sqlx.DB
}

func NewAuthMiddleware(db *sqlx.DB) *AuthMiddleware {
	return &AuthMiddleware{DB: db}
}

// ValidateKey validates that the key is valid
func (a *AuthMiddleware) ValidateKey(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")

	if len([]byte(apiKey)) > 36 {
		log.Printf("[middleware][ValidateKey] key is too long. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Key too long")
	}

	isValid := util.IsValidUUID(apiKey)

	if !isValid {
		log.Printf("[controller][user][Revoke] invalid key. Bad request %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid apikey")
	}

	// fetch the user from the database
	database := db.NewDB{DB: a.DB}

	user, err := database.FetchUserWithApiKey(apiKey)
	if err != nil {

		if err == sql.ErrNoRows {
			log.Printf("[middleware][ValidateKey] key not found. %s\n", apiKey)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid apikey")
		}

		log.Printf("[middleware][ValidateKey] error - Could not fetch user with api key: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	ctx.Locals("user", user)
	log.Printf("[middleware][ValidateKey] API key is valid")
	return ctx.Next()
}

func (a *AuthMiddleware) LogIncomingRequest(ctx *fiber.Ctx) error {
	log.Printf("[middleware][LogIncomingRequest] incoming request: %s  %s: %s\n", ctx.IP(), ctx.Method(), ctx.Path())
	return ctx.Next()
}

func (a *AuthMiddleware) AddAPIDeveloperToContext(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")

	if len([]byte(apiKey)) > 36 {
		log.Printf("[middleware][ValidateKey] key is too long. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Key too long")
	}

	if apiKey == "" {
		log.Printf("[middleware][ValidateKey] key is empty. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Key is empty")
	}

	isValid := util.IsValidUUID(apiKey)
	if isValid {
		log.Printf("[middleware][ValidateKey] key is valid. %s\n", apiKey)
		database := db.NewDB{DB: a.DB}
		user, err := database.FetchUserWithApiKey(apiKey)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("[middleware][ValidateKey] key not found. %s\n", apiKey)
				return util.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid apikey")
			}
			log.Printf("[middleware][ValidateKey] error - Could not fetch user with api key: %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}
		ctx.Locals("developer", user)
		return ctx.Next()
	}
	return util.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid apikey")
}
