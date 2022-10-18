package blueprint

import (
	"errors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx/types"
	"time"
)

var DeezerHost = []string{"deezer.page.link", "www.deezer.com"}

const (
	SpotifyHost    = "open.spotify.com"
	TidalHost      = "tidal.com"
	YoutubeHost    = "music.youtube.com"
	AppleMusicHost = "music.apple.com"
)

// perhaps have a different Error type declarations somewhere. For now, be here

var (
	EHOSTUNSUPPORTED = errors.New("EHOSTUNSUPPORTED")
	ENORESULT        = errors.New("ENORESULT")
	ENOTIMPLEMENTED  = errors.New("NOT_IMPLEMENTED")
	EGENERAL         = errors.New("EGENERAL")
	EINVALIDLINK     = errors.New("invalid link")
	EALREADY_EXISTS  = errors.New("already exists")
	EPHANTOMERR      = errors.New("unexpected error")
	ERRTOOMANY       = errors.New("too many parameters")
)

var (
	EEDESERIALIZE        = "EVENT_DESERIALIZE_MESSAGE_ERROR"
	EEPLAYLISTCONVERSION = "playlist:conversion"
)

// MorbinTime because "its morbin time"
type MorbinTime string

type User struct {
	Email        string      `json:"email" db:"email"`
	Usernames    interface{} `json:"usernames" db:"usernames"`
	Username     string      `json:"username" db:"username"`
	ID           int         `json:"id" db:"id"`
	UUID         uuid.UUID   `json:"uuid" db:"uuid"`
	CreatedAt    string      `json:"created_at" db:"created_at"`
	UpdatedAt    string      `json:"updated_at" db:"updated_at"`
	RefreshToken []byte      `json:"refresh_token" db:"refresh_token,omitempty"`
	PlatformID   string      `json:"platform_id" db:"platform_id"`
}

//type AppleMusicAuthBody struct {
//	Authorization struct {
//		Code    string `json:"code"`
//		IdToken string `json:"id_token"`
//		State   string `json:"state"`
//	} `json:"authorization"`
//}

type AppleMusicAuthBody struct {
	Token     string `json:"token"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	State     string `json:"state"`
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
	Platform   string `json:"platform"`
	TargetLink string `json:"target_link"`
	Entity     string `json:"entity"`
	EntityID   string `json:"entity_id"`
}

// TrackSearchResult represents a single search result for a platform.
// It represents what a single platform should return when trying to
// convert a link.
type TrackSearchResult struct {
	URL      string   `json:"url"`
	Artistes []string `json:"artistes"`
	Released string   `json:"released"`
	Duration string   `json:"duration"`
	Explicit bool     `json:"explicit"`
	Title    string   `json:"title"`
	Preview  string   `json:"preview"`
	Album    string   `json:"album,omitempty"`
	ID       string   `json:"id"`
	Cover    string   `json:"cover"`
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
	Owner   string              `json:"owner"`
	Cover   string              `json:"cover"`
}

// PlatformSearchTrack represents the key-value parameter passed
// when trying to convert playlist from spotify
type PlatformSearchTrack struct {
	Artistes []string `json:"artiste"`
	Title    string   `json:"title"`
	ID       string   `json:"id"`
	URL      string   `json:"url"`
}

// Conversion represents the final response for a typical track conversion
type Conversion struct {
	Entity    string `json:"entity"`
	Platforms struct {
		Deezer     *TrackSearchResult `json:"deezer"`
		Spotify    *TrackSearchResult `json:"spotify"`
		Tidal      *TrackSearchResult `json:"tidal"`
		YTMusic    *TrackSearchResult `json:"ytmusic"`
		AppleMusic *TrackSearchResult `json:"applemusic"`
	} `json:"platforms"`
	ShortURL string `json:"short_url,omitempty"`
}

// PlaylistConversion represents the final response for a typical playlist conversion
type PlaylistConversion struct {
	URL    string `json:"url"`
	Tracks struct {
		Deezer     *[]TrackSearchResult `json:"deezer"`
		Spotify    *[]TrackSearchResult `json:"spotify"`
		Tidal      *[]TrackSearchResult `json:"tidal"`
		AppleMusic *[]TrackSearchResult `json:"applemusic"`
	} `json:"tracks"`
	Length        string                     `json:"length"`
	Title         string                     `json:"title"`
	Preview       string                     `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
	OmittedTracks map[string][]OmittedTracks `json:"omitted_tracks"`
	Owner         string                     `json:"owner"`
	Cover         string                     `json:"cover"`
	ShortURL      string                     `json:"short_url,omitempty"`
}

