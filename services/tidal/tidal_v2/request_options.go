package tidal_v2

import (
	"net/url"
	"strings"
)

type RequestOption func(*requestOptions)

// PlaylistIncludeOption represents valid include options for playlist endpoints
type PlaylistIncludeOption string

type SearchIncludeOption string

type SearchSuggestionIncludeOption string

type TrackIncludeOption string

// AlbumIncludeOption represents valid include options for album endpoints
type AlbumIncludeOption string

const (
	PlaylistIncludeCoverArt      PlaylistIncludeOption = "coverArt"
	PlaylistIncludeItems         PlaylistIncludeOption = "items"
	PlaylistIncludeOwnerProfiles PlaylistIncludeOption = "ownerProfiles"
	PlaylistIncludeOwners        PlaylistIncludeOption = "owners"

	// single track options
	TrackIncludeAlbum           TrackIncludeOption = "albums"
	TrackIncludeArtists         TrackIncludeOption = "artists"
	TrackIncludeGenres          TrackIncludeOption = "genres"
	TrackIncludeLyrics          TrackIncludeOption = "lyrics"
	TrackIncludeOwners          TrackIncludeOption = "owners"
	TrackIncludeProviders       TrackIncludeOption = "providers"
	TrackIncludeRadio           TrackIncludeOption = "radio"
	TrackIncludeShares          TrackIncludeOption = "shares"
	TrackIncludeSimilarTracks   TrackIncludeOption = "similarTracks"
	TrackIncludeSourceFile      TrackIncludeOption = "sourceFile"
	TrackIncludeTrackStatistics TrackIncludeOption = "trackStatistics"

	// search options
	SearchIncludeAlbums    SearchIncludeOption = "albums"
	SearchIncludeArtists   SearchIncludeOption = "artists"
	SearchIncludePlaylists SearchIncludeOption = "playlists"
	SearchIncludeTopHits   SearchIncludeOption = "topHits"
	SearchIncludeTracks    SearchIncludeOption = "tracks"
	SearchIncludeVideos    SearchIncludeOption = "videos"

	// search suggestion
	SearchIncludeDirectHits SearchSuggestionIncludeOption = "directHits"

	// album include options
	AlbumIncludeArtists          AlbumIncludeOption = "artists"
	AlbumIncludeTracks           AlbumIncludeOption = "items"
	AlbumIncludeCoverArt         AlbumIncludeOption = "coverArt"
	AlbumIncludeGenres           AlbumIncludeOption = "genres"
	AlbumIncludeProviders        AlbumIncludeOption = "providers"
	AlbumIncludeSimilarAlbums    AlbumIncludeOption = "similarAlbums"
	AlbumIncludeOwners           AlbumIncludeOption = "owners"
	AlbumIncludeSuggestCoverArts AlbumIncludeOption = "owners"
)

type requestOptions struct {
	urlParams url.Values
}

type IncludeOption interface {
	~string
}

// CountryCode is the ISO 3166-1 alpha-2 country code. Eg US (United States), DE (Deutschland/Germany)
func CountryCode(code string) RequestOption {
	return func(ro *requestOptions) {
		ro.urlParams.Set("countryCode", code)
	}
}

// NextPage is the cursor to the next page of the result. This is returned from the TIDAL API and its optional.
func NextPage(cursor string) RequestOption {
	return func(ro *requestOptions) {
		ro.urlParams.Set("page[cursor]", cursor)
	}
}

type ExplicitFiltersOption string

const (
	IncludeExplicitContent ExplicitFiltersOption = "INCLUDE"
	ExcludeExplicitContent ExplicitFiltersOption = "EXCLUDE"
)

func ExplicitFilter(filter ExplicitFiltersOption) RequestOption {
	return func(ro *requestOptions) {
		ro.urlParams.Set("explicitFilter", string(filter))
	}
}

// IncludeInPlaylist adds one or more include options to the playlist request.
// Valid options are:
//   - PlaylistIncludeCoverArt,
//   - PlaylistIncludeItems
//   - PlaylistIncludeOwnerProfiles
//   - PlaylistIncludeOwners
func IncludeInPlaylist(options ...PlaylistIncludeOption) RequestOption {
	return includeOption(options...)
}

// IncludeInTrack adds one or more include options to the request to fetch a single track.
//
// Valid options are:
//   - TrackIncludeAlbum
//   - TrackIncludeArtists
//   - TrackIncludeGenres
//   - TrackIncludeLyrics
//   - TrackIncludeOwners
//   - TrackIncludeProviders
//   - TrackIncludeRadio
//   - TrackIncludeShares
//   - TrackIncludeSimilarTracks
//   - TrackIncludeSourceFile
//   - TrackIncludeTrackStatistics
func IncludeInTrack(options ...TrackIncludeOption) RequestOption {
	return includeOption(options...)
}

// IncludeInSearch adds one or more include options to the search request.
//
// Valid options are:
//
//   - SearchIncludeAlbums
//   - SearchIncludeArtists
//   - SearchIncludePlaylists
//   - SearchIncludeTopHits
//   - SearchIncludeTracks
//   - SearchIncludeVideos
//
// Docs: https://tidal-music.github.io/tidal-api-reference/#/searchResults
func IncludeInSearch(options ...SearchIncludeOption) RequestOption {
	return includeOption(options...)
}

func IncludeInSearchSuggestion(options ...SearchSuggestionIncludeOption) RequestOption {
	return includeOption(options...)
}

func includeOption[T IncludeOption](options ...T) RequestOption {
	return func(ro *requestOptions) {
		if len(options) == 0 {
			return
		}

		includeValues := make([]string, len(options))
		for i, opt := range options {
			includeValues[i] = string(opt)
			ro.urlParams.Add("include", string(opt))
		}
	}
}

func buildRequestOptions(options ...RequestOption) requestOptions {
	op := requestOptions{
		urlParams: url.Values{},
	}

	for _, opt := range options {
		opt(&op)
	}
	return op
}

// IncludeInAlbum adds one or more include options to the album request
func IncludeInAlbum(options ...AlbumIncludeOption) RequestOption {
	return func(ro *requestOptions) {
		if len(options) == 0 {
			return
		}

		includeValues := make([]string, len(options))
		for i, opt := range options {
			includeValues[i] = string(opt)
		}

		ro.urlParams.Set("include", strings.Join(includeValues, ","))
	}
}
