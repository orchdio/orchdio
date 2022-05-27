package universal

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	spotify2 "github.com/zmb3/spotify/v2"
	"log"
	"zoove/blueprint"
	"zoove/services/deezer"
	"zoove/services/spotify"
	"zoove/services/tidal"
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

		tidalTrack, err := tidal.SearchTrackWithTitle(trackTitle, deezerTrack.Artistes[0], red)

		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Tidal = nil
			}
		}

		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Deezer = deezerTrack
		conversion.Platforms.Tidal = tidalTrack

		spotifyCacheKey := "spotify:" + spSingleTrack.ID
		deezerCacheKey := "deezer:" + deezerTrack.ID
		tidalCacheKey := "tidal:" + tidalTrack.ID
		// create a map from spotifyCacheKey and deezerCacheKey
		cacheMap := map[string]*blueprint.TrackSearchResult{
			spotifyCacheKey: spSingleTrack,
			deezerCacheKey:  deezerTrack,
			tidalCacheKey:   tidalTrack,
		}

		err = CacheTracksWithID(cacheMap, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] warning - could not cache tracks: %v\n", err)
		}

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

		tidalTrack, err := tidal.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artistes[0], red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Tidal = nil
			}
		}
		conversion.Platforms.Tidal = tidalTrack
		return &conversion, nil

	case tidal.IDENTIFIER:
		tidalTrack, err := tidal.SearchWithID(info.EntityID, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertTrack] error - could not fetch track with ID from tidal: %v\n", err)
			return nil, err
		}
		// then search on spotify
		tidalArtist := tidalTrack.Artistes[0]
		tidalAlbum := tidalTrack.Album
		spotifyTrack, err := spotify.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertTrack] error - could not search track with ID from spotify: %v\n", err)
			return nil, err
		}
		deezerSingleTrack, err := deezer.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, tidalAlbum, red)
		conversion.Platforms.Spotify = spotifyTrack
		conversion.Platforms.Deezer = deezerSingleTrack
		conversion.Platforms.Tidal = tidalTrack
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
		spotifyTracks, omittedTracks := spotify.FetchPlaylistSearchResult(deezerPlaylist, red)
		convertedPlaylist := blueprint.PlaylistConversion{
			URL:           deezerPlaylist.URL,
			Length:        deezerPlaylist.Length,
			Title:         deezerPlaylist.Title,
			Preview:       "",
			Owner:         deezerPlaylist.Owner,
			Cover:         deezerPlaylist.Cover,
			OmittedTracks: *omittedTracks,
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

		return &convertedPlaylist, nil
	case spotify.IDENTIFIER:
		entityID := info.EntityID

		spotifyPlaylist, _, err := spotify.FetchPlaylistTracksAndInfo(entityID, red)

		// for whatever reason, the spotify API does not return the playlist. Probably because it is private
		if err != nil && err != spotify2.ErrNoMorePages {
			log.Printf("\n[controllers][platforms][base] Error fetching playlist tracks and info from spotify: %v\n", err)
			return nil, err
		}

		deezerTracks, omittedTracks := deezer.FetchPlaylistSearchResult(spotifyPlaylist, red)
		convertedPlaylist := blueprint.PlaylistConversion{
			URL:           spotifyPlaylist.URL,
			Title:         spotifyPlaylist.Title,
			Preview:       "",
			Length:        spotifyPlaylist.Length,
			Owner:         spotifyPlaylist.Owner,
			OmittedTracks: *omittedTracks,
			Cover:         spotifyPlaylist.Cover,
		}

		convertedPlaylist.Tracks.Deezer = deezerTracks
		err = CachePlaylistTracksWithID(deezerTracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] warning - could not cache tracks: %v %v\n\n", err, deezerTracks)
		}
		convertedPlaylist.Tracks.Spotify = &spotifyPlaylist.Tracks
		err = CachePlaylistTracksWithID(&spotifyPlaylist.Tracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] warning - could not cache tracks: %v %v\n\n", err, spotifyPlaylist.Tracks)
		}
		return &convertedPlaylist, nil
	default:
		return nil, blueprint.ENOTIMPLEMENTED
	}
}

// CacheTracksWithID caches the results of a track conversion, under a key with a scheme of "platform:trackID"
func CacheTracksWithID(records map[string]*blueprint.TrackSearchResult, red *redis.Client) error {
	for cacheKey, data := range records {
		// stringify data
		dataJSON, err := json.Marshal(data)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] Error marshalling track result data to JSON: %v\n", err)
			return err
		}
		if err := red.Set(context.Background(), cacheKey, string(dataJSON), 0).Err(); err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not cache track on %s: %v\n", cacheKey, err)
			return err
		}
		log.Printf("\n[controllers][platforms][spotify][ConvertTrack] cache - track %s cached on %s\n", data.Title, cacheKey)
	}
	return nil
}

// CachePlaylistTracksWithID caches the results of each of the tracks from a playlist conversion, under the same key scheme as CacheTracksWithID
func CachePlaylistTracksWithID(tracks *[]blueprint.TrackSearchResult, red *redis.Client) error {
	for _, data := range *tracks {
		// stringify data
		dataJSON, err := json.Marshal(data)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] Error marshalling track result data to JSON: %v\n", err)
			return err
		}
		if err := red.Set(context.Background(), "spotify:"+data.ID, string(dataJSON), 0).Err(); err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not cache track on %s: %v\n", "spotify:"+data.ID, err)
			return err
		}
		log.Printf("\n[controllers][platforms][spotify][ConvertTrack] cache - track %s cached on %s\n", data.Title, "spotify:"+data.ID)
	}
	return nil
}
