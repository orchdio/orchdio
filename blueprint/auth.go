package blueprint

import (
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

type LoginToOrgData struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginOrgToken struct {
	jwt.RegisteredClaims
	Description string     `json:"description"`
	Name        string     `json:"name"`
	OrgID       string     `json:"org_id"`
	Apps        *[]AppInfo `json:"apps"`
}

// AppJWT is the JWT token for orchdio app endpoint auths.
type AppJWT struct {
	jwt.RegisteredClaims
	DeveloperID string `json:"developer_id"`
	OrgID       string `json:"organization_id"`
}

type AuthMiddlewareUserInfo struct {
	Platform     string `json:"platform"`
	PlatformID   string `json:"platform_id"`
	RefreshToken string `json:"refresh_token"`
}

// AppAuthToken is the token generated after a user tries to authorize an app. This is the one passed to the state in the platform's redirect URL for plaforms
// that support persisting state param in final auth redirect.
// This is NOT the same as AppJWT that is the jwt for orchdio developer apps endpoints.
type AppAuthToken struct {
	jwt.RegisteredClaims
	App         string   `json:"app_id"`
	RedirectURL string   `json:"redirect_url"`
	Platform    string   `json:"platform"`
	Action      Action   `json:"action,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

// OrchdioUserToken represents a parsed user JWT claim
type OrchdioUserToken struct {
	jwt.RegisteredClaims
	Email              string                `json:"email"`
	Username           string                `json:"username"`
	UUID               uuid.UUID             `json:"uuid"`
	Platforms          []OrchdioUserAppsInfo `json:"platforms"`
	LastAuthedPlatform string                `json:"last_authed_platform"`
}

// OrchdioUserAppsInfo represents a user's single app info. A user app is whatever platform the user has authorized for a specific app
// It is what we send as part of the user's token after authorization.
type OrchdioUserAppsInfo struct {
	AppID      string `json:"app_id" db:"app_id"`
	Platform   string `json:"platform" db:"platform"`
	PlatformId string `json:"platform_id" db:"platform_id"`
	Username   string `json:"username" db:"username"`
}

type OrchdioOrgCreateResponse struct {
	OrgID       string `json:"org_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Token       string `json:"token"`
}

type OrchdioLoginUserResponse struct {
	OrgID       string     `json:"org_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Token       string     `json:"token"`
	Apps        *[]AppInfo `json:"apps"`
}

type UserAuthInfoForRequests struct {
	RefreshToken string
	AccessToken  string
	ExpiresIn    string
	Platform     string
	UserID       string
	AppID        string
	UserAppID    string
}
