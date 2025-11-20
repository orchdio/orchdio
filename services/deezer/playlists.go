package deezer

import (
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/util"
	"os"
	"strconv"
)

// FetchUserPlaylists fetches all the playlists for a user
func (s *Service) FetchLibraryPlaylists(token string) ([]blueprint.UserPlaylist, error) {
	deezerAPIBase := os.Getenv("DEEZER_API_BASE")
	// DEEZER PLAYLIST LIMIT IS 250 FOR NOW. THIS IS ORCHDIO IMPOSED AND IT IS
	// 1. TO EASE IMPLEMENTATION
	// 2. TO MAKE IT "PREMIUM" IN THE FUTURE  (i.e. if we want to charge for more playlists), makes it easier to enforce/assimilate from now
	reqURL := fmt.Sprintf("%s/user/me/playlists?access_token=%s&limit=250", deezerAPIBase, token)

	out := &UserPlaylistsResponse{}
	err := s.MakeRequest(reqURL, out)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserPlaylists] error - Could not fetch user playlists: %v\n", err)
		return nil, err
	}

	var userPlaylists []blueprint.UserPlaylist
	for _, playlist := range out.Data {
		userPlaylists = append(userPlaylists, blueprint.UserPlaylist{
			ID:            strconv.Itoa(int(playlist.ID)),
			Title:         playlist.Title,
			Duration:      util.GetFormattedDuration(playlist.Duration),
			DurationMilis: playlist.Duration * 1000,
			Public:        playlist.Public,
			Collaborative: playlist.Collaborative,
			NbTracks:      playlist.NbTracks,
			Fans:          playlist.Fans,
			URL:           playlist.Link,
			Cover:         playlist.PictureMedium,
			CreatedAt:     playlist.CreationDate,
			Checksum:      playlist.Checksum,
			Owner:         playlist.Creator.Name,
		})
	}

	return userPlaylists, nil
}
