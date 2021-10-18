package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"zoove/types"
)

// VerifyToken verifies a token and set the context local called "claim" to a type of *types.ZooveUserToken
func VerifyToken(ctx *fiber.Ctx) error {
	jwtToken := ctx.Locals("authToken").(*jwt.Token)
	claims := jwtToken.Claims.(*types.ZooveUserToken)
	ctx.Locals("claims", claims)
	return ctx.Next()
}
