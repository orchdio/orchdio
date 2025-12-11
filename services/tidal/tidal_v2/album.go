package tidal_v2

import (
	"context"
	"fmt"
	"log"
)

// AlbumData represents the main album data object
type AlbumData struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Attributes    AlbumAttributes    `json:"attributes"`
	Relationships AlbumRelationships `json:"relationships"`
	Links         AlbumDataLinks     `json:"links,omitempty"`
}

// AlbumAttributes contains the album metadata
type AlbumAttributes struct {
	Title             string              `json:"title"`
	BarcodeID         string              `json:"barcodeId,omitempty"`
	Barcodes          []string            `json:"barcodes,omitempty"`
	Duration          string              `json:"duration,omitempty"`
	ReleaseDate       string              `json:"releaseDate,omitempty"`
	NumberOfVolumes   int                 `json:"numberOfVolumes,omitempty"`
	NumberOfItems     int                 `json:"numberOfItems,omitempty"`
	Copyright         AlbumCopyright      `json:"copyright,omitempty"`
	MediaMetadataTags []string            `json:"mediaMetadataTags,omitempty"`
	Availability      []string            `json:"availability,omitempty"`
	Explicit          bool                `json:"explicit,omitempty"`
	Popularity        float64             `json:"popularity,omitempty"`
	Type              string              `json:"type,omitempty"`
	ExternalLinks     []AlbumExternalLink `json:"externalLinks,omitempty"`
}

// AlbumCopyright represents copyright information
type AlbumCopyright struct {
	Text string `json:"text"`
}

// AlbumExternalLink represents external links for the album
type AlbumExternalLink struct {
	Href string                `json:"href"`
	Meta AlbumExternalLinkMeta `json:"meta"`
}

// AlbumExternalLinkMeta contains metadata for external links
type AlbumExternalLinkMeta struct {
	Type string `json:"type"`
}

// AlbumRelationships contains all album relationship references
type AlbumRelationships struct {
	Artists       AlbumRelationship `json:"artists,omitempty"`
	CoverArt      AlbumRelationship `json:"coverArt,omitempty"`
	Genres        AlbumRelationship `json:"genres,omitempty"`
	Providers     AlbumRelationship `json:"providers,omitempty"`
	Tracks        AlbumRelationship `json:"tracks,omitempty"`
	SimilarAlbums AlbumRelationship `json:"similarAlbums,omitempty"`
}

// AlbumRelationship represents a relationship with data and links
type AlbumRelationship struct {
	Data  []ResourceIdentifier `json:"data,omitempty"`
	Links RelationshipLinks    `json:"links,omitempty"`
}

// AlbumDataLinks contains self link for album data
type AlbumDataLinks struct {
	Self string `json:"self"`
}

// AlbumIncluded represents included resources (artists, tracks, cover art, etc.)
type AlbumIncluded struct {
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Attributes    AlbumIncludedAttributes    `json:"attributes,omitempty"`
	Relationships AlbumIncludedRelationships `json:"relationships,omitempty"`
	Links         AlbumIncludedLinks         `json:"links,omitempty"`
}

// AlbumIncludedAttributes contains attributes for included resources
type AlbumIncludedAttributes struct {
	// Artist attributes
	Name    string `json:"name,omitempty"`
	Country string `json:"country,omitempty"`

	// Track attributes
	Title         string                 `json:"title,omitempty"`
	Version       string                 `json:"version,omitempty"`
	ISRC          string                 `json:"isrc,omitempty"`
	Duration      string                 `json:"duration,omitempty"`
	Copyright     string                 `json:"copyright,omitempty"`
	Explicit      bool                   `json:"explicit,omitempty"`
	Popularity    float64                `json:"popularity,omitempty"`
	TrackNumber   int                    `json:"trackNumber,omitempty"`
	VolumeNumber  int                    `json:"volumeNumber,omitempty"`
	Availability  []string               `json:"availability,omitempty"`
	MediaTags     []string               `json:"mediaTags,omitempty"`
	ExternalLinks []AlbumIncludedExtLink `json:"externalLinks,omitempty"`

	// Image/CoverArt attributes
	Files     []AlbumIncludedFile `json:"files,omitempty"`
	MediaType string              `json:"mediaType,omitempty"`

	// Genre attributes
	GenreName string `json:"genreName,omitempty"`

	// Provider attributes
	ProviderId string `json:"providerId,omitempty"`
}

// AlbumIncludedExtLink represents external links in included resources
type AlbumIncludedExtLink struct {
	Href string                   `json:"href"`
	Meta AlbumIncludedExtLinkMeta `json:"meta"`
}

// AlbumIncludedExtLinkMeta contains metadata for external links
type AlbumIncludedExtLinkMeta struct {
	Type string `json:"type"`
}

// AlbumIncludedFile represents an image/file resource
type AlbumIncludedFile struct {
	Href string                `json:"href"`
	Meta AlbumIncludedFileMeta `json:"meta"`
}

// AlbumIncludedFileMeta contains file metadata like dimensions
type AlbumIncludedFileMeta struct {
	Height int `json:"height"`
	Width  int `json:"width"`
}

// AlbumIncludedRelationships contains relationships for included resources
type AlbumIncludedRelationships struct {
	Albums        AlbumIncludedRelationship `json:"albums,omitempty"`
	Artists       AlbumIncludedRelationship `json:"artists,omitempty"`
	CoverArt      AlbumIncludedRelationship `json:"coverArt,omitempty"`
	Genres        AlbumIncludedRelationship `json:"genres,omitempty"`
	Owners        AlbumIncludedRelationship `json:"owners,omitempty"`
	Providers     AlbumIncludedRelationship `json:"providers,omitempty"`
	Radio         AlbumIncludedRelationship `json:"radio,omitempty"`
	SimilarAlbums AlbumIncludedRelationship `json:"similarAlbums,omitempty"`
	SimilarTracks AlbumIncludedRelationship `json:"similarTracks,omitempty"`
	Tracks        AlbumIncludedRelationship `json:"tracks,omitempty"`
}

// AlbumIncludedRelationship represents a single relationship in included resources
type AlbumIncludedRelationship struct {
	Data  []ResourceIdentifier `json:"data,omitempty"`
	Links RelationshipLinks    `json:"links,omitempty"`
}

// AlbumIncludedLinks contains links for included resources
type AlbumIncludedLinks struct {
	Self string `json:"self"`
}

// AlbumResponse is the complete API response type for GET /albums/{id}
type AlbumResponse = SuccessResponse[AlbumData, AlbumIncluded, Links]

// GetAlbum fetches album information by ID
// Valid include options: artists, tracks, coverArt, genres, providers, similarAlbums
func (tc *TidalClient) GetAlbum(ctx context.Context, albumID string, opts ...RequestOption) (*AlbumResponse, error) {
	albumURL := fmt.Sprintf("%salbums/%s", tc.baseURL, albumID)
	params := buildRequestOptions(opts...).urlParams.Encode()
	if params != "" {
		albumURL = fmt.Sprintf("%s?%s", albumURL, params)
	}

	var result AlbumResponse
	err := tc.get(ctx, albumURL, &result)
	if err != nil {
		log.Println("ERROR FETCHING ALBUM FROM TIDAL...")
		return nil, err
	}

	return &result, nil
}
