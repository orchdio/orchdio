package tidal

import (
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/util"
	"strconv"
)

func (s *Service) FetchLibraryAlbums(userId string) ([]blueprint.LibraryAlbum, error) {
	log.Printf("[tidal][FetchLibraryAlbums] info - Fetching user library albums from Tidal")
	link := fmt.Sprintf("/users/%s/favorites/albums?offset=0&limit=50&orderDirection=DESC&countryCode=US&locale=en_US&deviceType=BROWSER", userId)

	var albumResponse UserLibraryAlbumResponse
	err := s.MakeRequest(link, &albumResponse)
	if err != nil {
		log.Printf("[tidal][FetchLibraryAlbums] error - %s", err.Error())
	}
	var albums []blueprint.LibraryAlbum

	for _, item := range albumResponse.Items {
		album := item.Item
		var artists []string
		for _, artist := range album.Artists {
			artists = append(artists, artist.Name)
		}
		albums = append(albums, blueprint.LibraryAlbum{
			ID:          strconv.Itoa(album.Id),
			Title:       album.Title,
			URL:         album.Url,
			ReleaseDate: album.ReleaseDate,
			Explicit:    album.Explicit,
			TrackCount:  album.NumberOfTracks,
			Artists:     artists,
			Cover:       util.BuildTidalAssetURL(album.Cover),
		})
	}
	return albums, nil
}
