package tidal_auth

import (
	"context"
	"errors"
	"log"
	"net/http"

	"golang.org/x/oauth2"
)

type Authenticator struct {
	config *oauth2.Config
}

const AuthURL = "https://login.tidal.com/authorize"
const TokenURL = "https://auth.tidal.com/v1/oauth2/token"

var showDialog = oauth2.SetAuthURLParam("show_dialog", "true")

func NewTidalAuthClient(clientId, clientSecret, redirectURL string, scopes ...string) (*Authenticator, error) {
	config := &oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  AuthURL,
			TokenURL: TokenURL,
		},
		Scopes: scopes,
	}

	return &Authenticator{
		config: config,
	}, nil
}

// func NewTidalAuthClientWithB

func (ac *Authenticator) WithScopes(scopes ...string) {
	ac.config.Scopes = scopes
}

// AuthURL returns the auth url when authing on tidal. for now, we pass only state
func (ac *Authenticator) AuthURL(state string, opts ...oauth2.AuthCodeOption) string {
	// if showAuthDialog {
	// 	return ac.config.AuthCodeURL(state, showDialog)
	// }

	return ac.config.AuthCodeURL(state, opts...)
}

func (ac *Authenticator) Token(ctx context.Context, state string, r *http.Request, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	values := r.URL.Query()

	e := values.Get("error")
	if e != "" {
		log.Println("TIDAL auth thinggy...")
		return nil, errors.New("tidal client: auth failed - " + e)
	}

	code := values.Get("code")
	if code == "" {
		return nil, errors.New("tidal client: auth failed. could not get access code")
	}

	realState := values.Get("state")
	if realState != state {
		return nil, errors.New("tidal client: redirect state parameters dont match")
	}
	return ac.config.Exchange(ctx, code, opts...)
}

func (ac *Authenticator) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	tokenSrc := ac.config.TokenSource(ctx, token)
	return tokenSrc.Token()
}

func (ac *Authenticator) Client(ctx context.Context, token *oauth2.Token) *http.Client {
	return ac.config.Client(ctx, token)
}
