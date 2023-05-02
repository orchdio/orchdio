package deezer

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vicanso/go-axios"
	"log"
	"net/http"
	"net/url"
	"orchdio/blueprint"
	"strings"
)

const (
	ValidScopes = "basic_access,email,manage_library,delete_library,offline_access,listening_history"
)

// Deezer represents a deezer instance.
type Deezer struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// NewDeezerAuth returns a new deezer auth instance.
func NewDeezerAuth(clientID, clientSecret, redirectURI string) *Deezer {
	return &Deezer{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
	}
}

// FetchAuthURL fetches the auth url.
func (d *Deezer) FetchAuthURL(scopes []string) string {
	// scopes := "basic_access", "email", "manage_library", "delete_library", "offline_access", "listening_history"
	permissions := fmt.Sprintf("%s", strings.Join(scopes, ","))
	return fmt.Sprintf("%s/auth.php?app_id=%s&redirect_uri=%s&perms=%s", AuthBase, d.ClientID, d.RedirectURI, url.QueryEscape(permissions))
}

// FetchAccessToken fetches the access token.
func (d *Deezer) FetchAccessToken(code string) []byte {
	// first, extract the "code" param from the url
	authURL := fmt.Sprintf("%s/access_token.php?app_id=%s&secret=%s&code=%s&output=json", AuthBase, d.ClientID, d.ClientSecret, code)
	resp, err := axios.Get(authURL)
	if err != nil {
		log.Printf("\n[services][deezer][auth][FetchAccessToken] Error fetching access token from Deezer - %v\n", err)
		return nil
	}
	if resp.Status != http.StatusOK {
		log.Printf("\n[services][deezer][auth][FetchAccessToken] Deezer auth returned %d\n", resp.Status)
		return nil
	}
	authResponse := AuthResponse{}
	unmarshalErr := json.Unmarshal(resp.Data, &authResponse)
	if unmarshalErr != nil {
		log.Printf("\n[services][deezer][auth][FetchAccessToken] Error retrieving deezer auth token: could not deserialize response - %d\n", unmarshalErr)
	}

	return []byte(authResponse.AccessToken)
}

// CompleteUserAuth completes a user's auth process. It will return an error if the user's auth process has not been completed.
// and a deezer user object if the auth process has been completed.
func (d *Deezer) CompleteUserAuth(token []byte) (*DeezerUser, error) {
	t := string(token)
	link := fmt.Sprintf("%s/user/me?access_token=%s", ApiBase, t)

	resp, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][auth][CompleteUserAuth] Deezer auth returned %d\n", err)
		return nil, err
	}
	deezerUser := DeezerUser{}
	if resp.Status != http.StatusOK {
		log.Printf("\n[services][deezer][auth][CompleteUserAuth] Fetching user profile returns status: %d\n", resp.Status)
		return nil, errors.New(string(rune(resp.Status)))
	}

	// more deezer shenanigans:
	// it seems the free service is closed to api calls too. so if say the (vpn) ip address of the server
	// is in frankfurt, it'll work because free is still active in germany. but say nigeria or south africa, it doesnt work
	// and returns "free service closed" error
	if strings.Contains(string(resp.Data), "free service is closed") {
		log.Printf("[services][deezer][auth][CompleteUserAuth][warning] - deezer access blocked due to free service shutdown")
		return nil, blueprint.ESERVICECLOSED
	}

	unmarshalErr := json.Unmarshal(resp.Data, &deezerUser)
	if unmarshalErr != nil {
		log.Printf("\n[services][deezer][auth][CompleteUserAuth] Deezer auth returned %d\n", unmarshalErr)
		return nil, err
	}

	// if we get here and the user data returned is null, email would be empty. this is somewhat a hack
	// but no way around it thanks to the deezer api response (ideally this should've thrown an error as this
	// means its probably a permission issue i.e. the developer passed the wrong permissions).
	if deezerUser.Email == "" {
		log.Printf("\n[services][deezer][auth][CompleteUserAuth][warning] - authenticated deezer user seems to be empty. Might to be a permission issue. %d\n", unmarshalErr)
		return nil, blueprint.EINVALIDPERMISSIONS
	}

	return &deezerUser, nil
}
