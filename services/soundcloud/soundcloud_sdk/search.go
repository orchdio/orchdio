package soundcloud_sdk

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
)

type TrackSearch struct {
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
	Description           *string   `json:"description"`
	LabelName             *string   `json:"label_name"`
	Release               *string   `json:"release"`
	KeySignature          *string   `json:"key_signature"`
	ISRC                  *string   `json:"isrc"`
	BPM                   *float64  `json:"bpm"`
	ReleaseYear           *int      `json:"release_year"`
	ReleaseMonth          *int      `json:"release_month"`
	ReleaseDay            *int      `json:"release_day"`
	License               string    `json:"license"`
	URI                   string    `json:"uri"`
	User                  User      `json:"user"`
	PermalinkURL          string    `json:"permalink_url"`
	ArtworkURL            *string   `json:"artwork_url"`
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

type User struct {
	AvatarURL            string         `json:"avatar_url"`
	ID                   int64          `json:"id"`
	URN                  string         `json:"urn"`
	Kind                 string         `json:"kind"`
	PermalinkURL         string         `json:"permalink_url"`
	URI                  string         `json:"uri"`
	Username             string         `json:"username"`
	Permalink            string         `json:"permalink"`
	CreatedAt            string         `json:"created_at"`
	LastModified         string         `json:"last_modified"`
	FirstName            *string        `json:"first_name"`
	LastName             *string        `json:"last_name"`
	FullName             string         `json:"full_name"`
	City                 *string        `json:"city"`
	Description          *string        `json:"description"`
	Country              *string        `json:"country"`
	TrackCount           int            `json:"track_count"`
	PublicFavoritesCount int            `json:"public_favorites_count"`
	RepostsCount         int            `json:"reposts_count"`
	FollowersCount       int            `json:"followers_count"`
	FollowingsCount      int            `json:"followings_count"`
	Plan                 string         `json:"plan"`
	MyspaceName          *string        `json:"myspace_name"`
	DiscogsName          *string        `json:"discogs_name"`
	WebsiteTitle         *string        `json:"website_title"`
	Website              *string        `json:"website"`
	CommentsCount        int            `json:"comments_count"`
	Online               bool           `json:"online"`
	LikesCount           int            `json:"likes_count"`
	PlaylistCount        int            `json:"playlist_count"`
	Subscriptions        []Subscription `json:"subscriptions"`
}

func customPathEscape(query string) string {
	// First apply standard path escaping
	escaped := url.PathEscape(query)

	// Additionally encode & which PathEscape doesn't encode
	escaped = strings.ReplaceAll(escaped, "&", "%26")

	return escaped
}

func (sc *SoundcloudClient) Search(ctx context.Context, query string, opts ...RequestOption) (*[]TrackSearch, error) {

	allOpts := append([]RequestOption{}, opts...)
	reqOpts := buildRequestOptions(allOpts...)

	log.Println("Soundcloud search built request options are")
	for k, v := range reqOpts.urlParams {
		for _, val := range v {
			log.Printf("Key: %s, value: %s", k, val)
		}
	}

	searchUrl := fmt.Sprintf("%stracks?q=%s", sc.baseURL, customPathEscape(query))
	params := reqOpts.urlParams.Encode()
	if params != "" {
		searchUrl = fmt.Sprintf("%s?%s", searchUrl, params)
	}

	log.Println("final request to search with: ", searchUrl)

	var searchResults []TrackSearch
	err := sc.get(ctx, searchUrl, &searchResults)
	if err != nil {
		log.Println("Error searching for track on Soundcloud")
		return nil, err
	}

	return &searchResults, nil
}
