package soundcloud_sdk

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// BuildURL constructs a full API URL with the given path and query parameters
func BuildURL(baseURL, path string, params url.Values) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	u.Path = path
	if len(params) > 0 {
		u.RawQuery = params.Encode()
	}

	return u.String()
}

// AddAuthorizationHeader adds the OAuth authorization header to the request
func AddAuthorizationHeader(req *http.Request, token string) {
	req.Header.Set("Authorization", "OAuth "+token)
}

// AddStandardHeaders adds common headers for SoundCloud API requests
func AddStandardHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
}

// IsValidAccessLevel checks if the given access level is valid
func IsValidAccessLevel(level AccessLevel) bool {
	switch level {
	case AccessPlayable, AccessPreview, AccessBlocked:
		return true
	default:
		return false
	}
}

// FormatAPIError formats an API error response for better readability
func FormatAPIError(statusCode int, body string) error {
	return fmt.Errorf("soundcloud api error: status %d, body: %s", statusCode, body)
}

// ValidateLimit ensures the limit is within acceptable range
func ValidateLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

// BuildTrackURL constructs a URL for a specific track
func BuildTrackURL(trackID string) string {
	return fmt.Sprintf("%s%s/%s", APIBaseURL, EndpointTracks, trackID)
}

// BuildPlaylistURL constructs a URL for a specific playlist
func BuildPlaylistURL(playlistID string) string {
	return fmt.Sprintf("%s%s/%s", APIBaseURL, EndpointPlaylists, playlistID)
}

// BuildUserURL constructs a URL for a specific user
func BuildUserURL(userID string) string {
	return fmt.Sprintf("%s%s/%s", APIBaseURL, EndpointUsers, userID)
}

type URNComponents struct {
	Prefix       string
	ResourceType string
	ID           int64
}

// ParseURN parses a SoundCloud URN and returns all components
func ParseURN(urn string) (*URNComponents, error) {
	if urn == "" {
		return nil, errors.New("URN string is empty")
	}

	parts := strings.Split(urn, ":")

	if len(parts) != 3 {
		return nil, errors.New("invalid URN format: expected 'soundcloud:resource:id'")
	}

	if parts[0] != "soundcloud" {
		return nil, errors.New("invalid URN prefix: expected 'soundcloud'")
	}

	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, errors.New("invalid ID: must be a valid integer")
	}

	return &URNComponents{
		Prefix:       parts[0],
		ResourceType: parts[1],
		ID:           id,
	}, nil
}
