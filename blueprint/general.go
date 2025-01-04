package blueprint

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var DeezerHost = []string{"deezer.page.link", "www.deezer.com"}

const (
	SpotifyHost    = "open.spotify.com"
	TidalHost      = "tidal.com"
	YoutubeHost    = "music.youtube.com"
	AppleMusicHost = "music.apple.com"
)

const (
	EmailQueueTaskTypePattern         = "send_appauth_email"
	PlaylistConversionTaskTypePattern = "playlist_conversion_"
	SendResetPasswordTaskPattern      = "send_reset_password_email"
	SendWelcomeEmailTaskPattern       = "send_welcome_email"
)

const (
	PlaylistConversionQueueName = "playlist_conversion"
	EmailQueueName              = "email"
	DefaultQueueName            = "default"
)

// PlaylistConversionMetadataEvent is the event emitted when the meta of a playlist conversion is done.
// it uses lowercase snake_case because svix, the webhook service used does not allow
// : as delimiter
var (
	PlaylistConversionMetadataEvent = "playlist_conversion_metadata"
	PlaylistConversionTrackEvent    = "playlist_conversion_track"
	PlaylistConversionDoneEvent     = "playlist_conversion_done"
)

const (
	SecretKeyType   = "secret"
	VerifyKeyType   = "verify"
	PublicKeyType   = "public"
	DeezerStateType = "deezer_state"
)

var ValidUserIdentifiers = []string{"email", "id"}

// perhaps have a different Error type declarations somewhere. For now, be here
var (
	ErrHostUnsupported    = errors.New("ErrHostUnsupported")
	EnoResult             = errors.New("Not Found")
	ErrNotImplemented     = errors.New("NOT_IMPLEMENTED")
	EGENERAL              = errors.New("EGENERAL")
	ErrUnknown            = errors.New("ErrUnknown")
	ErrInvalidLink        = errors.New("invalid link")
	EalreadyExists        = errors.New("already exists")
	ErrPhantomErr         = errors.New("unexpected error")
	ErrTooMany            = errors.New("too many parameters")
	ErrForbidden          = errors.New("403 Forbidden")
	ErrUnAuthorized       = errors.New("401 Unauthorized")
	ErrBadRequest         = errors.New("400 Bad Request")
	ErrInvalidAuthCode    = errors.New("INVALID_AUTH_CODE")
	ErrCredentialsMissing = errors.New("credentials not added for platform")
	ErrInvalidPermissions = errors.New("invalid permissions")
	ErrServiceClosed      = errors.New("service closed")
	ErrInvalidPlatform    = errors.New("invalid platform")
	ErrNoCredentials      = errors.New("no credentials")
	ErrBadCredentials     = errors.New("bad credentials")

	// possible auth errors from each of the streaming platforms

	ErrSpotifyUserNotRegistered = "User not registered"
	ErrSpotifyInvalidGrant      = "invalid_grant"
	ErrSpotifyInvalidClient     = "invalid_client"
	ErrSpotifyInvalidRequest    = "invalid_request"

	ErrDeezerAccessDenied = "access_denied"
	ErrFreeServiceClosed  = "free service is closed"
)

const (
	TASK_STATUS_COMPLETED = "completed"
	TASK_STATUS_FAILED    = "failed"
)

type UserProfile struct {
	Email     string      `json:"email" db:"email"`
	Usernames interface{} `json:"usernames" db:"usernames"`
	UUID      uuid.UUID   `json:"uuid" db:"uuid"`
	CreatedAt string      `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt string      `json:"updated_at,omitempty" db:"updated_at"`
}

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

type SpotifyUser struct {
	Name      string   `json:"name,omitempty"`
	Moniker   string   `json:"moniker"`
	Platforms []string `json:"platforms"`
	Email     string   `json:"email"`
}

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

type ConversionBody struct {
	URL            string `json:"url"`
	TargetPlatform string `json:"target_platform,omitempty"`
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

// TaskRecord representsUs a task record in the database
type TaskRecord struct {
	Id         int       `json:"id,omitempty" db:"id"`
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
	EntityURL   string      `json:"entity_url,omitempty" db:"entity_url"`
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
