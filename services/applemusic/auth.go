package applemusic

import (
	"fmt"
	"net/url"
	"orchdio/blueprint"
	"strings"
)

func FetchAuthURL(state, redirectURL string, scopes []string, integrationCredentials *blueprint.IntegrationCredentials) ([]byte, error) {

	if len(scopes) == 0 {
		return nil, fmt.Errorf("at least one scope must be provided")
	}

	validScopes := map[string]bool{
		"name":  true,
		"email": true,
	}

	for _, scope := range scopes {
		if !validScopes[scope] {
			return nil, fmt.Errorf("invalid scope '%s': only 'name' and 'email' are allowed", scope)
		}
	}

	baseURL := "https://appleid.apple.com/auth/authorize"
	encodedScopes := url.QueryEscape(strings.Join(scopes, " "))
	bURL := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code id_token&state=%s&scope=%s&response_mode=form_post",
		baseURL,
		integrationCredentials.AppID,
		redirectURL,
		state,
		encodedScopes,
	)

	return []byte(bURL), nil
}
