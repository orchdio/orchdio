package blueprint

import (
	"errors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

var DeezerHost = []string{"deezer.page.link", "www.deezer.com"}

const (
	SpotifyHost = "open.spotify.com"
	TidalHost   = "tidal.com"
)

// perhaps have a different Error type declarations somewhere. For now, be here

var (
	EHOSTUNSUPPORTED = errors.New("EHOSTUNSUPPORTED")
	ENORESULT        = errors.New("ENORESULT")
	ENOTIMPLEMENTED  = errors.New("NOT_IMPLEMENTED")
	EGENERAL         = errors.New("EGENERAL")
)

var (
	EEDESERIALIZE        = "EVENT_DESERIALIZE_MESSAGE_ERROR"
	EEPLAYLISTCONVERSION = "EVENT_PLAYLIST_CONVERSION_ERROR"
)

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
		Deezer  *TrackSearchResult `json:"deezer"`
		Spotify *TrackSearchResult `json:"spotify"`
		Tidal   *TrackSearchResult `json:"tidal"`
	} `json:"platforms"`
}

// PlaylistConversion represents the final response for a typical playlist conversion
type PlaylistConversion struct {
	URL    string `json:"url"`
	Tracks struct {
		Deezer  *[]TrackSearchResult `json:"deezer"`
		Spotify *[]TrackSearchResult `json:"spotify"`
		Tidal   *[]TrackSearchResult `json:"tidal"`
	} `json:"tracks"`
	Length        string                     `json:"length"`
	Title         string                     `json:"title"`
	Preview       string                     `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
	OmittedTracks map[string][]OmittedTracks `json:"omitted_tracks"`
	Owner         string                     `json:"owner"`
	Cover         string                     `json:"cover"`
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
