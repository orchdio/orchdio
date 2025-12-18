package soundcloud_auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// ClientCredentials handles the Client Credentials flow for accessing public SoundCloud resources.
// This flow is used when your application needs to access public resources only (searching, playback, URL resolution)
// without user authorization.
type ClientCredentials struct {
	clientID     string
	clientSecret string
}

// NewClientCredentials creates a new ClientCredentials instance
func NewClientCredentials(clientID, clientSecret string) *ClientCredentials {
	return &ClientCredentials{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// Token requests an access token using the client credentials flow.
// SoundCloud requires Basic Authentication header for this flow.
func (cc *ClientCredentials) Token(ctx context.Context) (*oauth2.Token, error) {
	// Create Basic Auth header
	auth := base64.StdEncoding.EncodeToString([]byte(cc.clientID + ":" + cc.clientSecret))

	// Prepare form data
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json; charset=utf-8")

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status: %d", resp.StatusCode)
	}

	// Parse response using oauth2 package
	token := &oauth2.Token{}
	if err := json.NewDecoder(resp.Body).Decode(token); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return token, nil
}

// Client returns an HTTP client that automatically handles authentication
// using the client credentials token.
func (cc *ClientCredentials) Client(ctx context.Context) (*http.Client, error) {
	token, err := cc.Token(ctx)
	if err != nil {
		return nil, err
	}

	// Create a token source that can refresh the token
	src := oauth2.StaticTokenSource(token)
	return oauth2.NewClient(ctx, src), nil
}
