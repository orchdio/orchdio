package tidal

import (
	"context"
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/services/tidal/tidal_v2"
	tidal_auth "orchdio/services/tidal/tidal_v2/auth"

	"golang.org/x/oauth2"
)

func (s *Service) FetchUserInfo(refreshToken string) (*blueprint.UserPlatformInfo, error) {

	auth, err := tidal_auth.NewTidalAuthClient(
		s.IntegrationCredentials.AppID,
		s.IntegrationCredentials.AppSecret,
		"",
	)

	if err != nil {
		log.Println("Could not create new instance of tidal auth client in the tidal user space")
		return nil, err
	}

	client := tidal_v2.NewTidalClient(
		auth.Client(context.Background(), &oauth2.Token{
			AccessToken: refreshToken,
		}))

	// user info
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Println("Could not fetch current user....")
		return nil, err
	}

	var userProfile = blueprint.UserPlatformInfo{
		Platform:       "tidal",
		Username:       user.Attributes.Username,
		ProfilePicture: "",
		Followers:      0,
		PlatformID:     user.ID,
		Url:            fmt.Sprintf("https://tidal.com/artist/%s", user.ID),
	}

	log.Println("Fetched user profile from new TIDAL setup")

	return &userProfile, nil
}
