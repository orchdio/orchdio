package spotify

import (
	"context"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"log"
	"net/http"
	"os"
)

// FetchAuthURL fetches the auth url
func FetchAuthURL(state string) []byte {
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")
	var auth = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI),
		// TODO: update the scopes as I need them
		spotifyauth.WithScopes(spotifyauth.ScopeUserReadPrivate,
			spotifyauth.ScopeUserLibraryRead,
			spotifyauth.ScopeUserReadEmail))
	url := auth.AuthURL(state)
	return []byte(url)
}

// CompleteUserAuth finishes authorizing a spotify user
func CompleteUserAuth(ctx context.Context, request *http.Request) (*spotify.Client, []byte) {
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")
	state := request.FormValue("state")
	auth := spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(spotifyauth.ScopeUserReadPrivate,
			spotifyauth.ScopeUserLibraryRead,
			spotifyauth.ScopeUserReadEmail))

	token, err := auth.Token(ctx, state, request)
	if err != nil {
		log.Printf("[account][auth][spotify] error - Error getting authorized token %v", err)
		return nil, nil
	}

	client := spotify.New(auth.Client(request.Context(), token))
	return client, []byte(token.RefreshToken)
}
