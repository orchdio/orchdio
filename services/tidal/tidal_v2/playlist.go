package tidal_v2

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
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
	AccessType     string                 `json:"accessType,omitempty"`
	Bounded        bool                   `json:"bounded,omitempty"`
	CreatedAt      time.Time              `json:"createdAt,omitempty"`
	Description    string                 `json:"description,omitempty"`
	Duration       string                 `json:"duration,omitempty"`
	ExternalLinks  []PlaylistExternalLink `json:"externalLinks,omitempty"`
	LastModifiedAt time.Time              `json:"lastModifiedAt,omitempty"`
	Name           string                 `json:"name,omitempty"`
	NumberOfItems  int                    `json:"numberOfItems,omitempty"`
	PlaylistType   string                 `json:"playlistType,omitempty"`
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
	Links RelationshipLinks    `json:"links,omitempty"`
}

// PlaylistItemsRelationship represents the items relationship
type PlaylistItemsRelationship struct {
	Links RelationshipLinks `json:"links,omitempty"`
}

// PlaylistOwnerProfilesRelationship represents the owner profiles relationship
type PlaylistOwnerProfilesRelationship struct {
	Links RelationshipLinks `json:"links,omitempty"`
}

// PlaylistOwnersRelationship represents the owners relationship
type PlaylistOwnersRelationship struct {
	Links RelationshipLinks `json:"links,omitempty"`
}

// ResourceIdentifier represents a minimal resource reference
type ResourceIdentifier struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// RelationshipLinks contains links for a relationship
type RelationshipLinks struct {
	Self string `json:"self"`
}

// PlaylistIncluded represents included resources (cover art, owner profiles, etc.)
type PlaylistIncluded struct {
	ID            string                        `json:"id"`
	Type          string                        `json:"type"`
	Attributes    PlaylistIncludedAttributes    `json:"attributes,omitempty"`
	Relationships PlaylistIncludedRelationships `json:"relationships,omitempty"`
}

// PlaylistIncludedAttributes contains attributes for included resources
type PlaylistIncludedAttributes struct {
	Files     []PlaylistIncludedFile `json:"files,omitempty"`
	MediaType string                 `json:"mediaType,omitempty"`
}

// PlaylistIncludedFile represents an image/file resource
type PlaylistIncludedFile struct {
	Href string                   `json:"href"`
	Meta PlaylistIncludedFileMeta `json:"meta"`
}

// PlaylistIncludedFileMeta contains file metadata like dimensions
type PlaylistIncludedFileMeta struct {
	Height int `json:"height"`
	Width  int `json:"width"`
}

// PlaylistIncludedRelationships contains relationships for included resources
type PlaylistIncludedRelationships struct {
	Owners PlaylistIncludedOwnersRelationship `json:"owners,omitempty"`
}

// PlaylistIncludedOwnersRelationship represents owners relationship in included resources
type PlaylistIncludedOwnersRelationship struct {
	Links RelationshipLinks `json:"links,omitempty"`
}

// PlaylistResponse is the complete API response type
type PlaylistResponse = SuccessResponse[PlaylistData, PlaylistIncluded, Links]

// GetPlaylist fetches the playlist information. It takes request options:
// -
func (tc *TidalClient) GetPlaylist(ctx context.Context, playlistID string, opts ...RequestOption) (*PlaylistResponse, error) {
	playlistInfoURL := fmt.Sprintf("%splaylists/%s", tc.baseURL, playlistID)
	params := buildRequestOptions(opts...).urlParams.Encode()
	if params != "" {
		playlistInfoURL = fmt.Sprintf("%s?%s", playlistInfoURL, params)
	}
	var playlistInf PlaylistResponse
	err := tc.get(ctx, playlistInfoURL, &playlistInf)
	if err != nil {
		log.Println("ERROR FETCHING PLAYLIST INFO FROM TIDAL...")
		return nil, err
	}

	log.Println("DEV::: RESULT OF PLAYLIST IS...")
	spew.Dump(playlistInf)
	return &playlistInf, nil
}
