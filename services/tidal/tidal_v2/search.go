package tidal_v2

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

// SearchResultsData represents the main data object for search results
type SearchResultsData struct {
	Attributes    SearchResultsAttributes    `json:"attributes"`
	ID            string                     `json:"id"`
	Relationships SearchResultsRelationships `json:"relationships"`
	Type          string                     `json:"type"`
}

// SearchResultsAttributes contains the search results metadata
type SearchResultsAttributes struct {
	TrackingID string `json:"trackingId"`
}

// SearchResultsRelationships contains all relationship references
type SearchResultsRelationships struct {
	Albums    SearchResultsRelationship `json:"albums,omitempty"`
	Artists   SearchResultsRelationship `json:"artists,omitempty"`
	Playlists SearchResultsRelationship `json:"playlists,omitempty"`
	TopHits   SearchResultsRelationship `json:"topHits,omitempty"`
	Tracks    SearchResultsRelationship `json:"tracks,omitempty"`
	Videos    SearchResultsRelationship `json:"videos,omitempty"`
}

// SearchResultsRelationship represents a relationship with data and links
type SearchResultsRelationship struct {
	Data  []ResourceIdentifier           `json:"data,omitempty"`
	Links SearchResultsRelationshipLinks `json:"links,omitempty"`
}

// SearchResultsRelationshipLinks contains links for relationships
type SearchResultsRelationshipLinks struct {
	Self string                         `json:"self,omitempty"`
	Next string                         `json:"next,omitempty"`
	Meta *SearchResultsRelationshipMeta `json:"meta,omitempty"`
}

// SearchResultsRelationshipMeta contains metadata like cursor for pagination
type SearchResultsRelationshipMeta struct {
	NextCursor string `json:"nextCursor,omitempty"`
}

// SearchResultsIncluded represents included resources (albums, artists, tracks, etc.)
type SearchResultsIncluded struct {
	Attributes    SearchResultsIncludedAttributes    `json:"attributes,omitempty"`
	ID            string                             `json:"id"`
	Relationships SearchResultsIncludedRelationships `json:"relationships,omitempty"`
	Type          string                             `json:"type"`
}

// SearchResultsCopyright represents copyright information
type SearchResultsCopyright struct {
	Text string `json:"text"`
}

// SearchResultsIncludedAttributes contains attributes for included resources
type SearchResultsIncludedAttributes struct {
	// Common fields
	Title      string  `json:"title,omitempty"`
	Name       string  `json:"name,omitempty"`
	Popularity float64 `json:"popularity,omitempty"`

	// Album/Track/Playlist specific
	AccessType      string                      `json:"accessType,omitempty"`
	Availability    []string                    `json:"availability,omitempty"`
	BarcodeID       string                      `json:"barcodeId,omitempty"`
	Copyright       *SearchResultsCopyright     `json:"copyright,omitempty"`
	Duration        string                      `json:"duration,omitempty"`
	Explicit        bool                        `json:"explicit,omitempty"`
	ExternalLinks   []SearchResultsExternalLink `json:"externalLinks,omitempty"`
	MediaTags       []string                    `json:"mediaTags,omitempty"`
	NumberOfItems   int                         `json:"numberOfItems,omitempty"`
	NumberOfVolumes int                         `json:"numberOfVolumes,omitempty"`
	ReleaseDate     string                      `json:"releaseDate,omitempty"`
	Type            string                      `json:"type,omitempty"`
	Version         *string                     `json:"version"`

	// Track specific
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	ISRC        string     `json:"isrc,omitempty"`
	Spotlighted bool       `json:"spotlighted,omitempty"`

	// Playlist specific
	Bounded        bool       `json:"bounded,omitempty"`
	Description    string     `json:"description,omitempty"`
	LastModifiedAt *time.Time `json:"lastModifiedAt,omitempty"`
	PlaylistType   string     `json:"playlistType,omitempty"`
}

// SearchResultsExternalLink represents external links
type SearchResultsExternalLink struct {
	Href string                        `json:"href"`
	Meta SearchResultsExternalLinkMeta `json:"meta"`
}

// SearchResultsExternalLinkMeta contains metadata for external links
type SearchResultsExternalLinkMeta struct {
	Type string `json:"type"`
}

