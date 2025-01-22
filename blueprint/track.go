package blueprint

// OmittedTracks represents tracks that could not be processed in a playlist, for whatever reason
type OmittedTracks struct {
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Artistes []string `json:"artistes"`
	Platform string   `json:"platform,omitempty"`
	Index    int      `json:"index,omitempty"`
}

// TrackConversion represents the final response for a typical track conversion
type TrackConversion struct {
	Entity    string `json:"entity"`
	Platforms struct {
		Deezer     *TrackSearchResult `json:"deezer,omitempty"`
		Spotify    *TrackSearchResult `json:"spotify,omitempty"`
		Tidal      *TrackSearchResult `json:"tidal,omitempty"`
		YTMusic    *TrackSearchResult `json:"ytmusic,omitempty"`
		AppleMusic *TrackSearchResult `json:"applemusic,omitempty"`
	} `json:"platforms"`
	// shortURL is the same as taskId. also adding shortURL here because it's easier
	// and (probably) makes more sense for the track conversion payload to carry it itself
	// for easier integration.
	UniqueID string `json:"unique_id,omitempty"`
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

type TrackSearchData struct {
	Title   string   `json:"title"`
	Artists []string `json:"artists"`
	Album   string   `json:"album"`
	Meta    struct {
		HasPlaylist bool   `json:"has_playlist"`
		PlaylistID  string `json:"playlist_id"`
	}
}

// PlatformSearchTrack represents the key-value parameter passed
// when trying to convert playlist from spotify
type PlatformSearchTrack struct {
	Artistes []string `json:"artist"`
	Title    string   `json:"title"`
	ID       string   `json:"id"`
	URL      string   `json:"url"`
}

type UserArtist struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Cover string `json:"cover"`
	URL   string `json:"url"`
}

type UserLibraryArtists struct {
	Payload []UserArtist `json:"payload"`
	Total   int          `json:"total"`
}
