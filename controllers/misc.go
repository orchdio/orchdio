package controllers

import (
	"github.com/gofiber/fiber/v2"
	"net/http"
	"zoove/blueprint"
	"zoove/util"
)

// LinkInfo returns the information about link that the information has been extracted from in the middleware
func LinkInfo(ctx *fiber.Ctx) error {
	local := ctx.Locals("linkInfo")
	if local == nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Link not passed.")
	}
	info := local.(*blueprint.LinkInfo)
	return util.SuccessResponse(ctx, http.StatusOK, info)
}
