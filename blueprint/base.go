package blueprint

import (
	"errors"
	"github.com/golang-jwt/jwt/v4"
)

var DeezerHost = []string{"deezer.page.link", "www.deezer.com"}

const (
	SpotifyHost = "open.spotify.com"
)

// perhaps have a different Error type declarations somewhere. For now, be here

var (
	EHOSTUNSUPPORTED = errors.New("EHOSTUNSUPPORTED")
	ENORESULT        = errors.New("ENORESULT")
	ENOTIMPLEMENTED  = errors.New("NOT_IMPLEMENTED")
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
	//Pagination []map[string]string `json:"pagination"`
}

type Pagination struct {
	Next     string `json:"next"`
	Previous string `json:"previous"`
	Total    int    `json:"total,omitempty"`
	Platform string `json:"platform"`
}

// PlaylistSearchResult represents a single playlist result for a platform.
type PlaylistSearchResult struct {
	URL     string              `json:"url"`
	Tracks  []TrackSearchResult `json:"tracks"`
	Length  string              `json:"length,omitempty"`
	Title   string              `json:"title"`
	Preview string              `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
}

// DeezerSearchTrack represents the key-value parameter passed
// when trying to convert playlist from spotify
type DeezerSearchTrack struct {
	Artiste string `json:"artiste"`
	Title   string `json:"title"`
	ID      string `json:"id"`
}

// Conversion represents the final response for a typical track conversion
type Conversion struct {
	Entity    string `json:"entity"`
	Platforms struct {
		Deezer  *TrackSearchResult `json:"deezer"`
		Spotify *TrackSearchResult `json:"spotify"`
	} `json:"platforms"`
}

// PlaylistConversion represents the final response for a typical playlist conversion
type PlaylistConversion struct {
	URL string `json:"url"`
	//Tracks  []map[string]*[]blueprint.TrackSearchResult `json:"tracks"`
	Tracks struct {
		Deezer  *[]TrackSearchResult `json:"deezer"`
		Spotify *[]TrackSearchResult `json:"spotify"`
	} `json:"tracks"`
	Length     string     `json:"length"`
	Title      string     `json:"title"`
	Preview    string     `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
	Pagination Pagination `json:"pagination"`
}

// Message represents a message sent from the client to the server over websocket
type Message struct {
	Link       string   `json:"link"`
	Attributes struct{} `json:"attributes"`
}
