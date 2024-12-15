package blueprint

import (
	"errors"
	"github.com/google/uuid"
	"time"
)

var DeezerHost = []string{"deezer.page.link", "www.deezer.com"}

const (
	SpotifyHost    = "open.spotify.com"
	TidalHost      = "tidal.com"
	YoutubeHost    = "music.youtube.com"
	AppleMusicHost = "music.apple.com"
)

const (
	EMAIL_QUEUE_PATTERN               = "send:appauth:email"
	PLAYLIST_CONVERSION_QUEUE_PATTERN = "playlist:conversion"
	SEND_RESET_PASSWIRD_QUEUE_PATTERN = "send:reset_password_email"
	SEND_WELCOME_EMAIL_QUEUE_PATTER   = "send:welcome_email"
)

var ValidUserIdentifiers = []string{"email", "id"}

// perhaps have a different Error type declarations somewhere. For now, be here
var (
	EHOSTUNSUPPORTED    = errors.New("EHOSTUNSUPPORTED")
	ENORESULT           = errors.New("Not Found")
	ENOTIMPLEMENTED     = errors.New("NOT_IMPLEMENTED")
	EGENERAL            = errors.New("EGENERAL")
	EUNKNOWN            = errors.New("EUNKNOWN")
	EINVALIDLINK        = errors.New("invalid link")
	EALREADY_EXISTS     = errors.New("already exists")
	EPHANTOMERR         = errors.New("unexpected error")
	ERRTOOMANY          = errors.New("too many parameters")
	EFORBIDDEN          = errors.New("403 Forbidden")
	EUNAUTHORIZED       = errors.New("401 Unauthorized")
	EBADREQUEST         = errors.New("400 Bad Request")
	EINVALIDAUTHCODE    = errors.New("INVALID_AUTH_CODE")
	ECREDENTIALSMISSING = errors.New("credentials not added for platform")
	EINVALIDPERMISSIONS = errors.New("invalid permissions")
	ESERVICECLOSED      = errors.New("service closed")
	EINVALIDPLATFORM    = errors.New("invalid platform")
	ENOCREDENTIALS      = errors.New("no credentials")
	EBADCREDENTIALS     = errors.New("bad credentials")
)

var (
	EEDESERIALIZE        = "EVENT_DESERIALIZE_MESSAGE_ERROR"
	EEPLAYLISTCONVERSION = "playlist:conversion"
)

type UserProfile struct {
	Email     string      `json:"email" db:"email"`
	Usernames interface{} `json:"usernames" db:"usernames"`
	UUID      uuid.UUID   `json:"uuid" db:"uuid"`
	CreatedAt string      `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt string      `json:"updated_at,omitempty" db:"updated_at"`
}

//type AppleMusicAuthBody struct {
//	Authorization struct {
//		Code    string `json:"code"`
//		IdToken string `json:"id_token"`
//		State   string `json:"state"`
//	} `json:"authorization"`
//}

type AppleMusicAuthBody struct {
	MusicToken    string `json:"token"`
	Email         string `json:"email"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	State         string `json:"state"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	App           string `json:"app,omitempty"`
}

// swagger:response redirectAuthResponse
type ErrorResponse struct {
	// Description: The message attached to the response.
	//
	// Required: true
	//
	// Example: "This is a message about whatever i can tell you about the error"
	Message string `json:"message"`
	// Description: The error code attached to the response. This will return 200 (or 201), depending on the endpoint. It returns 4xx - 5xx as suitable, otherwise.
	//
	// Required: true
	//
	// Example: 201
	Status int         `json:"status"`
	Error  interface{} `json:"error"`
}

type (
	SpotifyUser struct {
		Name      string   `json:"name,omitempty"`
		Moniker   string   `json:"moniker"`
		Platforms []string `json:"platforms"`
		Email     string   `json:"email"`
	}
)

// LinkInfo represents the metadata about a link user wants to convert
type LinkInfo struct {
	Platform       string `json:"platform"`
	TargetLink     string `json:"target_link"`
	Entity         string `json:"entity"`
	EntityID       string `json:"entity_id"`
	TargetPlatform string `json:"target_platform,omitempty"`
	App            string `json:"app,omitempty"`
	Developer      string `json:"developer,omitempty"`
}

type Pagination struct {
	Next     string `json:"next"`
	Previous string `json:"previous"`
	Total    int    `json:"total,omitempty"`
	Platform string `json:"platform"`
}

// Conversion represents the final response for a typical track conversion
type Conversion struct {
	Entity    string `json:"entity"`
	Platforms struct {
		Deezer     *TrackSearchResult `json:"deezer,omitempty"`
		Spotify    *TrackSearchResult `json:"spotify,omitempty"`
		Tidal      *TrackSearchResult `json:"tidal,omitempty"`
		YTMusic    *TrackSearchResult `json:"ytmusic,omitempty"`
		AppleMusic *TrackSearchResult `json:"applemusic,omitempty"`
	} `json:"platforms"`
	ShortURL string `json:"short_url,omitempty"`
}

type NewTask struct {
	ID string `json:"task_id"`
}

type TaskResponse struct {
	ID      string      `json:"task_id,omitempty"`
	Payload interface{} `json:"payload"`
	Status  string      `json:"status,omitempty"`
}

type TaskErrorPayload struct {
	Platform string `json:"platform"`
	Status   string `json:"status"`
	Error    string `json:"error"`
	Message  string `json:"message"`
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

// TaskRecord representsUs a task record in the database
type TaskRecord struct {
	Id int `json:"id,omitempty" db:"id"`
	//User       uuid.UUID `json:"user,omitempty" db:"user"`
	UID        uuid.UUID `json:"uid,omitempty" db:"uuid"`
	CreatedAt  time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at,omitempty" db:"updated_at"`
	Result     string    `json:"result,omitempty" db:"result"`
	Status     string    `json:"status,omitempty" db:"status"`
	EntityID   string    `json:"entity_id,omitempty" db:"entity_id"`
	Type       string    `json:"type,omitempty" db:"type"`
	RetryCount int       `json:"retry_count,omitempty" db:"retry_count"`
	App        string    `json:"app,omitempty" db:"app"`
}

