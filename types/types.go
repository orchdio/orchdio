package types

import "github.com/golang-jwt/jwt/v4"

type (
	SpotifyUser struct {
		Name      string   `json:"name,omitempty"`
		Moniker   string   `json:"moniker"`
		Platforms []string `json:"platforms"`
		Email     string   `json:"email"`
	}
)

type ControllerError struct {
	Message string      `json:"message"`
	Status  int         `json:"status"`
	Error   interface{} `json:"error,omitempty"`
}

type ControllerResult struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Status  int         `json:"status"`
}

type ZooveUserToken struct {
	jwt.StandardClaims
	Platform   string `json:"platform"`
	PlatformID string `json:"platform_id"`
	Email      string `json:"email"`
	Role       string `json:"role"`
}
