package spotify

import (
	"context"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
	"orchdio/util"
	"os"
	"time"

	"github.com/samber/lo"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

func (s *Service) RefreshToken(refreshToken, accessToken, userId string) (*oauth2.Token, error) {
	authClient := spotifyauth.New(
		spotifyauth.WithClientID(s.IntegrationAppID),
		spotifyauth.WithClientSecret(s.IntegrationAppSecret),
		spotifyauth.WithRedirectURL(s.App.RedirectURL),
	)

	refTok, err := authClient.RefreshToken(context.Background(), &oauth2.Token{
		RefreshToken: refreshToken,
		AccessToken:  accessToken,
		TokenType:    "Bearer",
	})

	if err != nil {
		log.Println("Error refreshing user spotify refresh token")
		return nil, err
	}

	// encrypt refresh token
	encryptedRefreshToken, err := util.Encrypt([]byte(refTok.RefreshToken), []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Println("Could not encrypt refreshtoken....")
		return nil, err
	}

	expiresIn := time.Now().Add(time.Hour).Format(time.RFC3339)
	// update the refreshtoken in the database
	// todo: use the db method for saving auth tokens.
	_, err = s.PgClient.Exec(queries.UpdateOAuthTokens, encryptedRefreshToken, expiresIn, refTok.AccessToken, userId, "spotify")

	if err != nil {
		log.Println("Could not update refresh token in DB")
		return nil, err
	}

	return refTok, nil
}

func (s *Service) FetchLibraryAlbums(refreshToken string) ([]blueprint.LibraryAlbum, error) {
	log.Printf("[spotify][FetchLibraryAlbums] info - Fetching user library albums from Spotify")
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: refreshToken})
	libraryAlbums, err := client.CurrentUsersAlbums(context.Background(), spotify.Limit(50))
	if err != nil {
		log.Printf("[spotify][FetchLibraryAlbums] error - %s", err.Error())
		return nil, err
	}

	for {
		if libraryAlbums.Next == "" {
			break
		}
		out := spotify.SavedAlbumPage{}
		err = client.NextPage(context.Background(), &out)
		if err == spotify.ErrNoMorePages {
			break
		}
		if err != nil {
			log.Printf("[spotify][FetchLibraryAlbums] error - %s", err.Error())
			return nil, err
		}
		libraryAlbums.Albums = append(libraryAlbums.Albums, out.Albums...)
		libraryAlbums.Next = out.Next
	}

	var albums []blueprint.LibraryAlbum
	for _, album := range libraryAlbums.Albums {
		cover := ""
		if len(album.Images) > 0 {
			cover = album.Images[0].URL
		}
		albums = append(albums, blueprint.LibraryAlbum{
			ID:          album.ID.String(),
			Title:       album.Name,
			URL:         album.ExternalURLs["spotify"],
			ReleaseDate: album.ReleaseDate,
			TrackCount:  int(album.FullAlbum.Tracks.Total),
			Artists: lo.Map(album.Artists, func(artist spotify.SimpleArtist, _ int) string {
				return artist.Name
			}),
			Cover: cover,
		})
	}
	return albums, nil
}
