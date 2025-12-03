package tidal

import (
	"context"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services/tidal/tidal_v2"
	tidal_auth "orchdio/services/tidal/tidal_v2/auth"

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

func CompleteUserAuth(ctx context.Context, req *http.Request, redirectURL string, integrationCredentials *blueprint.IntegrationCredentials, verifier string) (*tidal_v2.TidalClient, *oauth2.Token, error) {

	if redirectURL == "" {
		log.Printf("[services][auth][tidal] error - Redirect URI is empty")
		return nil, nil, errors.New("redirect URI is empty")
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
		return nil, nil, err
	}

	token, err := tAuth.Token(ctx, state, req,
		oauth2.VerifierOption(verifier),
		// oauth2.SetAuthURLParam("code_verifier", verifier)
	)
	if err != nil {
		log.Println("[services][auth][TIDAL] CompleteUserAuth - could not fetch oauth token from request")
		log.Println(err)
		return nil, nil, err
	}

	tClient := tidal_v2.NewTidalClient(tAuth.Client(req.Context(), token))
	return tClient, token, nil
}
