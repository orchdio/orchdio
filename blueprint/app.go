package blueprint

import (
	"database/sql"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type AppKeys struct {
	PublicKey    string `json:"public_key,omitempty" db:"public_key"`
	SecretKey    string `json:"secret_key,omitempty" db:"secret_key"`
	VerifySecret string `json:"verify_secret,omitempty" db:"verify_token"`
	DeezerState  string `json:"deezer_state,omitempty" db:"deezer_state"`
}

type Organization struct {
	ID          int       `json:"id,omitempty" db:"id"`
	UID         uuid.UUID `json:"uid,omitempty" db:"uuid"`
	Name        string    `json:"name,omitempty" db:"name"`
	Description string    `json:"description,omitempty" db:"description"`
	CreatedAt   string    `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   string    `json:"updated_at,omitempty" db:"updated_at"`
	Owner       uuid.UUID `json:"owner,omitempty" db:"owner,omitempty"`
}

type UpdateOrganizationData struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type OrganizationResponse struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   string    `json:"created_at"`
	Owner       uuid.UUID `json:"owner"`
}

type UserApp struct {
	UUID         uuid.UUID      `json:"uuid" db:"uuid"`
	RefreshToken []byte         `json:"refreshToken" db:"refresh_token"`
	Scopes       pq.StringArray `json:"scopes" db:"scopes"`
	User         uuid.UUID      `json:"user" db:"user"`
	AuthedAt     string         `json:"authed_at" db:"authed_at"`
	LastAuthedAt string         `json:"last_authed_at" db:"last_authed_at"`
	App          uuid.UUID      `json:"app" db:"app"`
	Platform     string         `json:"platform" db:"platform"`
	Username     string         `json:"username" db:"username"`
	PlatformID   string         `json:"platform_id" db:"platform_id"`
}

type UserAccountInfo struct {
	Email     string `json:"email"`
	Usernames struct {
		Spotify string `json:"spotify,omitempty"`
		Deezer  string `json:"deezer,omitempty"`
		YtMusic string `json:"ytmusic,omitempty"`
	} `json:"usernames"`
	ProfilePictures struct {
		Spotify    string `json:"spotify,omitempty"`
		Deezer     string `json:"deezer,omitempty"`
		YtMusic    string `json:"ytmusic,omitempty"`
		AppleMusic string `json:"applemusic,omitempty"`
		Tidal      string `json:"tidal,omitempty"`
	} `json:"profile_pictures"`
	ExplicitContents struct {
		Spotify    bool `json:"spotify,omitempty"`
		Deezer     bool `json:"deezer,omitempty"`
		YtMusic    bool `json:"ytmusic,omitempty"`
		AppleMusic bool `json:"applemusic,omitempty"`
		Tidal      bool `json:"tidal,omitempty"`
	} `json:"explicit_contents"`
}

type UserPlatformInfo struct {
	Platform       string `json:"platform"`
	Username       string `json:"username"`
	ProfilePicture string `json:"profile_picture"`
	// at the moment, it seems the spotify library being used doesnt return the user
	// explicit content level. so for now, we set to false always for spotify and other platforms
	// that may have same issue
	ExplicitContent bool   `json:"explicit_content,omitempty"`
	Followers       int    `json:"followers,omitempty"`
	Following       int    `json:"following,omitempty"`
	PlatformID      string `json:"platform_id,omitempty"`
	PlatformSubPlan string `json:"subscription_plan,omitempty"`
	Url             string `json:"url,omitempty"`
}

type UserInfo struct {
	Email      string            `json:"email"`
	ID         string            `json:"id"`
	Deezer     *UserPlatformInfo `json:"deezer,omitempty"`
	Spotify    *UserPlatformInfo `json:"spotify,omitempty"`
	YtMusic    *UserPlatformInfo `json:"ytmusic,omitempty"`
	AppleMusic *UserPlatformInfo `json:"applemusic,omitempty"`
	Tidal      *UserPlatformInfo `json:"tidal,omitempty"`
}

type UserAppAndPlatformInfo struct {
	AppID        string         `json:"app_id" db:"app_id"`
	Platform     string         `json:"platform" db:"platform"`
	PlatformID   string         `json:"platform_id" db:"platform_id"`
	RefreshToken []byte         `json:"refresh_token" db:"refresh_token"`
	Username     string         `json:"username" db:"username"`
	Email        string         `json:"email" db:"email"`
	UserID       string         `json:"user_id" db:"user_id"`
	AccessToken  string         `json:"access_token" db:"access_token"`
	ExpiresIn    sql.NullString `json:"expires_in" db:"expires_in"`
}

type User struct {
	Email               string    `json:"email" db:"email"`
	ID                  int       `json:"id,omitempty" db:"id"`
	UUID                uuid.UUID `json:"uuid" db:"uuid"`
	CreatedAt           string    `json:"created_at" db:"created_at"`
	UpdatedAt           string    `json:"updated_at" db:"updated_at"`
	Password            string    `json:"password,omitempty" db:"password,omitempty"`
	ResetToken          string    `json:"reset_token,omitempty" db:"reset_token,omitempty"`
	ResetTokenExpiry    string    `json:"reset_token_expiry,omitempty" db:"reset_token_expiry,omitempty"`
	ResetTokenCreatedAt string    `json:"reset_token_created_at,omitempty" db:"reset_token_created_at,omitempty"`
}

type CreateOrganizationData struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	OwnerEmail    string `json:"owner_email"`
	OwnerPassword string `json:"owner_password"`
}

type CreateNewUserAppData struct {
	RefreshToken []byte    `json:"refreshToken"`
	Scopes       []string  `json:"scopes"`
	User         uuid.UUID `json:"user"`
	// todo: remove this. make it automatically use current timestamp. assignee: @jhym3s
	LastAuthedAt string    `json:"last_authed_at"`
	App          uuid.UUID `json:"app"`
	Platform     string    `json:"platform"`
	AccessToken  string    `json:"access_token"`
	// fixme: convert this to use a type of time.
	ExpiresIn string `json:"expires_in"`
}

type IntegrationCredentials struct {
	AppID           string `json:"app_id,omitempty"`
	AppSecret       string `json:"app_secret,omitempty"`
	AppRefreshToken string `json:"app_refresh_token,omitempty"`
	Platform        string `json:"app_platform,omitempty"`
}

type SrcTargetCredentials struct {
	Source IntegrationCredentials `json:"source"`
	Target IntegrationCredentials `json:"target"`
}

type AppInfo struct {
	AppID       string                   `json:"app_id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	RedirectURL string                   `json:"redirect_url"`
	WebhookURL  string                   `json:"webhook_url"`
	PublicKey   string                   `json:"public_key"`
	Authorized  bool                     `json:"authorized"`
	Credentials []IntegrationCredentials `json:"credentials"`
	// due to the weird nature of deezer auth, we add the deezer state for the app here
	// to be visible to the developer
	DeezerState string `json:"deezer_state,omitempty"`

	WebhookPortalURL string `json:"webhook_portal_url,omitempty"`
}

type AppTaskData struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

type Action struct {
	Payload interface{} `json:"payload"`
	// this is the action the developer was trying to do before auth
	// for example if its adding a playlist to account, it would be something like
	// "add_playlist_to_account"
	// TODO: define a list of actions and their keys.
	Action string `json:"action"`
}

type DeveloperApp struct {
	ID                    int       `json:"id,omitempty" db:"id"`
	UID                   uuid.UUID `json:"uid,omitempty" db:"uuid"`
	Name                  string    `json:"name,omitempty" db:"name"`
	Description           string    `json:"description,omitempty" db:"description"`
	Developer             uuid.UUID `json:"developer,omitempty" db:"developer"`
	SecretKey             []byte    `json:"secret_key,omitempty" db:"secret_key"`
	PublicKey             uuid.UUID `json:"public_key,omitempty" db:"public_key"`
	RedirectURL           string    `json:"redirect_url,omitempty" db:"redirect_url"`
	WebhookURL            string    `json:"webhook_url,omitempty" db:"webhook_url"`
	VerifyToken           []byte    `json:"verify_token,omitempty" db:"verify_token"`
	CreatedAt             string    `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt             string    `json:"updated_at,omitempty" db:"updated_at"`
	Authorized            bool      `json:"authorized,omitempty" db:"authorized"`
	Organization          string    `json:"organization,omitempty" db:"organization"`
	SpotifyCredentials    []byte    `json:"spotify_credentials,omitempty" db:"spotify_credentials"`
	AppleMusicCredentials []byte    `json:"applemusic_credentials,omitempty" db:"applemusic_credentials"`
	DeezerCredentials     []byte    `json:"deezer_credentials,omitempty" db:"deezer_credentials"`
	TidalCredentials      []byte    `json:"tidal_credentials,omitempty" db:"tidal_credentials"`
	DeezerState           string    `json:"deezer_state,omitempty" db:"deezer_state,omitempty"`
	WebhookAppID          string    `json:"webhook_app_id,omitempty" db:"webhook_app_id,omitempty"`
}

type UpdateDeveloperAppData struct {
	Name                string `json:"name,omitempty"`
	Description         string `json:"description,omitempty"`
	RedirectURL         string `json:"integration_redirect_url,omitempty"`
	IntegrationPlatform string `json:"platform,omitempty"`
	WebhookURL          string `json:"webhook_url,omitempty"`
	// for apple music, this is TEAM_ID
	IntegrationAppID string `json:"integration_app_id,omitempty"`
	// for apple music, this is KEY_ID
	IntegrationAppSecret string `json:"integration_app_secret,omitempty"`
	// for apple music, this is API_KEY
	// for TIDAL, this is the Refresh MusicToken
	IntegrationRefreshToken string `json:"integration_refresh_token,omitempty"`
}

type CreateNewDeveloperAppData struct {
	Name                 string `json:"name"`
	Description          string `json:"description"`
	WebhookURL           string `json:"webhook_url"`
	Organization         string `json:"organization_id"`
	IntegrationAppSecret string `json:"integration_app_secret"`
	IntegrationAppId     string `json:"integration_app_id"`
	RedirectURL          string `json:"integration_redirect_url"`
	IntegrationPlatform  string `json:"platform"`
}

// ControllerError represents a valid error response
type ControllerError struct {
	Message string      `json:"message"`
	Status  int         `json:"status"`
	Error   interface{} `json:"error,omitempty"`
}

// ControllerResult represents a valid success response
type ControllerResult struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Status  int         `json:"status"`
}

// ApiKey represents an API key record
type ApiKey struct {
	ID        int       `json:"id" db:"id"`
	Key       uuid.UUID `json:"key" db:"key"`
	User      uuid.UUID `json:"user" db:"user"`
	Revoked   bool      `json:"revoked" db:"revoked"`
	CreatedAt string    `json:"created_at" db:"created_at"`
	UpdatedAt string    `json:"updated_at" db:"updated_at"`
}

// WebhookMessage struct represents the message sent from the server to the client over webhook
type WebhookMessage struct {
	Message string      `json:"message"`
	Event   string      `json:"event_name"`
	Payload interface{} `json:"payload,omitempty"`
}

// Webhook represents a webhook record in the db
type Webhook struct {
	Id          int       `json:"id" db:"id"`
	User        uuid.UUID `json:"user" db:"user"`
	Url         string    `json:"url" db:"url"`
	CreatedAt   string    `json:"created_at" db:"created_at"`
	UpdatedAt   string    `json:"updated_at" db:"updated_at"`
	VerifyToken string    `json:"verify_token" db:"verify_token"`
	UID         uuid.UUID `json:"uuid" db:"uuid"`
}

type CreateNewDevAppResponse struct {
	AppId string `json:"app_id"`
}
