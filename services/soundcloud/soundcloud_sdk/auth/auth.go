package soundcloud_auth

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

const AuthURL = "https://secure.soundcloud.com/authorize"
const TokenURL = "https://secure.soundcloud.com/oauth/token"

func NewSoundCloudAuthClient(clientId, clientSecret, redirectURL string, scopes ...string) (*Authenticator, error) {
	config := &oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  AuthURL,
			TokenURL: TokenURL,
		},
	}

	return &Authenticator{
		config: config,
	}, nil
}

func (ac *Authenticator) WithScopes(scopes ...string) {
	ac.config.Scopes = scopes
}

// AuthURL returns the auth url when authing on SoundCloud.
// PKCE is required for SoundCloud OAuth 2.1, so you must pass the code_challenge and code_challenge_method options.
func (ac *Authenticator) AuthURL(state string, opts ...oauth2.AuthCodeOption) string {
	return ac.config.AuthCodeURL(state, opts...)
}

func (ac *Authenticator) Token(ctx context.Context, state string, r *http.Request, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	values := r.URL.Query()

	e := values.Get("error")
	if e != "" {
		log.Println("SoundCloud auth error...")
		return nil, errors.New("soundcloud client: auth failed - " + e)
	}

	code := values.Get("code")
	if code == "" {
		return nil, errors.New("soundcloud client: auth failed. could not get access code")
	}

	realState := values.Get("state")
	if realState != state {
		return nil, errors.New("soundcloud client: redirect state parameters dont match")
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
