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
	var conversion blueprint.PlaylistConversion
	switch info.Platform {
	case deezer.IDENTIFIER:
		var deezerPlaylist, tracklistErr = deezer.FetchPlaylistTracklist(info.EntityID, red)
		if tracklistErr != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][error] - Could not fetch tracklist from deezer %v\n", tracklistErr)
			return nil, tracklistErr
		}

		// then for each of these playlists, search for the tracks on spotify
		var omittedTracks []blueprint.OmittedTracks
		spotifyTracks, omittedSpotifyTracks := spotify.FetchPlaylistSearchResult(deezerPlaylist, red)
		tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(deezerPlaylist, red)

		omittedTracks = append(omittedTracks, *omittedSpotifyTracks...)
		omittedTracks = append(omittedTracks, *omittedTidalTracks...)

		conversion.URL = deezerPlaylist.URL
		conversion.Title = deezerPlaylist.Title
		conversion.Length = deezerPlaylist.Length
		conversion.Owner = deezerPlaylist.Owner
		conversion.OmittedTracks = omittedTracks
		conversion.Cover = deezerPlaylist.Cover

		conversion.Tracks.Deezer = &deezerPlaylist.Tracks
		conversion.Tracks.Spotify = spotifyTracks
		conversion.Tracks.Tidal = tidalTracks
		/**
		what the structure looks like
			{
			  "spotify": [{ Title: '', URL: ''}, { Title: '', URL: ''}],
			  "deezer": [{ Title: '', URL: ''}, { Title: '', URL: ''}]
			}
		*/

		err := CachePlaylistTracksWithID(&deezerPlaylist.Tracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][warning] - Could not cache tracks for playlist %s: %v\n", deezerPlaylist.Title, err)
		} else {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][success] - Cached tracks for playlist %s\n", deezerPlaylist.Title)
		}

		return &conversion, nil
	case spotify.IDENTIFIER:
		entityID := info.EntityID

		spotifyPlaylist, _, err := spotify.FetchPlaylistTracksAndInfo(entityID, red)

		// for whatever reason, the spotify API does not return the playlist. Probably because it is private
		if err != nil && err != spotify2.ErrNoMorePages {
			log.Printf("\n[controllers][platforms][base] Error fetching playlist tracks and info from spotify: %v\n", err)
			return nil, err
		}

		var omittedTracks []blueprint.OmittedTracks
		deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(spotifyPlaylist, red)
		tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(spotifyPlaylist, red)

		omittedTracks = append(omittedTracks, *omittedDeezerTracks...)
		omittedTracks = append(omittedTracks, *omittedTidalTracks...)

		conversion.URL = spotifyPlaylist.URL
		conversion.Title = spotifyPlaylist.Title
		conversion.Length = spotifyPlaylist.Length
		conversion.Owner = spotifyPlaylist.Owner
		conversion.OmittedTracks = omittedTracks
		conversion.Cover = spotifyPlaylist.Cover

		err = CachePlaylistTracksWithID(deezerTracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] warning - could not cache tracks: %v %v\n\n", err, deezerTracks)
		}
		conversion.Tracks.Spotify = &spotifyPlaylist.Tracks
		conversion.Tracks.Deezer = deezerTracks
		conversion.Tracks.Tidal = tidalTracks

		err = CachePlaylistTracksWithID(&spotifyPlaylist.Tracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] warning - could not cache tracks: %v %v\n\n", err, spotifyPlaylist.Tracks)
		}
		return &conversion, nil

	case tidal.IDENTIFIER:
		tidalPlaylist, err := tidal.FetchPlaylist(info.EntityID, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] error - could not fetch playlist with ID from tidal: %v\n", err)
			return nil, err
		}
		var omittedTracks []blueprint.OmittedTracks
		deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(tidalPlaylist, red)
		spotifyTracks, omittedSpotifyTracks := spotify.FetchPlaylistSearchResult(tidalPlaylist, red)
		omittedTracks = append(*omittedDeezerTracks, *omittedSpotifyTracks...)
		conversion.URL = tidalPlaylist.URL
		conversion.Title = tidalPlaylist.Title
		conversion.Length = tidalPlaylist.Length
		conversion.Owner = tidalPlaylist.Owner
		// hated doing this but lol tsk tsk
		conversion.OmittedTracks = *&omittedTracks
		conversion.Cover = tidalPlaylist.Cover

		conversion.Tracks.Deezer = deezerTracks
		conversion.Tracks.Spotify = spotifyTracks
		conversion.Tracks.Tidal = &tidalPlaylist.Tracks
		err = CachePlaylistTracksWithID(deezerTracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] warning - could not cache tracks: %v %v\n\n", err, deezerTracks)
		} else {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] success - cached tracks: %v\n\n", tidalPlaylist.Title)
		}

		return &conversion, nil
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