// SearchResultsIncludedRelationships contains relationships for included resources
type SearchResultsIncludedRelationships struct {
	Albums             SearchResultsIncludedRelationship `json:"albums,omitempty"`
	Artists            SearchResultsIncludedRelationship `json:"artists,omitempty"`
	Biography          SearchResultsIncludedRelationship `json:"biography,omitempty"`
	CoverArt           SearchResultsIncludedRelationship `json:"coverArt,omitempty"`
	Followers          SearchResultsIncludedRelationship `json:"followers,omitempty"`
	Following          SearchResultsIncludedRelationship `json:"following,omitempty"`
	Genres             SearchResultsIncludedRelationship `json:"genres,omitempty"`
	Items              SearchResultsIncludedRelationship `json:"items,omitempty"`
	Lyrics             SearchResultsIncludedRelationship `json:"lyrics,omitempty"`
	OwnerProfiles      SearchResultsIncludedRelationship `json:"ownerProfiles,omitempty"`
	Owners             SearchResultsIncludedRelationship `json:"owners,omitempty"`
	ProfileArt         SearchResultsIncludedRelationship `json:"profileArt,omitempty"`
	Providers          SearchResultsIncludedRelationship `json:"providers,omitempty"`
	Radio              SearchResultsIncludedRelationship `json:"radio,omitempty"`
	Roles              SearchResultsIncludedRelationship `json:"roles,omitempty"`
	Shares             SearchResultsIncludedRelationship `json:"shares,omitempty"`
	SimilarAlbums      SearchResultsIncludedRelationship `json:"similarAlbums,omitempty"`
	SimilarArtists     SearchResultsIncludedRelationship `json:"similarArtists,omitempty"`
	SimilarTracks      SearchResultsIncludedRelationship `json:"similarTracks,omitempty"`
	SourceFile         SearchResultsIncludedRelationship `json:"sourceFile,omitempty"`
	SuggestedCoverArts SearchResultsIncludedRelationship `json:"suggestedCoverArts,omitempty"`
	ThumbnailArt       SearchResultsIncludedRelationship `json:"thumbnailArt,omitempty"`
	TrackProviders     SearchResultsIncludedRelationship `json:"trackProviders,omitempty"`
	Tracks             SearchResultsIncludedRelationship `json:"tracks,omitempty"`
	TrackStatistics    SearchResultsIncludedRelationship `json:"trackStatistics,omitempty"`
	Videos             SearchResultsIncludedRelationship `json:"videos,omitempty"`
}

// SearchResultsIncludedRelationship represents a single relationship in included resources
type SearchResultsIncludedRelationship struct {
	Links RelationshipLinks `json:"links,omitempty"`
}

// SearchResponse is the complete API response type for search
type SearchResponse = SuccessResponse[SearchResultsData, SearchResultsIncluded, Links]

func customPathEscape(query string) string {
	// First apply standard path escaping
	escaped := url.PathEscape(query)

	// Additionally encode & which PathEscape doesn't encode
	escaped = strings.ReplaceAll(escaped, "&", "%26")

	return escaped
}

func (tc *TidalClient) Search(ctx context.Context, query, countryCode string, opts ...RequestOption) (*SearchResponse, error) {
	// Build the options first
	allOpts := append([]RequestOption{CountryCode(countryCode)}, opts...)
	requestOpts := buildRequestOptions(allOpts...)

	// Log the actual parameter values
	log.Println("=== Search Parameters ===")
	log.Println("Query:", query)
	log.Println("URL Parameters:")
	for key, values := range requestOpts.urlParams {
		for _, value := range values {
			log.Printf("  %s: %s", key, value)
		}
	}
	log.Println("========================")

	// Use customPathEscape instead of url.PathEscape
	searchURL := fmt.Sprintf("%ssearchResults/%s", tc.baseURL, customPathEscape(query))

	params := requestOpts.urlParams.Encode()
	if params != "" {
		searchURL = fmt.Sprintf("%s?%s", searchURL, params)
	}

	log.Println("Final URL:", searchURL)

	var searchResults SearchResponse
	err := tc.get(ctx, searchURL, &searchResults)
	if err != nil {
		log.Println("ERROR FETCHING SEARCH RESULTS FROM TIDAL...")
		return nil, err
	}

	return &searchResults, nil
}

// SearchSuggestionsData represents the main data object for search suggestions
type SearchSuggestionsData struct {
	Attributes    SearchSuggestionsAttributes    `json:"attributes"`
	ID            string                         `json:"id"`
	Relationships SearchSuggestionsRelationships `json:"relationships"`
	Type          string                         `json:"type"`
}

// SearchSuggestionsAttributes contains the search suggestions metadata
type SearchSuggestionsAttributes struct {
	History     []SearchSuggestionItem `json:"history"`
	Suggestions []SearchSuggestionItem `json:"suggestions"`
	TrackingID  string                 `json:"trackingId"`
}

// SearchSuggestionItem represents a single suggestion with highlights
type SearchSuggestionItem struct {
	Highlights []SearchSuggestionHighlight `json:"highlights"`
	Query      string                      `json:"query"`
}

// SearchSuggestionHighlight represents text highlighting information
type SearchSuggestionHighlight struct {
	Length int `json:"length"`
	Start  int `json:"start"`
}

// SearchSuggestionsRelationships contains relationship references for suggestions
type SearchSuggestionsRelationships struct {
	DirectHits SearchResultsRelationship `json:"directHits,omitempty"`
}

// SearchSuggestionsResponse is the complete API response type for search suggestions
type SearchSuggestionsResponse = SuccessResponse[SearchSuggestionsData, SearchResultsIncluded, Links]

// SearchSuggestions fetches search suggestions for a query
func (tc *TidalClient) SearchSuggestions(ctx context.Context, query, countryCode string, opts ...RequestOption) (*SearchSuggestionsResponse, error) {
	// Build the options first
	allOpts := append([]RequestOption{CountryCode(countryCode)}, opts...)
	requestOpts := buildRequestOptions(allOpts...)

	searchURL := fmt.Sprintf("%ssearchSuggestions/%s", tc.baseURL, customPathEscape(query))
	params := requestOpts.urlParams.Encode()
	if params != "" {
		searchURL = fmt.Sprintf("%s?%s", searchURL, params)
	}

	var searchSuggestions SearchSuggestionsResponse
	err := tc.get(ctx, searchURL, &searchSuggestions)
	if err != nil {
		log.Println("ERROR FETCHING SEARCH SUGGESTIONS FROM TIDAL...")
		return nil, err
	}

	return &searchSuggestions, nil
}
