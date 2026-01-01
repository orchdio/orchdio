package tidal_v2

import (
	"context"
	"fmt"
	"log"
	"time"
)

// PlaylistData represents the main playlist data object
type PlaylistData struct {
	ID            string                `json:"id"`
	Type          string                `json:"type"`
	Attributes    PlaylistAttributes    `json:"attributes"`
	Relationships PlaylistRelationships `json:"relationships"`
}

// PlaylistAttributes contains the playlist metadata
type PlaylistAttributes struct {
	AccessType        string                 `json:"accessType,omitempty"`
	Bounded           bool                   `json:"bounded,omitempty"`
	CreatedAt         time.Time              `json:"createdAt,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Duration          string                 `json:"duration,omitempty"`
	ExternalLinks     []PlaylistExternalLink `json:"externalLinks,omitempty"`
	LastModifiedAt    time.Time              `json:"lastModifiedAt,omitempty"`
	Name              string                 `json:"name,omitempty"`
	NumberOfFollowers int                    `json:"numberOfFollowers,omitempty"`
	NumberOfItems     int                    `json:"numberOfItems,omitempty"`
	PlaylistType      string                 `json:"playlistType,omitempty"`
}

// PlaylistExternalLink represents external links for the playlist
type PlaylistExternalLink struct {
	Href string                   `json:"href"`
	Meta PlaylistExternalLinkMeta `json:"meta"`
}

// PlaylistExternalLinkMeta contains metadata for external links
type PlaylistExternalLinkMeta struct {
	Type string `json:"type"`
}

// PlaylistRelationships contains all relationship references
type PlaylistRelationships struct {
	CoverArt      PlaylistCoverArtRelationship      `json:"coverArt,omitempty"`
	Items         PlaylistItemsRelationship         `json:"items,omitempty"`
	OwnerProfiles PlaylistOwnerProfilesRelationship `json:"ownerProfiles,omitempty"`
	Owners        PlaylistOwnersRelationship        `json:"owners,omitempty"`
}

// PlaylistCoverArtRelationship represents the cover art relationship
type PlaylistCoverArtRelationship struct {
	Data  []ResourceIdentifier `json:"data,omitempty"`
	Links Links                `json:"links,omitempty"`
}

// PlaylistItemsRelationship represents the items relationship
type PlaylistItemsRelationship struct {
	Data  []PlaylistItemData `json:"data,omitempty"`
	Links Links              `json:"links,omitempty"`
}

// PlaylistItemData represents a single item in the playlist
type PlaylistItemData struct {
	ID   string               `json:"id"`
	Type string               `json:"type"`
	Meta PlaylistItemDataMeta `json:"meta,omitempty"`
}

// PlaylistItemDataMeta contains metadata for playlist items
type PlaylistItemDataMeta struct {
	AddedAt time.Time `json:"addedAt,omitempty"`
	ItemID  string    `json:"itemId,omitempty"`
}

// PlaylistOwnerProfilesRelationship represents the owner profiles relationship
type PlaylistOwnerProfilesRelationship struct {
	Data  []ResourceIdentifier `json:"data,omitempty"`
	Links Links                `json:"links,omitempty"`
}

// PlaylistOwnersRelationship represents the owners relationship
type PlaylistOwnersRelationship struct {
	Data  []ResourceIdentifier `json:"data,omitempty"`
	Links Links                `json:"links,omitempty"`
}

// ResourceIdentifier represents a minimal resource reference
type ResourceIdentifier struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// // RelationshipLinks contains links for a relationship
// type RelationshipLinks struct {
// 	Links
// }

// PlaylistIncluded represents included resources (artists, artworks, tracks, users)
type PlaylistIncluded struct {
	ID            string                        `json:"id"`
	Type          string                        `json:"type"`
	Attributes    PlaylistIncludedAttributes    `json:"attributes,omitempty"`
	Relationships PlaylistIncludedRelationships `json:"relationships,omitempty"`
}

// PlaylistIncludedAttributes contains all possible attributes for included resources
// Fields are populated based on the Type field in PlaylistIncluded
type PlaylistIncludedAttributes struct {
	// Artist fields
	Handle     string   `json:"handle,omitempty"`
	Name       string   `json:"name,omitempty"`
	Popularity *float64 `json:"popularity,omitempty"`

	// Artwork fields
	Files     []ArtworkFile `json:"files,omitempty"`
	MediaType string        `json:"mediaType,omitempty"`

	// Track fields
	AccessType   string          `json:"accessType,omitempty"`
	Availability []string        `json:"availability,omitempty"`
	Copyright    *TrackCopyright `json:"copyright,omitempty"`
	CreatedAt    *time.Time      `json:"createdAt,omitempty"`
	Duration     string          `json:"duration,omitempty"`
	Explicit     bool            `json:"explicit,omitempty"`
	ISRC         string          `json:"isrc,omitempty"`
	MediaTags    []string        `json:"mediaTags,omitempty"`
	Spotlighted  bool            `json:"spotlighted,omitempty"`
	Title        string          `json:"title,omitempty"`
	Version      *string         `json:"version"`

	// User fields
	Country       string `json:"country,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"emailVerified,omitempty"`
	FirstName     string `json:"firstName,omitempty"`
	LastName      string `json:"lastName,omitempty"`
	Username      string `json:"username,omitempty"`

	// Shared fields
	ExternalLinks []PlaylistExternalLink `json:"externalLinks,omitempty"`
}

// ArtworkFile represents an image/file resource
type ArtworkFile struct {
	Href string          `json:"href"`
	Meta ArtworkFileMeta `json:"meta"`
}

