package tidal_v2

import (
	"context"
	"fmt"
	"log"
	"time"
)

// TrackData represents the main track data object
type TrackData struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Attributes    TrackAttributes    `json:"attributes"`
	Relationships TrackRelationships `json:"relationships"`
	Links         TrackDataLinks     `json:"links,omitempty"`
}

// TrackAttributes contains the track metadata
type TrackAttributes struct {
	AccessType    string         `json:"accessType"`
	Availability  []string       `json:"availability"`
	Copyright     Copyright      `json:"copyright"`
	CreatedAt     time.Time      `json:"createdAt"`
	Duration      string         `json:"duration"`
	Explicit      bool           `json:"explicit"`
	ExternalLinks []ExternalLink `json:"externalLinks"`
	ISRC          string         `json:"isrc"`
	MediaTags     []string       `json:"mediaTags"`
	Popularity    float64        `json:"popularity"`
	Spotlighted   bool           `json:"spotlighted"`
	Title         string         `json:"title"`
	Version       *string        `json:"version"`
}

type Copyright struct {
	Text string `json:"text"`
}

type ExternalLink struct {
	Href string       `json:"href"`
	Meta ExternalMeta `json:"meta"`
}

type ExternalMeta struct {
	Type string `json:"type"`
}

// TrackExternalLink represents external links for the track
type TrackExternalLink struct {
	Href string                `json:"href"`
	Meta TrackExternalLinkMeta `json:"meta"`
}

// TrackExternalLinkMeta contains metadata for external links
type TrackExternalLinkMeta struct {
	Type string `json:"type"`
}

// TrackRelationships contains all track relationship references
type TrackRelationships struct {
	Albums          TrackRelationship `json:"albums,omitempty"`
	Artists         TrackRelationship `json:"artists,omitempty"`
	Providers       TrackRelationship `json:"providers,omitempty"`
	Radio           TrackRelationship `json:"radio,omitempty"`
	SimilarTracks   TrackRelationship `json:"similarTracks,omitempty"`
	Lyrics          TrackRelationship `json:"lyrics,omitempty"`
	TrackStatistics TrackRelationship `json:"trackStatistics,omitempty"`
}

// TrackRelationship represents a relationship with data and links
type TrackRelationship struct {
	Data  []ResourceIdentifier `json:"data,omitempty"`
	Links RelationshipLinks    `json:"links,omitempty"`
}

// TrackDataLinks contains self link for track data
type TrackDataLinks struct {
	Self string `json:"self"`
}

// TrackIncluded represents included resources (albums, artists, providers, etc.)
type TrackIncluded struct {
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Attributes    TrackIncludedAttributes    `json:"attributes,omitempty"`
	Relationships TrackIncludedRelationships `json:"relationships,omitempty"`
	Links         TrackIncludedLinks         `json:"links,omitempty"`
}

// TrackIncludedAttributes contains attributes for included resources
type TrackIncludedAttributes struct {
	// Album attributes
	Title             string                 `json:"title,omitempty"`
	Barcodes          []string               `json:"barcodes,omitempty"`
	Duration          string                 `json:"duration,omitempty"`
	NumberOfItems     int                    `json:"numberOfItems,omitempty"`
	NumberOfVolumes   int                    `json:"numberOfVolumes,omitempty"`
	ReleaseDate       string                 `json:"releaseDate,omitempty"`
	Copyright         TrackIncludedCopyright `json:"copyright,omitempty"`
	MediaMetadataTags []string               `json:"mediaMetadataTags,omitempty"`
	ExternalLinks     []TrackIncludedExtLink `json:"externalLinks,omitempty"`
	Popularity        float64                `json:"popularity,omitempty"`
	Explicit          bool                   `json:"explicit,omitempty"`

	// Artist attributes
	Name    string `json:"name,omitempty"`
	Country string `json:"country,omitempty"`

	// Image/CoverArt attributes
	Files     []TrackIncludedFile `json:"files,omitempty"`
	MediaType string              `json:"mediaType,omitempty"`

	// Lyrics attributes
	Text         string `json:"text,omitempty"`
	SubTitle     string `json:"subTitle,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
	ProviderId   string `json:"providerId,omitempty"`
	RightHolder  string `json:"rightHolder,omitempty"`

	// Track statistics
	Streams   int64     `json:"streams,omitempty"`
	Listeners int64     `json:"listeners,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// TrackIncludedCopyright represents copyright information
type TrackIncludedCopyright struct {
	Text string `json:"text"`
}

// TrackIncludedExtLink represents external links in included resources
type TrackIncludedExtLink struct {
	Href string                   `json:"href"`
	Meta TrackIncludedExtLinkMeta `json:"meta"`
}

// TrackIncludedExtLinkMeta contains metadata for external links
type TrackIncludedExtLinkMeta struct {
	Type string `json:"type"`
}

// TrackIncludedFile represents an image/file resource
type TrackIncludedFile struct {
	Href string                `json:"href"`
	Meta TrackIncludedFileMeta `json:"meta"`
}

// TrackIncludedFileMeta contains file metadata like dimensions
type TrackIncludedFileMeta struct {
	Height int `json:"height"`
	Width  int `json:"width"`
}

// TrackIncludedRelationships contains relationships for included resources
type TrackIncludedRelationships struct {
	Albums         TrackIncludedRelationship `json:"albums,omitempty"`
	Artists        TrackIncludedRelationship `json:"artists,omitempty"`
	CoverArt       TrackIncludedRelationship `json:"coverArt,omitempty"`
	Genres         TrackIncludedRelationship `json:"genres,omitempty"`
	Providers      TrackIncludedRelationship `json:"providers,omitempty"`
	Tracks         TrackIncludedRelationship `json:"tracks,omitempty"`
	SimilarAlbums  TrackIncludedRelationship `json:"similarAlbums,omitempty"`
	SimilarArtists TrackIncludedRelationship `json:"similarArtists,omitempty"`
}

// TrackIncludedRelationship represents a single relationship in included resources
type TrackIncludedRelationship struct {
	Data  []ResourceIdentifier `json:"data,omitempty"`
	Links RelationshipLinks    `json:"links,omitempty"`
}

// TrackIncludedLinks contains links for included resources
type TrackIncludedLinks struct {
	Self string `json:"self"`
}

// TrackResponse is the complete API response type for GET /tracks/{id}
type TrackResponse = SuccessResponse[TrackData, TrackIncluded, Links]

func (tc *TidalClient) GetTrack(ctx context.Context, trackId, countryCode string, opts ...RequestOption) (*TrackResponse, error) {
	trackURL := fmt.Sprintf("%stracks/%s", tc.baseURL, trackId)
	allOpts := append([]RequestOption{CountryCode(countryCode)}, opts...)
	params := buildRequestOptions(allOpts...).urlParams.Encode()
	if params != "" {
		trackURL = fmt.Sprintf("%s?%s", trackURL, params)
	}

	var response TrackResponse
	err := tc.get(ctx, trackURL, &response)
	if err != nil {
		log.Println("DEBUG (to remove after): Could not fetch single track from TIDAL")
		return nil, err
	}

	return &response, nil
}
