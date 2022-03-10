package universal

import (
	"github.com/go-redis/redis/v8"
	spotify2 "github.com/zmb3/spotify/v2"
	"log"
	"zoove/blueprint"
	"zoove/services/deezer"
	"zoove/services/spotify"
)

// ConvertTrack fetches all the tracks converted from all the supported platforms
func ConvertTrack(info *blueprint.LinkInfo, red *redis.Client) (*blueprint.Conversion, error) {
	var conversion blueprint.Conversion
	conversion.Entity = "track"
	switch info.Platform {
	case deezer.IDENTIFIER:
		deezerTrack := deezer.SearchTrackWithLink(info, red)
		trackTitle := deezer.ExtractTitle(deezerTrack.Title)

		spSingleTrack, err := spotify.SearchTrackWithTitle(trackTitle, deezerTrack.Artistes[0], red)

		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Spotify = nil
			}
		}
		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Deezer = deezerTrack
		return &conversion, nil

	case spotify.IDENTIFIER:
		spSingleTrack, err := spotify.SearchTrackWithID(info.EntityID, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not search track with ID from spotify: %v\n", err)
			return nil, err
		}

		dzSingleTrack, err := deezer.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artistes[0], spSingleTrack.Album, red)
		if err != nil && err != blueprint.ENORESULT {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not search track with title '%s' on deezer. err %v\n", spSingleTrack.Title, err)
			return nil, err
		}

		if err != nil && err == blueprint.ENORESULT {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not search track with title %s on deezer. No result found\n", spSingleTrack.Title)
		}

		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Deezer = dzSingleTrack
		return &conversion, nil
	default:
		return nil, blueprint.ENOTIMPLEMENTED
	}
}

func ConvertPlaylist(info *blueprint.LinkInfo, red *redis.Client) (*blueprint.PlaylistConversion, error) {
	switch info.Platform {
	case deezer.IDENTIFIER:
		var deezerPlaylist, tracklistErr = deezer.FetchPlaylistTracklist(info.EntityID, red)
		if tracklistErr != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][error] - Could not fetch tracklist from deezer %v\n", tracklistErr)
			return nil, tracklistErr
		}

		// then for each of these playlists, search for the tracks on spotify
		//var trackTitles blueprint.PlaylistSearchResult
		//for _, tracks := range deezerPlaylist.Tracks {
		//	trackTitles = append(trackTitles, tracks.Title)
		//}

		spotifyTracks, omittedTracks := spotify.FetchPlaylistSearchResult(deezerPlaylist, red)
		omitted := make([]map[string][]blueprint.OmittedTracks, 0)
		convertedPlaylist := blueprint.PlaylistConversion{
			URL:     deezerPlaylist.URL,
			Length:  deezerPlaylist.Length,
			Title:   deezerPlaylist.Title,
			Preview: "",
			Owner:   deezerPlaylist.Owner,
		}

		convertedPlaylist.Tracks.Deezer = &deezerPlaylist.Tracks
		convertedPlaylist.Tracks.Spotify = spotifyTracks
		/**
		what the structure looks like
			{
			  "spotify": [{ Title: '', URL: ''}, { Title: '', URL: ''}],
			  "deezer": [{ Title: '', URL: ''}, { Title: '', URL: ''}]
			}
		*/
		omitted = append(omitted, map[string][]blueprint.OmittedTracks{"spotify": *omittedTracks})
		convertedPlaylist.OmittedTracks = omitted

		return &convertedPlaylist, nil
	case spotify.IDENTIFIER:
		entityID := info.EntityID

		spotifyPlaylist, _, err := spotify.FetchPlaylistTracksAndInfo(entityID, red)
		if err != nil && err != spotify2.ErrNoMorePages {
			log.Printf("\n[controllers][platforms][base] Error fetching playlist tracks and info from spotify: %v\n", err)
		}

		deezerTracks := deezer.FetchPlaylistSearchResult(spotifyPlaylist, red)
		convertedPlaylist := blueprint.PlaylistConversion{
			URL:     spotifyPlaylist.URL,
			Title:   spotifyPlaylist.Title,
			Preview: "",
			Length:  spotifyPlaylist.Length,
			Owner:   spotifyPlaylist.Owner,
		}

		convertedPlaylist.Tracks.Deezer = deezerTracks
		convertedPlaylist.Tracks.Spotify = &spotifyPlaylist.Tracks
		return &convertedPlaylist, nil
	default:
		return nil, blueprint.ENOTIMPLEMENTED
	}
}
