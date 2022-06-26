package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services"
	"orchdio/util"
)

// VerifyToken verifies a token and set the context local called "claim" to a type of *blueprint.OrchdioUserToken
func VerifyToken(ctx *fiber.Ctx) error {
	jwtToken := ctx.Locals("authToken").(*jwt.Token)
	claims := jwtToken.Claims.(*blueprint.OrchdioUserToken)
	ctx.Locals("claims", claims)
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

// ValidateKey validates that the key is valid
func ValidateKey(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")

	if len([]byte(apiKey)) > 36 {
		log.Printf("[middleware][ValidateKey] key is too long. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Key too long")
	}

	isValid := util.IsValidUUID(apiKey)

	if !isValid {
		log.Printf("[controller][user][Revoke] invalid key. Bad request %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid apikey")
	}

	log.Printf("[middleware][ValidateKey] API key is valid")
	return ctx.Next()
}
