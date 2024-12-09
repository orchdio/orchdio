package blueprint

import (
	"errors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
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

// MorbinTime because "its morbin time"
type MorbinTime string

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

// OrchdioUserToken represents a parsed user JWT claim
type OrchdioUserToken struct {
	jwt.RegisteredClaims
	Email    string    `json:"email"`
	Username string    `json:"username"`
	UUID     uuid.UUID `json:"uuid"`
	Platform string    `json:"platform"`
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

// TrackSearchResult represents a single search result for a platform.
// It represents what a single platform should return when trying to
// convert a link.
type TrackSearchResult struct {
	URL           string   `json:"url"`
	Artists       []string `json:"artists"`
	Released      string   `json:"release_date,omitempty"`
	Duration      string   `json:"duration"`
	DurationMilli int      `json:"duration_milli,omitempty"`
	Explicit      bool     `json:"explicit"`
	Title         string   `json:"title"`
	Preview       string   `json:"preview"`
	Album         string   `json:"album,omitempty"`
	ID            string   `json:"id"`
	Cover         string   `json:"cover"`
}

type Pagination struct {
	Next     string `json:"next"`
	Previous string `json:"previous"`
	Total    int    `json:"total,omitempty"`
	Platform string `json:"platform"`
}

// PlaylistSearchResult represents a single playlist result for a platform.
type PlaylistSearchResult struct {
	Title   string              `json:"title"`
	Tracks  []TrackSearchResult `json:"tracks"`
	URL     string              `json:"url"`
	Length  string              `json:"length,omitempty"`
	Preview string              `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
	Owner   string              `json:"owner,omitempty"`
	Cover   string              `json:"cover"`
}

// PlatformSearchTrack represents the key-value parameter passed
// when trying to convert playlist from spotify
type PlatformSearchTrack struct {
	Artistes []string `json:"artist"`
	Title    string   `json:"title"`
	ID       string   `json:"id"`
	URL      string   `json:"url"`
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

type PlatformPlaylistTrackResult struct {
	Tracks        *[]TrackSearchResult `json:"tracks"`
	Length        int                  `json:"length"`
	OmittedTracks *[]OmittedTracks     `json:"empty_tracks,omitempty"`
}

// PlaylistConversion represents the final response for a typical playlist conversion
type PlaylistConversion struct {
	Platforms struct {
		Deezer     *PlatformPlaylistTrackResult `json:"deezer,omitempty"`
		Spotify    *PlatformPlaylistTrackResult `json:"spotify,omitempty"`
		Tidal      *PlatformPlaylistTrackResult `json:"tidal,omitempty"`
		AppleMusic *PlatformPlaylistTrackResult `json:"applemusic,omitempty"`
	} `json:"platforms,omitempty"`
	Meta struct {
		Length   string `json:"length"`
		Title    string `json:"title"`
		Preview  string `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
		Owner    string `json:"owner"`
		Cover    string `json:"cover"`
		Entity   string `json:"entity"`
		URL      string `json:"url"`
		ShortURL string `json:"short_url,omitempty"`
	} `json:"meta,omitempty"`
}

type TrackConversion struct {
	Entity    string `json:"entity"`
	Platforms struct {
		Deezer     TrackSearchResult `json:"deezer"`
		Spotify    TrackSearchResult `json:"spotify"`
		Tidal      TrackSearchResult `json:"tidal"`
		Ytmusic    TrackSearchResult `json:"ytmusic"`
		Applemusic TrackSearchResult `json:"applemusic"`
	} `json:"platforms"`
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

// Message represents a message sent from the client to the server over websocket
type Message struct {
	Link string `json:"link"`
	// TODO: investigate if i could just use interface{} here.
	Attributes struct{} `json:"attributes"`
	EventName  string   `json:"event_name"`
}

// OmittedTracks represents tracks that could not be processed in a playlist, for whatever reason
type OmittedTracks struct {
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Artistes []string `json:"artistes"`
}

// WebsocketErrorMessage represents the error message sent from the server to the client over websocket
type WebsocketErrorMessage struct {
	Message   string      `json:"message"`
	Error     string      `json:"error"`
	EventName string      `json:"event_name"`
	Payload   interface{} `json:"payload,omitempty"`
}

// WebsocketMessage represents the message sent from the server to the client over websocket
type WebsocketMessage struct {
	Message string      `json:"message"`
	Event   string      `json:"event_name"`
	Payload interface{} `json:"payload,omitempty"`
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

// ApiKey represents an API key record
type ApiKey struct {
	ID        int       `json:"id" db:"id"`
	Key       uuid.UUID `json:"key" db:"key"`
	User      uuid.UUID `json:"user" db:"user"`
	Revoked   bool      `json:"revoked" db:"revoked"`
	CreatedAt string    `json:"created_at" db:"created_at"`
	UpdatedAt string    `json:"updated_at" db:"updated_at"`
}

// PlaylistTaskData represents the payload of a playlist task
type PlaylistTaskData struct {
	LinkInfo *LinkInfo     `json:"link_info"`
	App      *DeveloperApp `json:"app"`
	TaskID   string        `json:"task_id"`
	ShortURL string        `json:"short_url"`
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

type PlaylistFollow struct {
	ID        int       `json:"id,omitempty" db:"id"`
	UID       uuid.UUID `json:"uid,omitempty" db:"uuid"`
	EntityID  uuid.UUID `json:"entity_id,omitempty" db:"entity_id"`
	CreatedAt time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty" db:"updated_at"`
	User      uuid.UUID `json:"user,omitempty" db:"user"`
	Status    string    `json:"status,omitempty" db:"status"`
	// an array of subscribers
	Result []types.JSONText `json:"result,omitempty" db:"result"`
	Type   string           `json:"type,omitempty" db:"type"`
	App    string           `json:"app,omitempty" db:"app"`
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

type AddPlaylistToAccountData struct {
	// TODO: in the future, perhaps look into the viability of allowing multiple users and also support email and id in api for user id
	User uuid.UUID `json:"user"`
	Url  string    `json:"url"`
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
	Name        string `json:"name"`
	Description string `json:"description"`
	//RedirectURL            string `json:"redirect_url"`
	WebhookURL           string `json:"webhook_url"`
	Organization         string `json:"organization_id"`
	IntegrationAppSecret string `json:"integration_app_secret"`
	IntegrationAppId     string `json:"integration_app_id"`
	RedirectURL          string `json:"integration_redirect_url"`
	IntegrationPlatform  string `json:"platform"`
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

type Action struct {
	Payload interface{} `json:"payload"`
	// this is the action the developer was trying to do before auth
	// for example if its adding a playlist to account, it would be something like
	// "add_playlist_to_account"
	// TODO: define a list of actions and their keys.
	Action string `json:"action"`
}

type AppKeys struct {
	PublicKey    string `json:"public_key,omitempty" db:"public_key"`
	SecretKey    string `json:"secret_key,omitempty" db:"secret_key"`
	VerifySecret string `json:"verify_secret,omitempty" db:"verify_token"`
	DeezerState  string `json:"deezer_state,omitempty" db:"deezer_state"`
}

type AddToWaitlistBody struct {
	Email    string `json:"email"`
	Platform string `json:"platform"`
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
	// due to the weird nature of deezer auth, we add the deezerstate for the app here
	// to be visible to the developer
	DeezerState string `json:"deezer_state,omitempty"`
}

type UserPlaylist struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Duration      string `json:"duration,omitempty"`
	DurationMilis int    `json:"duration_millis,omitempty"`
	Public        bool   `json:"public"`
	Collaborative bool   `json:"collaborative"`
	NbTracks      int    `json:"nb_tracks,omitempty"`
	Fans          int    `json:"fans,omitempty"`
	URL           string `json:"url"`
	Cover         string `json:"cover"`
	CreatedAt     string `json:"created_at"`
	Checksum      string `json:"checksum,omitempty"`
	// use the name as the owner for now
	Owner string `json:"owner"`
}

type UserLibraryPlaylists struct {
	Total   int            `json:"total"`
	Payload []UserPlaylist `json:"data"`
}

type AppTaskData struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
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

type UserArtist struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Cover string `json:"cover"`
	URL   string `json:"url"`
}

type LibraryAlbum struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	ReleaseDate string   `json:"release_date"`
	Explicit    bool     `json:"explicit"`
	TrackCount  int      `json:"nb_tracks"`
	Artists     []string `json:"artists"`
	Cover       string   `json:"cover"`
}

type UserLibraryAlbums struct {
	Payload []LibraryAlbum `json:"payload"`
	Total   int            `json:"total"`
}

type UserLibraryArtists struct {
	Payload []UserArtist `json:"payload"`
	Total   int          `json:"total"`
}

type AuthMiddlewareUserInfo struct {
	Platform     string `json:"platform"`
	PlatformID   string `json:"platform_ids"`
	RefreshToken string `json:"refresh_token"`
}

type CreateOrganizationData struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	OwnerEmail    string `json:"owner_email"`
	OwnerPassword string `json:"owner_password"`
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
	// TODO: i may need to JOIN the users table to get the owner name and other stuff
	Owner uuid.UUID `json:"owner"`
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

type ExtractedTitleInfo struct {
	Artists []string `json:"artists"`
	Title   string   `json:"title"`
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

type CreateNewUserAppData struct {
	//AccessToken  []byte      `json:"accessToken"`
	RefreshToken []byte    `json:"refreshToken"`
	Scopes       []string  `json:"scopes"`
	User         uuid.UUID `json:"user"`
	// todo: remove this. make it automatically use current timestamp. assignee: @jhym3s
	LastAuthedAt string    `json:"last_authed_at"`
	App          uuid.UUID `json:"app"`
	Platform     string    `json:"platform"`
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
	AppID        string `json:"app_id" db:"app_id"`
	Platform     string `json:"platform" db:"platform"`
	PlatformID   string `json:"platform_id" db:"platform_id"`
	RefreshToken []byte `json:"refresh_token" db:"refresh_token"`
	Username     string `json:"username" db:"username"`
	Email        string `json:"email" db:"email"`
	UserID       string `json:"user_id" db:"user_id"`
}

type User struct {
	Email string `json:"email" db:"email"`
	//Usernames interface{} `json:"usernames" db:"usernames"`
	//Username     string      `json:"username,omitempty" db:"username"`
	ID                  int       `json:"id,omitempty" db:"id"`
	UUID                uuid.UUID `json:"uuid" db:"uuid"`
	CreatedAt           string    `json:"created_at" db:"created_at"`
	UpdatedAt           string    `json:"updated_at" db:"updated_at"`
	Password            string    `json:"password,omitempty" db:"password,omitempty"`
	ResetToken          string    `json:"reset_token,omitempty" db:"reset_token,omitempty"`
	ResetTokenExpiry    string    `json:"reset_token_expiry,omitempty" db:"reset_token_expiry,omitempty"`
	ResetTokenCreatedAt string    `json:"reset_token_created_at,omitempty" db:"reset_token_created_at,omitempty"`
	//RefreshToken []byte      `json:"refresh_token" db:"refresh_token,omitempty"`
	//PlatformID  string      `json:"platform_id" db:"platform_id"`
	//Authorized  bool        `json:"authorized,omitempty" db:"authorized,omitempty"`
	//PlatformIDs interface{} `json:"platform_ids,omitempty" db:"platform_ids,omitempty"`
}

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
