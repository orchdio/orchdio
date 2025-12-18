package tidal

import (
	"context"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/constants"
	"orchdio/services/tidal/tidal_v2"
	tidal_auth "orchdio/services/tidal/tidal_v2/auth"
	"orchdio/util"
	"os"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

func FetchAuthURL(state, redirectURL string, scopes []string,
	integrationCredentials *blueprint.IntegrationCredentials, rawVerifier string) ([]byte, error) {

	log.Println("The generated code challenge to be used for auth is", rawVerifier)

	var auth, err = tidal_auth.NewTidalAuthClient(integrationCredentials.AppID,
		integrationCredentials.AppSecret,
		redirectURL,
		scopes...,
	)

	if err != nil {
		log.Println("[services][auth][TIDAL] FetchAuthURL - could not create new tidal auth client")
		return nil, err
	}

	url := auth.AuthURL(state,
		oauth2.S256ChallengeOption(rawVerifier),
		// oauth2.SetAuthURLParam("code_challenge_method", "256"),
		// oauth2.SetAuthURLParam("code_challenge", codeChallenge)
	)
	return []byte(url), nil
}

func CompleteUserAuth(ctx context.Context, req *http.Request, redirectURL string, integrationCredentials *blueprint.IntegrationCredentials, verifier string) (*tidal_v2.UserProfile, *oauth2.Token, *blueprint.UserAuthCredentials, error) {

	if redirectURL == "" {
		log.Printf("[services][auth][tidal] error - Redirect URI is empty")
		return nil, nil, nil, errors.New("redirect URI is empty")
	}

	state := req.FormValue("state")
	tAuth, err := tidal_auth.NewTidalAuthClient(
		integrationCredentials.AppID,
		integrationCredentials.AppSecret,
		redirectURL,
	)

	if err != nil {
		log.Println("[services][auth][TIDAL] CompleteUserAuth - could not create new tidal auth client")
		log.Println(err)
		return nil, nil, nil, err
	}

	token, err := tAuth.Token(ctx, state, req,
		oauth2.VerifierOption(verifier),
		// oauth2.SetAuthURLParam("code_verifier", verifier)
	)
	if err != nil {
		log.Println("[services][auth][TIDAL] CompleteUserAuth - could not fetch oauth token from request")
		log.Println(err)
		return nil, nil, nil, err
	}

	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	encryptedRefreshToken, rErr := util.Encrypt([]byte(token.RefreshToken), []byte(encryptionSecretKey))
	if rErr != nil {
		log.Println("Could not encrypt refresh token during user auth completion on TIDAL")
		return nil, nil, nil, err
	}

	tClient := tidal_v2.NewTidalClient(tAuth.Client(req.Context(), token))
	// get the current user
	user, err := tClient.CurrentUser(ctx)
	if err != nil {
		log.Println("[controllers][CompleteUserAuth] developer -  error: unable to fetch tidal user", zap.Error(err))
		return nil, nil, nil, err
	}

	userCreds := &blueprint.UserAuthCredentials{
		Username:   user.Data.Attributes.Username,
		Platform:   constants.TidalIdentifier,
		PlatformId: user.Data.ID,
		Token:      encryptedRefreshToken,
	}
	return user, token, userCreds, nil
}
