package soundcloud_sdk

import (
	"context"
	"fmt"
	"log"
)

type Track struct {
	Kind                  string    `json:"kind"`
	ID                    int64     `json:"id"`
	URN                   string    `json:"urn"`
	CreatedAt             string    `json:"created_at"`
	Duration              int       `json:"duration"`
	Commentable           bool      `json:"commentable"`
	CommentCount          int       `json:"comment_count"`
	Sharing               string    `json:"sharing"`
	TagList               string    `json:"tag_list"`
	Streamable            bool      `json:"streamable"`
	EmbeddableBy          string    `json:"embeddable_by"`
	PurchaseURL           *string   `json:"purchase_url"`
	PurchaseTitle         *string   `json:"purchase_title"`
	Genre                 string    `json:"genre"`
	Title                 string    `json:"title"`
	Description           string    `json:"description"`
	LabelName             *string   `json:"label_name"`
	Release               *string   `json:"release"`
	KeySignature          *string   `json:"key_signature"`
	ISRC                  string    `json:"isrc"`
	BPM                   *float64  `json:"bpm"`
	ReleaseYear           *int      `json:"release_year"`
	ReleaseMonth          *int      `json:"release_month"`
	ReleaseDay            *int      `json:"release_day"`
	License               string    `json:"license"`
	URI                   string    `json:"uri"`
	User                  User      `json:"user"`
	PermalinkURL          string    `json:"permalink_url"`
	ArtworkURL            string    `json:"artwork_url"`
	StreamURL             string    `json:"stream_url"`
	DownloadURL           *string   `json:"download_url"`
	WaveformURL           string    `json:"waveform_url"`
	AvailableCountryCodes *[]string `json:"available_country_codes"`
	SecretURI             *string   `json:"secret_uri"`
	UserFavorite          *bool     `json:"user_favorite"`
	UserPlaybackCount     *int      `json:"user_playback_count"`
	PlaybackCount         int       `json:"playback_count"`
	DownloadCount         int       `json:"download_count"`
	FavoritingsCount      int       `json:"favoritings_count"`
	RepostsCount          int       `json:"reposts_count"`
	Downloadable          bool      `json:"downloadable"`
	Access                string    `json:"access"`
	Policy                *string   `json:"policy"`
	MonetizationModel     *string   `json:"monetization_model"`
	MetadataArtist        *string   `json:"metadata_artist"`
}

// todo: soundcloud GetTrack endpoint takes a secret token to get a track thats private (ostensibly). ensure to pass this along when
// implementing for private resources.
func (sc *SoundcloudClient) GetTrack(ctx context.Context, trackId string) (*Track, error) {
	trackURL := fmt.Sprintf("%stracks/%s", sc.baseURL, trackId)

	var response Track
	err := sc.get(ctx, trackURL, &response)
	if err != nil {
		log.Printf("Could not get single soundcloud track with id: %s", trackId)
		return nil, err
	}

	return &response, nil
}