type TrackConversion struct {
	Entity    string `json:"entity"`
	Platforms struct {
		Deezer struct {
			Url      string   `json:"url"`
			Artistes []string `json:"artistes"`
			Released string   `json:"released"`
			Duration string   `json:"duration"`
			Explicit bool     `json:"explicit"`
			Title    string   `json:"title"`
			Preview  string   `json:"preview"`
			Album    string   `json:"album"`
			Id       string   `json:"id"`
			Cover    string   `json:"cover"`
		} `json:"deezer"`
		Spotify struct {
			Url      string   `json:"url"`
			Artistes []string `json:"artistes"`
			Released string   `json:"released"`
			Duration string   `json:"duration"`
			Explicit bool     `json:"explicit"`
			Title    string   `json:"title"`
			Preview  string   `json:"preview"`
			Album    string   `json:"album"`
			Id       string   `json:"id"`
			Cover    string   `json:"cover"`
		} `json:"spotify"`
		Tidal struct {
			Url      string   `json:"url"`
			Artistes []string `json:"artistes"`
			Released string   `json:"released"`
			Duration string   `json:"duration"`
			Explicit bool     `json:"explicit"`
			Title    string   `json:"title"`
			Preview  string   `json:"preview"`
			Album    string   `json:"album"`
			Id       string   `json:"id"`
			Cover    string   `json:"cover"`
		} `json:"tidal"`
		Ytmusic struct {
			Url      string   `json:"url"`
			Artistes []string `json:"artistes"`
			Released string   `json:"released"`
			Duration string   `json:"duration"`
			Explicit bool     `json:"explicit"`
			Title    string   `json:"title"`
			Preview  string   `json:"preview"`
			Album    string   `json:"album"`
			Id       string   `json:"id"`
			Cover    string   `json:"cover"`
		} `json:"ytmusic"`
		Applemusic struct {
			Url      string   `json:"url"`
			Artistes []string `json:"artistes"`
			Released string   `json:"released"`
			Duration string   `json:"duration"`
			Explicit bool     `json:"explicit"`
			Title    string   `json:"title"`
			Preview  string   `json:"preview"`
			Album    string   `json:"album"`
			Id       string   `json:"id"`
			Cover    string   `json:"cover"`
		} `json:"applemusic"`
	} `json:"platforms"`
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
	LinkInfo *LinkInfo `json:"link_info"`
	User     *User     `json:"user"`
}

// TaskRecord representsUs a task record in the database
type TaskRecord struct {
	Id        int       `json:"id,omitempty" db:"id"`
	User      uuid.UUID `json:"user,omitempty" db:"user"`
	UID       uuid.UUID `json:"uid,omitempty" db:"uuid"`
	CreatedAt time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty" db:"updated_at"`
	Result    string    `json:"result,omitempty" db:"result"`
	Status    string    `json:"status,omitempty" db:"status"`
	EntityID  string    `json:"entity_id,omitempty" db:"entity_id"`
	Type      string    `json:"type,omitempty" db:"type"`
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
}

type FollowTaskData struct {
	User     uuid.UUID `json:"user"`
	Url      string    `json:"url"`
	EntityID string    `json:"entity_id"`
	Platform string    `json:"platform"`
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
