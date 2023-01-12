package middleware

import (
	"encoding/json"
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
	log.Printf("[middleware][VerifyToken] method - Verifying token...\n")
	jt := ctx.Locals("authToken")
	if jt == nil {
		log.Printf("[middlware][VerifyToken] method - JWT header missing")
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "JWT header is missing")
	}
	jwtToken := jt.(*jwt.Token)
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
