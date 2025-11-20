package spotify

import (
	"context"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"

	"strings"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

// FetchAuthURL fetches the auth url
// to fetch a platform auth url, we pass the redirect url for the integrated platform app. in this case,
// its always "https://orchdio.com/v1/auth/:platform/callback", where platform here is spotify. this is different from the redirect url
// which is orchdio's and used after the user has been authenticated on the platform and email notification job scheduled, at the end of
// the auth flow.
func FetchAuthURL(state, redirectURL string, scopes []string,
	integrationCredentials *blueprint.IntegrationCredentials, verifier string) ([]byte, error) {

	var auth = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURL),
		spotifyauth.WithScopes(scopes...), spotifyauth.WithClientID(integrationCredentials.AppID),
		spotifyauth.WithClientSecret(integrationCredentials.AppSecret))
	url := auth.AuthURL(state)
	return []byte(url), nil
}

// CompleteUserAuth finishes authorizing a spotify user
func CompleteUserAuth(ctx context.Context, request *http.Request, redirectURL string,
	integrationCredentials *blueprint.IntegrationCredentials, rawCodeVerifier string) (*spotify.Client, *oauth2.Token, error) {
	if redirectURL == "" {
		log.Printf("[account][auth][spotify] error - Redirect URI is empty")
		return nil, nil, errors.New("redirect URI is empty")
	}
	state := request.FormValue("state")
	auth := spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURL),
		spotifyauth.WithClientID(integrationCredentials.AppID),
		spotifyauth.WithClientSecret(integrationCredentials.AppSecret),
	)
	token, err := auth.Token(ctx, state, request)
	if err != nil {
		// TODO: handle auth error here. instead of ending up throwing a 500, just return accordingly
		log.Printf("[account][auth][spotify] error - Error getting user refresh and access tokens: %v", err.Error())
		if strings.Contains(err.Error(), "invalid_grant") {
			return nil, nil, errors.New("invalid grant")
		}
		if strings.Contains(err.Error(), "invalid_client") {
			return nil, nil, errors.New("invalid client")
		}
		return nil, nil, blueprint.ErrInvalidAuthCode
	}

	client := spotify.New(auth.Client(request.Context(), token))
	return client, token, nil
}
