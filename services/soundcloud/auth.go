package soundcloud

import (
	"context"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/constants"
	"orchdio/services/soundcloud/soundcloud_sdk"
	soundcloud_auth "orchdio/services/soundcloud/soundcloud_sdk/auth"
	"orchdio/util"
	"os"
	"strconv"

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

func CompleteUserAuth(ctx context.Context, request *http.Request, redirectURL string, integrationCredentials *blueprint.IntegrationCredentials, rawCodeVerifier string) (*soundcloud_sdk.UserProfile, *oauth2.Token, *blueprint.UserAuthCredentials, error) {
	if redirectURL == "" {
		log.Println("[account][auth][soundcloud] error - Redirect URI is empty")
		return nil, nil, nil, errors.New("redirect URI is empty")
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
		return nil, nil, nil, err
	}

	token, err := scAuth.Token(ctx, state, request, oauth2.VerifierOption(rawCodeVerifier))
	if err != nil {
		log.Println("Could not fetch soundcloud auth token for user in complete auth")
		return nil, nil, nil, err
	}

	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	encryptedRefreshToken, rErr := util.Encrypt([]byte(token.RefreshToken), []byte(encryptionSecretKey))
	if rErr != nil {
		log.Println("Could not encrypt soundcloud refresh token")
		return nil, nil, nil, errors.New("unable to encrypt refresh token")
	}

	scClient := soundcloud_sdk.NewSoundcloudClient(scAuth.Client(request.Context(), token))
	user, err := scClient.CurrentUser(ctx)
	if err != nil {
		log.Println("Could not fetch soundcloud user in complete auth")
		return nil, nil, nil, err
	}

	soundcloudURN, pErr := soundcloud_sdk.ParseURN(user.URN)
	if pErr != nil {
		log.Println("Could not parse soundcloud ID from URN")
		return nil, nil, nil, pErr
	}

	userCreds := &blueprint.UserAuthCredentials{
		Username:   user.Username,
		PlatformId: strconv.FormatInt(soundcloudURN.ID, 10),
		Platform:   constants.SoundCloudIdentifier,
		Token:      encryptedRefreshToken,
	}
	return user, token, userCreds, nil

}
