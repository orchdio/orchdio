package types

import (
	"errors"
	"github.com/golang-jwt/jwt/v4"
)

const (
	DeezerHost  = "www.deezer.com"
	SpotifyHost = "open.spotify.com"
)

// perhaps have a different Error type declarations somewhere. For now, be here

var (
	EHOSTUNSUPPORTED = errors.New("EHOSTUNSUPPORTED")
)

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

type LinkInfo struct {
	Platform   string `json:"platform"`
	TargetLink string `json:"target_link"`
	Entity     string `json:"entity"`
	EntityID   string `json:"entity_id"`
}
