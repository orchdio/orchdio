package deezer

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/vicanso/go-axios"
	"log"
	"net/http"
)

type Deezer struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

func (d *Deezer) FetchAuthURL() string {
	permissions := fmt.Sprintf("%s,%s,%s,%s", "basic_access", "email", "manage_library", "delete_library")
	uniqueID, _ := uuid.NewUUID()
	return fmt.Sprintf("%s/auth.php?app_id=%s&redirect_uri=%s&perms=%s&state=%s", AuthBase, d.ClientID, d.RedirectURI, permissions, uniqueID.String())
}

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
	if err != nil {
		log.Printf("\n[services][deezer][auth][FetchAccessToken] Error retrieving deezer auth token - %d\n", unmarshalErr)
	}

	return []byte(authResponse.AccessToken)
}

func (d *Deezer) CompleteUserAuth(token []byte) (DeezerUser, error) {
	t := string(token)
	url := fmt.Sprintf("%s/user/me?access_token=%s", ApiBase, t)

	resp, err := axios.Get(url)
	if err != nil {
		log.Printf("\n[services][deezer][auth][CompleteUserAuth] Deezer auth returned %d\n", err)
		return DeezerUser{}, err
	}
	deezerUser := DeezerUser{}
	if resp.Status != http.StatusOK {
		log.Printf("\n[services][deezer][auth][CompleteUserAuth] Fetching user profile returns status: %d\n", resp.Status)
		return DeezerUser{}, errors.New(string(rune(resp.Status)))
	}

	unmarshalErr := json.Unmarshal(resp.Data, &deezerUser)
	if unmarshalErr != nil {
		log.Printf("\n[services][deezer][auth][CompleteUserAuth] Deezer auth returned %d\n", unmarshalErr)
		return DeezerUser{}, err
	}

	return deezerUser, nil
}
