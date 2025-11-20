package deezer

import (
	"fmt"
	"log"
	"orchdio/blueprint"
	"os"
	"strconv"
)

// FetchLibraryAlbums fetches all the deezer library albums for a user
func (s *Service) FetchLibraryAlbums(token string) ([]blueprint.LibraryAlbum, error) {
	log.Printf("\n[services][deezer][FetchLibraryAlbums] Fetching user deezer albums\n")
	deezerApiBase := os.Getenv("DEEZER_API_BASE")
	reqURL := fmt.Sprintf("%s/user/me/albums?access_token=%s", deezerApiBase, token)
	var albumsResponse UserLibraryAlbumResponse

	err := s.MakeRequest(reqURL, &albumsResponse)
	if err != nil {
		log.Printf("\n[services][deezer][FetchLibraryAlbums] error - Could not fetch user albums: %v\n", err)
	}
	var albums []blueprint.LibraryAlbum
	for _, album := range albumsResponse.Data {
		albums = append(albums, blueprint.LibraryAlbum{
			ID:          strconv.Itoa(album.Id),
			Title:       album.Title,
			URL:         album.Link,
			ReleaseDate: album.ReleaseDate,
			Explicit:    album.ExplicitLyrics,
			TrackCount:  album.NbTracks,
			Artists:     []string{album.Artist.Name},
			Cover:       album.Cover,
		})
	}
	return albums, nil
}