type FollowTask struct {
	Id          int         `json:"id,omitempty" db:"id"`
	User        uuid.UUID   `json:"user,omitempty" db:"user"`
	CreatedAt   time.Time   `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at,omitempty" db:"updated_at"`
	UID         uuid.UUID   `json:"uid,omitempty" db:"uuid"`
	Task        uuid.UUID   `json:"task,omitempty" db:"task"`
	Subscribers interface{} `json:"subscribers,omitempty" db:"subscribers"`
	EntityID    string      `json:"entity_id,omitempty" db:"entity_id"`
	Developer   string      `json:"developer,omitempty" db:"developer"`
	EntityURL   string      `json:"entity_url,omitempty" db:"entity_url"`
	Status      string      `json:"status,omitempty" db:"status"`
}

type FollowData struct {
	User uuid.UUID `json:"user"`
}

type FollowsToProcess struct {
	ID          int         `json:"id,omitempty" db:"id"`
	UID         uuid.UUID   `json:"uid,omitempty" db:"uuid"`
	EntityID    string      `json:"entity_id,omitempty" db:"entity_id"`
	CreatedAt   time.Time   `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at,omitempty" db:"updated_at"`
	Developer   uuid.UUID   `json:"user,omitempty" db:"developer"`
	App         uuid.UUID   `json:"app,omitempty" db:"app"`
	Subscribers interface{} `json:"subscribers,omitempty" db:"subscribers"`
	//Result    interface{} `json:"result,omitempty" db:"result"`
	//Type      string      `json:"type,omitempty" db:"type"`
	EntityURL string `json:"entity_url,omitempty" db:"entity_url"`
}

type FollowTaskData struct {
	User      uuid.UUID `json:"user"`
	Url       string    `json:"url"`
	EntityID  string    `json:"entity_id"`
	Platform  string    `json:"platform"`
	App       string    `json:"app"`
	Developer string    `json:"developer,omitempty"`
	//Subscribers interface{} `json:"subscribers"`
}

type WebhookVerificationResponse struct {
	VerifyToken     string `json:"verify_token"`
	VerifyChallenge string `json:"verify_challenge"`
}

type AddToWaitlistBody struct {
	Email    string `json:"email"`
	Platform string `json:"platform"`
}

type EmailTaskData struct {
	App        *AppTaskData           `json:"app"`
	From       string                 `json:"from"`
	To         string                 `json:"to"`
	Payload    map[string]interface{} `json:"payload"`
	TaskID     string                 `json:"task_id"`
	TemplateID int                    `json:"template_id"`
	Subject    string                 `json:"subject,omitempty"`
}

type ExtractedTitleInfo struct {
	Artists []string `json:"artists"`
	Title   string   `json:"title"`
}

type OrchdioLoggerOptions struct {
	Component            string      `json:"component"`
	RequestID            string      `json:"request_id"`
	Timestamp            string      `json:"timestamp"`
	ApplicationPublicKey string      `json:"application_public_key"`
	AppID                string      `json:"app_id"`
	Platform             string      `json:"platform"`
	Entity               interface{} `json:"entity"`
	Error                interface{} `json:"error"`
	Message              string      `json:"message"`
	AddTrace             bool        `json:"add_trace"`
}

//// DeveloperAppWithUserApp is similar to ```DeveloperApp``` but includes the user app id and other user app info
//type DeveloperAppWithUserApp struct {
//	ID                    int            `json:"id,omitempty" db:"id"`
//	UID                   uuid.UUID      `json:"uid,omitempty" db:"uuid"`
//	Name                  string         `json:"name,omitempty" db:"name"`
//	Description           string         `json:"description,omitempty" db:"description"`
//	Developer             uuid.UUID      `json:"developer,omitempty" db:"developer"`
//	SecretKey             []byte         `json:"secret_key,omitempty" db:"secret_key"`
//	PublicKey             uuid.UUID      `json:"public_key,omitempty" db:"public_key"`
//	RedirectURL           string         `json:"redirect_url,omitempty" db:"redirect_url"`
//	WebhookURL            string         `json:"webhook_url,omitempty" db:"webhook_url"`
//	VerifyToken           []byte         `json:"verify_token,omitempty" db:"verify_token"`
//	CreatedAt             string         `json:"created_at,omitempty" db:"created_at"`
//	UpdatedAt             string         `json:"updated_at,omitempty" db:"updated_at"`
//	Authorized            bool           `json:"authorized,omitempty" db:"authorized"`
//	Organization          string         `json:"organization,omitempty" db:"organization"`
//	SpotifyCredentials    []byte         `json:"spotify_credentials,omitempty" db:"spotify_credentials"`
//	AppleMusicCredentials []byte         `json:"applemusic_credentials,omitempty" db:"applemusic_credentials"`
//	DeezerCredentials     []byte         `json:"deezer_credentials,omitempty" db:"deezer_credentials"`
//	TidalCredentials      []byte         `json:"tidal_credentials,omitempty" db:"tidal_credentials"`
//	DeezerState           string         `json:"deezer_state,omitempty" db:"deezer_state,omitempty"`
//	UserAppID             string         `json:"user_app_id,omitempty" db:"user_app_id"`
//	Scopes                pq.StringArray `json:"scopes,omitempty" db:"scopes"`
//}
