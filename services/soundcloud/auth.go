package soundcloud

import (
	"context"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services/soundcloud/soundcloud_sdk"
	soundcloud_auth "orchdio/services/soundcloud/soundcloud_sdk/auth"

	"golang.org/x/oauth2"
)

func FetchAuthURL(state, redirectURL string, scopes []string, integrationCredentials *blueprint.IntegrationCredentials, rawVerifier string) ([]byte, error) {

	authR, err := soundcloud_auth.NewSoundCloudAuthClient(
		integrationCredentials.AppID,
		integrationCredentials.AppSecret,
		redirectURL,
		scopes...,
	)

	if err != nil {
		log.Println("[services][auth][SoundCloud] FetchAuthURL - could not create new soundcloud auth client")
		return nil, err
	}

	url := authR.AuthURL(state, oauth2.S256ChallengeOption(rawVerifier))

	return []byte(url), nil
}

func CompleteUserAuth(ctx context.Context, request *http.Request, redirectURL string, integrationCredentials *blueprint.IntegrationCredentials, rawCodeVerifier string) (*soundcloud_sdk.SoundcloudClient, *oauth2.Token, error) {
	if redirectURL == "" {
		log.Println("[account][auth][soundcloud] error - Redirect URI is empty")
		return nil, nil, errors.New("redirect URI is empty")
	}

	state := request.FormValue("state")
	scAuth, err := soundcloud_auth.NewSoundCloudAuthClient(
		integrationCredentials.AppID,
		integrationCredentials.AppSecret,
		redirectURL,
	)
	if err != nil {
		log.Println("[services][auth][SoundCloud] CompleteUserAuth - could not create new soundcloud auth client")
		log.Println(err)
		return nil, nil, err
	}

	token, err := scAuth.Token(ctx, state, request, oauth2.VerifierOption(rawCodeVerifier))

	scClient := soundcloud_sdk.NewSoundcloudClient(scAuth.Client(request.Context(), token))
	return scClient, token, nil

}