// ArtworkFileMeta contains file metadata like dimensions
type ArtworkFileMeta struct {
	Height int `json:"height"`
	Width  int `json:"width"`
}

// TrackCopyright represents copyright information
type TrackCopyright struct {
	Text string `json:"text"`
}

// PlaylistIncludedRelationships contains relationships for included resources
type PlaylistIncludedRelationships struct {
	Albums          *PlaylistIncludedGenericRelationship `json:"albums,omitempty"`
	Artists         *PlaylistIncludedGenericRelationship `json:"artists,omitempty"`
	Biography       *PlaylistIncludedGenericRelationship `json:"biography,omitempty"`
	Followers       *PlaylistIncludedGenericRelationship `json:"followers,omitempty"`
	Following       *PlaylistIncludedGenericRelationship `json:"following,omitempty"`
	Genres          *PlaylistIncludedGenericRelationship `json:"genres,omitempty"`
	Lyrics          *PlaylistIncludedGenericRelationship `json:"lyrics,omitempty"`
	Owners          *PlaylistIncludedGenericRelationship `json:"owners,omitempty"`
	ProfileArt      *PlaylistIncludedGenericRelationship `json:"profileArt,omitempty"`
	Providers       *PlaylistIncludedGenericRelationship `json:"providers,omitempty"`
	Radio           *PlaylistIncludedGenericRelationship `json:"radio,omitempty"`
	Roles           *PlaylistIncludedGenericRelationship `json:"roles,omitempty"`
	Shares          *PlaylistIncludedGenericRelationship `json:"shares,omitempty"`
	SimilarArtists  *PlaylistIncludedGenericRelationship `json:"similarArtists,omitempty"`
	SimilarTracks   *PlaylistIncludedGenericRelationship `json:"similarTracks,omitempty"`
	SourceFile      *PlaylistIncludedGenericRelationship `json:"sourceFile,omitempty"`
	TrackProviders  *PlaylistIncludedGenericRelationship `json:"trackProviders,omitempty"`
	Tracks          *PlaylistIncludedGenericRelationship `json:"tracks,omitempty"`
	TrackStatistics *PlaylistIncludedGenericRelationship `json:"trackStatistics,omitempty"`
	Videos          *PlaylistIncludedGenericRelationship `json:"videos,omitempty"`
}

// PlaylistIncludedGenericRelationship represents a generic relationship in included resources
type PlaylistIncludedGenericRelationship struct {
	Links Links `json:"links,omitempty"`
}

// PlaylistLinks contains response-level links
type PlaylistLinks struct {
	Self string `json:"self"`
}

// PlaylistResponse is the complete API response type
type PlaylistResponse = SuccessResponse[PlaylistData, PlaylistIncluded, PlaylistLinks]

// GetPlaylist fetches the playlist information. It takes request options:
// -
func (tc *TidalClient) GetPlaylist(ctx context.Context, playlistID string, countryCode string, opts ...RequestOption) (*PlaylistResponse, error) {
	playlistInfoURL := fmt.Sprintf("%splaylists/%s", tc.baseURL, playlistID)
	allOpts := append([]RequestOption{CountryCode(countryCode)}, opts...)
	params := buildRequestOptions(allOpts...).urlParams.Encode()
	if params != "" {
		playlistInfoURL = fmt.Sprintf("%s?%s", playlistInfoURL, params)
	}

	var playlistInf PlaylistResponse
	err := tc.get(ctx, playlistInfoURL, &playlistInf)
	if err != nil {
		return nil, err
	}
	return &playlistInf, nil
}

// PlaylistItemsLinksMeta contains pagination metadata
type PlaylistItemsLinksMeta struct {
	NextCursor string `json:"nextCursor,omitempty"`
}

// PlaylistItemsLinks contains links for playlist items response
type PlaylistItemsLinks struct {
	Self string                 `json:"self"`
	Next string                 `json:"next,omitempty"`
	Meta PlaylistItemsLinksMeta `json:"meta,omitempty"`
}

// PlaylistItemsResponse is the complete API response for playlist items
type PlaylistItemsResponse struct {
	Data     []PlaylistItemData `json:"data"`
	Included []PlaylistIncluded `json:"included,omitempty"`
	Links    PlaylistItemsLinks `json:"links"`
}

// GetPlaylistItems fetches items from a playlist with optional pagination.
// The only supported include option is "items".
func (tc *TidalClient) GetPlaylistItems(ctx context.Context, playlistID, countryCode string, nextURL ...string) (*PlaylistItemsResponse, error) {
	var playlistItemsEndpoint string

	if len(nextURL) > 0 && nextURL[0] != "" {
		playlistItemsEndpoint = tc.baseURL + nextURL[0]
	} else {
		playlistItemsEndpoint = fmt.Sprintf("%splaylists/%s/relationships/items", tc.baseURL, playlistID)
		allOpts := append([]RequestOption{
			CountryCode(countryCode)},
			IncludeInPlaylistItems(PlaylistItemsInclude),
		)
		params := buildRequestOptions(allOpts...).urlParams.Encode()
		if params != "" {
			playlistItemsEndpoint = fmt.Sprintf("%s?%s", playlistItemsEndpoint, params)
		}
	}
	log.Println("===============================================")
	log.Println("DEBUG: playlist item link visited...")
	log.Println(playlistItemsEndpoint)
	log.Println("===============================================")

	var response PlaylistItemsResponse
	err := tc.get(ctx, playlistItemsEndpoint, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}
