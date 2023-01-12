package spotify

import (
	"context"
	"encoding/json"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"log"
	"net/http"
	"orchdio/blueprint"
	"os"
)

// FetchAuthURL fetches the auth url
func FetchAuthURL(state string) []byte {
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")
	//if src == "orchdio" {
	//	redirectURI = os.Getenv("ORCHDIO_SPOTIFY_REDIRECT_URI")
	//}
	var auth = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI),
		// TODO: update the scopes as I need them
		spotifyauth.WithScopes(spotifyauth.ScopeUserReadPrivate,
			spotifyauth.ScopeUserLibraryRead,
			spotifyauth.ScopePlaylistReadPrivate,
			spotifyauth.ScopePlaylistReadCollaborative,
			spotifyauth.ScopeUserFollowRead,
			spotifyauth.ScopePlaylistModifyPrivate,
			spotifyauth.ScopePlaylistModifyPublic,
			spotifyauth.ScopeUserReadEmail))
	url := auth.AuthURL(state)
	return []byte(url)
}

// CompleteUserAuth finishes authorizing a spotify user
func CompleteUserAuth(ctx context.Context, request *http.Request) (*spotify.Client, []byte, error) {
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")

	//if src == "orchdio" {
	//	redirectURI = os.Getenv("ORCHDIO_SPOTIFY_REDIRECT_URI")
	//}
	state := request.FormValue("state")
	auth := spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI))

	token, err := auth.Token(ctx, state, request)
	if err != nil {
		// TODO: handle auth error here. instead of ending up throwing a 500, just return accordingly
		log.Printf("[account][auth][spotify] error - Error getting authorized token %v", err.Error())
		var errorResponse map[string]string
		err := json.Unmarshal([]byte(err.Error()), &errorResponse)
		if err != nil {
			log.Printf("[account][auth][spotify] error - Error unmarshalling error response %v", err.Error())
			return nil, nil, err
		}
		if errorResponse["error"] == "invalid_grant" {
			return nil, nil, err
		}
		return nil, nil, blueprint.EINVALIDAUTHCODE
	}

	client := spotify.New(auth.Client(request.Context(), token))
	return client, []byte(token.RefreshToken), nil
}
