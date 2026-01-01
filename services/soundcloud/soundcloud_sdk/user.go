package soundcloud_sdk

import (
	"context"
)

// UserProfile represents a SoundCloud user profile
type UserProfile struct {
	AvatarURL             string         `json:"avatar_url"`
	City                  string         `json:"city"`
	CommentsCount         int            `json:"comments_count"`
	Country               string         `json:"country"`
	CreatedAt             string         `json:"created_at"`
	Description           string         `json:"description"`
	DiscogsName           *string        `json:"discogs_name"`
	FirstName             string         `json:"first_name"`
	FollowersCount        int            `json:"followers_count"`
	FollowingsCount       int            `json:"followings_count"`
	FullName              string         `json:"full_name"`
	ID                    float64        `json:"id"`
	Kind                  string         `json:"kind"`
	LastModified          string         `json:"last_modified"`
	LastName              string         `json:"last_name"`
	LikesCount            int            `json:"likes_count"`
	Locale                string         `json:"locale"`
	MyspaceName           *string        `json:"myspace_name"`
	Online                bool           `json:"online"`
	Permalink             string         `json:"permalink"`
	PermalinkURL          string         `json:"permalink_url"`
	Plan                  string         `json:"plan"`
	PlaylistCount         int            `json:"playlist_count"`
	PrimaryEmailConfirmed bool           `json:"primary_email_confirmed"`
	PrivatePlaylistsCount int            `json:"private_playlists_count"`
	PrivateTracksCount    int            `json:"private_tracks_count"`
	PublicFavoritesCount  int            `json:"public_favorites_count"`
	Quota                 Quota          `json:"quota"`
	RepostsCount          int            `json:"reposts_count"`
	Subscriptions         []Subscription `json:"subscriptions"`
	TrackCount            int            `json:"track_count"`
	UploadSecondsLeft     *int           `json:"upload_seconds_left"`
	URI                   string         `json:"uri"`
	URN                   string         `json:"urn"`
	Username              string         `json:"username"`
	Website               *string        `json:"website"`
	WebsiteTitle          *string        `json:"website_title"`
}

// Quota represents upload quota information for a user
type Quota struct {
	UnlimitedUploadQuota bool `json:"unlimited_upload_quota"`
	UploadSecondsLeft    *int `json:"upload_seconds_left"`
	UploadSecondsUsed    int  `json:"upload_seconds_used"`
}

// Subscription represents a user's subscription
type Subscription struct {
	Product   Product `json:"product"`
	Recurring bool    `json:"recurring"`
}

// Product represents a SoundCloud product/plan
type Product struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (sc *SoundcloudClient) CurrentUser(ctx context.Context) (*UserProfile, error) {
	userUrl := sc.baseURL + "me"
	var currUserProfile UserProfile

	err := sc.get(ctx, userUrl, &currUserProfile)
	if err != nil {
		return nil, err
	}
	return &currUserProfile, nil
}
