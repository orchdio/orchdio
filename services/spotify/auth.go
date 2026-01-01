package spotify

import (
	"context"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/constants"
	"orchdio/util"
	"os"

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
		spotifyauth.WithScopes(scopes...),
		spotifyauth.WithClientID(integrationCredentials.AppID),
		spotifyauth.WithClientSecret(integrationCredentials.AppSecret))
	url := auth.AuthURL(state)
	return []byte(url), nil
}

// CompleteUserAuth finishes authorizing a spotify user
func CompleteUserAuth(ctx context.Context, request *http.Request, redirectURL string,
	integrationCredentials *blueprint.IntegrationCredentials, rawCodeVerifier string) (*oauth2.Token, *spotify.PrivateUser, *blueprint.UserAuthCredentials, error) {
	if redirectURL == "" {
		log.Printf("[account][auth][spotify] error - Redirect URI is empty")
		return nil, nil, nil, errors.New("redirect URI is empty")
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
		log.Printf("[services][auth][spotify] error - Error getting user refresh and access tokens: %v", err.Error())
		if strings.Contains(err.Error(), "invalid_grant") {
			return nil, nil, nil, errors.New("invalid grant")
		}
		if strings.Contains(err.Error(), "invalid_client") {
			return nil, nil, nil, errors.New("invalid client")
		}
		return nil, nil, nil, blueprint.ErrInvalidAuthCode
	}

	client := spotify.New(auth.Client(request.Context(), token))
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")

	encryptedRefreshToken, rErr := util.Encrypt([]byte(token.RefreshToken), []byte(encryptionSecretKey))
	if rErr != nil {
		log.Println("[controllers][CompleteUserAuth] developer -  error: unable to encrypt spotify refresh token", rErr)
		return nil, nil, nil, errors.New("unable to encrypt refresh token")
	}

	user, uErr := client.CurrentUser(ctx)
	if uErr != nil {
		if strings.Contains(uErr.Error(), blueprint.ErrSpotifyUserNotRegistered) {
			log.Println("[controllers][HandleAppAuthRedirect] developer -  error: user not registered", uErr)
			return nil, nil, nil, uErr
		}
		log.Println("[controllers][HandleAppAuthRedirect] developer -  error: unable to get current user during auth", uErr)
		return nil, nil, nil, uErr
	}

	userCreds := &blueprint.UserAuthCredentials{
		Username:   user.DisplayName,
		Platform:   constants.SpotifyIdentifier,
		PlatformId: user.ID,
		Token:      encryptedRefreshToken,
	}

	return token, user, userCreds, nil
}
