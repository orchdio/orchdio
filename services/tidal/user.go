package tidal

import (
	"context"
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/tidal/tidal_v2"
	tidal_auth "orchdio/services/tidal/tidal_v2/auth"
	"orchdio/util"
	"os"
	"time"

	"golang.org/x/oauth2"
)

func (s *Service) Tokens(authInfo blueprint.UserAuthInfoForRequests) (*tidal_auth.Authenticator, *oauth2.Token, error) {
	auth, err := tidal_auth.NewTidalAuthClient(
		s.IntegrationCredentials.AppID,
		s.IntegrationCredentials.AppSecret,
		"",
	)

	if err != nil {
		log.Println("Could not create new instance of tidal auth client in the tidal user space")
		return nil, nil, err
	}

	hasExpired, err := util.HasTokenExpired(authInfo.ExpiresIn)
	if err != nil {
		return nil, nil, err
	}

	var tokens *oauth2.Token

	if hasExpired {
		// refresh the token
		refreshedTokens, err := auth.RefreshToken(context.TODO(), &oauth2.Token{
			RefreshToken: authInfo.RefreshToken,
		})
		if err != nil {
			return nil, nil, err
		}
		tokens = refreshedTokens
		dbN := db.NewDB{DB: s.DB}
		expiresIn := time.Now().Add(time.Hour).Format(time.RFC3339)

		encryptedRefreshToken, err := util.Encrypt([]byte(tokens.RefreshToken), []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			return nil, nil, err
		}

		updateErr := dbN.UpdateUserAppAuthTokens(authInfo.UserID, tokens.AccessToken, expiresIn, IDENTIFIER, encryptedRefreshToken)
		if updateErr != nil {
			return nil, nil, err
		}
	} else {
		tokens = &oauth2.Token{
			AccessToken:  authInfo.AccessToken,
			RefreshToken: authInfo.RefreshToken,
		}
	}

	return auth, tokens, nil
}

func (s *Service) FetchUserInfo(authInfo blueprint.UserAuthInfoForRequests) (*blueprint.UserPlatformInfo, error) {

	auth, tokens, err := s.Tokens(authInfo)
	if err != nil {
		log.Println("DEBUG: could not fetch valid tokens for tidal user.")
		return nil, err
	}

	authClient := auth.Client(context.Background(), tokens)
	client := tidal_v2.NewTidalClient(authClient)

	user, err := client.CurrentUser(context.Background())
	if err != nil {
		return nil, err
	}

	var userProfile = blueprint.UserPlatformInfo{
		Platform:       "tidal",
		Username:       user.Data.Attributes.Username,
		ProfilePicture: "",
		Followers:      0,
		PlatformID:     user.Data.ID,
		Url:            fmt.Sprintf("https://tidal.com/users/%s", user.Data.ID),
	}

	return &userProfile, nil
}
