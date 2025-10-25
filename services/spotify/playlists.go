package spotify

import (
	"context"
	"log"
	"orchdio/blueprint"

	"github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"
)

// FetchUserPlaylist fetches the user's playlist
func (s *Service) FetchLibraryPlaylists(token string) ([]blueprint.UserPlaylist, error) {
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: token})
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), &oauth2.MusicToken{RefreshToken: token})
	//client := spotify.New(httpClient)
	playlists, err := client.CurrentUsersPlaylists(context.Background())
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchUserPlaylist] error - could not fetch playlist: %v\n", err)
		return nil, err
	}
	for {
		out := spotify.SimplePlaylistPage{}
		paginationErr := client.NextPage(context.Background(), &out)
		if paginationErr == spotify.ErrNoMorePages {
			log.Printf("\n[services][spotify][base][FetchUserPlaylist] - no more pages. User's full playlist retrieved\n")
			break
		}
		if paginationErr != nil {
			log.Printf("\n[services][spotify][base][FetchUserPlaylist] error - could not fetch playlist: %v\n", err)
			return nil, paginationErr
		}
		playlists.Playlists = append(playlists.Playlists, out.Playlists...)
	}

	var userPlaylists []blueprint.UserPlaylist
	for _, playlist := range playlists.Playlists {
		pix := playlist.Images
		var cover string
		if len(pix) > 0 {
			cover = pix[0].URL
		}
		userPlaylists = append(userPlaylists, blueprint.UserPlaylist{
			ID:            string(playlist.ID),
			Title:         playlist.Name,
			Public:        playlist.IsPublic,
			Collaborative: playlist.Collaborative,
			NbTracks:      int(playlist.Tracks.Total),
			URL:           playlist.ExternalURLs["spotify"],
			Cover:         cover,
			Checksum:      playlist.SnapshotID,
			Owner:         playlist.Owner.DisplayName,
		})
	}

	return userPlaylists, nil
}
