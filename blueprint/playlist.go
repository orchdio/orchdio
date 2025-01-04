package blueprint

import (
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx/types"
	"time"
)

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
	Meta PlaylistMetadata `json:"meta,omitempty"`
}

type PlaylistMetadata struct {
	Length   string `json:"length"`
	Title    string `json:"title"`
	Preview  string `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
	Owner    string `json:"owner"`
	Cover    string `json:"cover"`
	Entity   string `json:"entity"`
	URL      string `json:"url"`
	ShortURL string `json:"short_url,omitempty"`
}

type PlaylistConversionEventMetadata struct {
	Platform string            `json:"platform"`
	Meta     *PlaylistMetadata `json:"meta"`
}

type PlaylistConversionEventTrack struct {
	Platform string             `json:"platform"`
	Track    *TrackSearchResult `json:"track"`
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

// PlaylistTaskData represents the payload of a playlist task
type PlaylistTaskData struct {
	LinkInfo *LinkInfo     `json:"link_info"`
	App      *DeveloperApp `json:"app"`
	TaskID   string        `json:"task_id"`
	ShortURL string        `json:"short_url"`
}

type AddPlaylistToAccountData struct {
	// TODO: in the future, perhaps look into the viability of allowing multiple users and also support email and id in api for user id
	User uuid.UUID `json:"user"`
	Url  string    `json:"url"`
}
