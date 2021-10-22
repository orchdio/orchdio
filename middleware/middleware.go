package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"log"
	"net/http"
	"zoove/services"
	"zoove/types"
	"zoove/util"
)

// VerifyToken verifies a token and set the context local called "claim" to a type of *types.ZooveUserToken
func VerifyToken(ctx *fiber.Ctx) error {
	jwtToken := ctx.Locals("authToken").(*jwt.Token)
	claims := jwtToken.Claims.(*types.ZooveUserToken)
	ctx.Locals("claims", claims)
	return ctx.Next()
}

// ExtractLinkInfo fetches the extracted info about a link and save it into local context called "linkInfo"
func ExtractLinkInfo(ctx *fiber.Ctx) error {
	link := ctx.Query("link")
	if link == "" {
		log.Printf("\n[middleware][ExtractLinkInfo] warning - Link not detected. Skipping...\n")
		return ctx.Next()
	}
	linkInfo, err := services.ExtractLinkInfo(link)
	if err != nil {
		if err == types.EHOSTUNSUPPORTED {
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