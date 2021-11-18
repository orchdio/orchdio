package universal

import (
	"github.com/go-redis/redis/v8"
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

		spSingleTrack, err := spotify.SearchTrackWithTitle(trackTitle)

		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Spotify = nil
			}
		}
		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Deezer = deezerTrack
		return &conversion, nil

	case spotify.IDENTIFIER:
		spSingleTrack, err := spotify.SearchTrackWithID(info.EntityID)
		if err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not search track with ID from spotify: %v\n", err)
			return nil, err
		}

		dzSingleTrack, err := deezer.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artistes[0], spSingleTrack.Album)
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

func ConvertPlaylist(info *blueprint.LinkInfo) (*blueprint.PlaylistConversion, error) {
	var conversion blueprint.PlaylistConversion
	switch info.Platform {
	case deezer.IDENTIFIER:
		var deezerPlaylist, dzPagination, tracklistErr = deezer.FetchPlaylistTracklist(info.TargetLink)
		if tracklistErr != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][error] - Could not fetch tracklist from deezer %v\n", tracklistErr)
			return nil, tracklistErr
		}

		// then for each of these playlists, search for the tracks on spotify
		var trackTitles []string
		for _, tracks := range deezerPlaylist.Tracks {
			trackTitles = append(trackTitles, tracks.Title)
		}

		spotifyTracks := spotify.FetchTracks(trackTitles)
		convertedPlaylist := blueprint.PlaylistConversion{
			URL:        deezerPlaylist.URL,
			Length:     deezerPlaylist.Length,
			Title:      deezerPlaylist.Title,
			Preview:    "",
			Pagination: *dzPagination,
		}

		convertedPlaylist.Tracks.Deezer = &deezerPlaylist.Tracks
		convertedPlaylist.Tracks.Spotify = spotifyTracks

		return &conversion, nil
	case spotify.IDENTIFIER:
		entityID := info.EntityID
		spotifyPlaylist := blueprint.PlaylistSearchResult{}
		spPagination := blueprint.Pagination{}

		// THIS FEELS FUCKING HACKY. IT'S ALL SHADES OF WRONG
		// but, I am not sure how else to proceed for now
		// What this part does is that it checks if there's pagination and then fetch
		// the paginated playlist tracks. for example if its the third page, it fetches this
		// and converts the result into the standard *[]blueprint.TrackSearchResult
		// However, the request might be just for fetching the first page (normal playlist conversion)
		// So because of this, we're creating the empty variables, making the respective
		// requests and assigning the variables to the response of the result.
		//if pagination != "" {
		//	log.Printf("\n[debug] - Its a pagination.. Extracted spotify ID")
		//	s1, s2, err := spotify.FetchNextPage(info.TargetLink)
		//	spotifyPlaylist = *s1
		//	spPagination = *s2
		//	if err != nil {
		//		log.Printf("\n[controllers][platforms][base] Error fetching next page from spotify")
		//	}
		//} else {
		//	s1, s2, err := spotify.FetchPlaylistTracksAndInfo(entityID)
		//	if err != nil {
		//		log.Printf("\n[controllers][platforms][base] Error fetching next page from spotify")
		//	}
		//	spotifyPlaylist = *s1
		//	spPagination = *s2
		//}

		spotifyTracks, _, err := spotify.FetchPlaylistTracksAndInfo(entityID)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] Error fetching playlist tracks and info from spotify: %v\n", err)
		}

		deezerTracks := deezer.FetchPlaylistSearchResult(&spotifyPlaylist)
		convertedPlaylist := blueprint.PlaylistConversion{
			URL:        spotifyPlaylist.URL,
			Title:      spotifyPlaylist.Title,
			Preview:    "",
			Pagination: spPagination,
		}

		convertedPlaylist.Tracks.Deezer = deezerTracks
		convertedPlaylist.Tracks.Spotify = &spotifyTracks.Tracks
	default:
		return nil, blueprint.ENOTIMPLEMENTED
	}
	return nil, nil
}
