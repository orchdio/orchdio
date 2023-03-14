package spotify

import (
	"context"
	"github.com/samber/lo"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"log"
	"net/url"
	"orchdio/blueprint"
)

func FetchLibraryAlbums(refreshToken string) ([]blueprint.LibraryAlbum, error) {
	log.Printf("[spotify][FetchLibraryAlbums] info - Fetching user library albums from Spotify")

	httpClient := spotifyauth.New().Client(context.Background(), &oauth2.Token{RefreshToken: refreshToken})
	client := spotify.New(httpClient)
	values := url.Values{}
	values.Set("limit", "50")

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
			TrackCount:  album.FullAlbum.Tracks.Total,
			Artists: lo.Map(album.Artists, func(artist spotify.SimpleArtist, _ int) string {
				return artist.Name
			}),
			Cover: cover,
		})
	}
	return albums, nil
}
