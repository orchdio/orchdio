package universal

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	spotify2 "github.com/zmb3/spotify/v2"
	"log"
	"orchdio/blueprint"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
)

// ConvertTrack fetches all the tracks converted from all the supported platforms
func ConvertTrack(info *blueprint.LinkInfo, red *redis.Client) (*blueprint.Conversion, error) {
	var conversion blueprint.Conversion
	conversion.Entity = "track"
	spotifyCacheKey := ""
	deezerCacheKey := ""
	tidalCacheKey := ""
	ytmusicCacheKey := ""
	appleCacheKey := ""

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

		ytmusicTrack, err := ytmusic.SearchTrackWithTitle(trackTitle, deezerTrack.Artistes[0], red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.YTMusic = nil
			}
		}

		apple, err := applemusic.SearchTrackWithTitle(trackTitle, deezerTrack.Artistes[0], red)
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - could not get apple music track")
			if err == blueprint.ENORESULT {
				conversion.Platforms.AppleMusic = nil
			}
		}

		conversion.Platforms.Deezer = deezerTrack
		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Tidal = tidalTrack
		conversion.Platforms.YTMusic = ytmusicTrack
		conversion.Platforms.AppleMusic = apple

		if deezerTrack != nil {
			deezerCacheKey = "deezer:" + deezerTrack.ID
		}

		if tidalTrack != nil {
			tidalCacheKey = "tidal:" + tidalTrack.ID
		}

		if ytmusicTrack != nil {
			ytmusicCacheKey = "ytmusic:" + ytmusicTrack.ID
		}

		if spSingleTrack != nil {
			spotifyCacheKey = "spotify:" + spSingleTrack.ID
		}

		if apple != nil {
			appleCacheKey = "applemusic:" + apple.ID
		}

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

		tidalTrack, err := tidal.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artistes[0], red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Tidal = nil
			}
		}

		ytmusicTrack, err := ytmusic.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artistes[0], red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.YTMusic = nil
			}
		}

		apple, err := applemusic.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artistes[0], red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.AppleMusic = nil
			}
		}

		conversion.Platforms.Tidal = tidalTrack
		conversion.Platforms.YTMusic = ytmusicTrack
		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Deezer = dzSingleTrack
		conversion.Platforms.AppleMusic = apple

		if dzSingleTrack != nil {
			deezerCacheKey = "deezer:" + dzSingleTrack.ID
		}

		if tidalTrack != nil {
			tidalCacheKey = "tidal:" + tidalTrack.ID
		}

		if ytmusicTrack != nil {
			ytmusicCacheKey = "ytmusic:" + ytmusicTrack.ID
		}

		if spSingleTrack != nil {
			spotifyCacheKey = "spotify:" + spSingleTrack.ID
		}

		if apple != nil {
			appleCacheKey = "applemusic:" + apple.ID
		}

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
			conversion.Platforms.Spotify = nil
		}
		deezerSingleTrack, err := deezer.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, tidalAlbum, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertTrack] error - could not search track with ID from deezer: %v\n", err)
			conversion.Platforms.Deezer = nil
		}

		ytmusicTrack, err := ytmusic.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.YTMusic = nil
			}
		}

		apple, err := applemusic.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.AppleMusic = nil
			}
		}

		conversion.Platforms.Spotify = spotifyTrack
		conversion.Platforms.Deezer = deezerSingleTrack
		conversion.Platforms.Tidal = tidalTrack
		conversion.Platforms.YTMusic = ytmusicTrack
		conversion.Platforms.AppleMusic = apple

		if spotifyTrack != nil {
			//cacheKeys = append(cacheKeys, "spotify:"+spotifyTrack.ID)
			spotifyCacheKey = "spotify:" + spotifyTrack.ID
		}

		if deezerSingleTrack != nil {
			//cacheKeys = append(cacheKeys, "deezer:"+deezerSingleTrack.ID)
			deezerCacheKey = "deezer:" + deezerSingleTrack.ID
		}

		if tidalTrack != nil {
			//cacheKeys = append(cacheKeys, "tidal:"+tidalTrack.ID)
			tidalCacheKey = "tidal:" + tidalTrack.ID
		}

		if ytmusicTrack != nil {
			//cacheKeys = append(cacheKeys, "ytmusic:"+ytmusicTrack.ID)
			ytmusicCacheKey = "ytmusic:" + ytmusicTrack.ID
		}

		if apple != nil {
			//cacheKeys = append(cacheKeys, "applemusic:"+apple.ID)
			appleCacheKey = "applemusic:" + apple.ID
		}

	case ytmusic.IDENTIFIER:
		ytmusicTrack, err := ytmusic.SearchTrackWithLink(info, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][ytmusic][ConvertTrack] error - could not fetch track with ID from ytmusic: %v\n", err)
			return nil, err
		}
		ytmusicArtiste := ytmusicTrack.Artistes[0]
		ytmusicAlbum := ytmusicTrack.Album
		spotifyTrack, err := spotify.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicArtiste, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][ytmusic][ConvertTrack] error - could not search track with ID from spotify: %v\n", err)
			conversion.Platforms.Spotify = nil
		}
		deezerSingleTrack, err := deezer.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicArtiste, ytmusicAlbum, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][ytmusic][ConvertTrack] error - could not search track with ID from deezer: %v\n", err)
			conversion.Platforms.Deezer = nil
		}
		tidalTrack, err := tidal.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicArtiste, red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Tidal = nil
			}
		}

		apple, err := applemusic.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicTrack.Artistes[0], red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.AppleMusic = nil
			}
		}

		conversion.Platforms.Spotify = spotifyTrack
		conversion.Platforms.Deezer = deezerSingleTrack
		conversion.Platforms.Tidal = tidalTrack
		conversion.Platforms.YTMusic = ytmusicTrack
		conversion.Platforms.AppleMusic = apple

		if spotifyTrack != nil {
			//cacheKeys = append(cacheKeys, "spotify:"+spotifyTrack.ID)
			spotifyCacheKey = "spotify:" + spotifyTrack.ID
		}

		if deezerSingleTrack != nil {
			//cacheKeys = append(cacheKeys, "deezer:"+deezerSingleTrack.ID)
			deezerCacheKey = "deezer:" + deezerSingleTrack.ID
		}

		if tidalTrack != nil {
			//cacheKeys = append(cacheKeys, "tidal:"+tidalTrack.ID)
			tidalCacheKey = "tidal:" + tidalTrack.ID
		}

		if ytmusicTrack != nil {
			//cacheKeys = append(cacheKeys, "ytmusic:"+ytmusicTrack.ID)
			ytmusicCacheKey = "ytmusic:" + ytmusicTrack.ID
		}

	case applemusic.IDENTIFIER:
		apple, err := applemusic.SearchTrackWithLink(info, red)
		if err != nil {
			log.Printf("\n[controller][platforms][applemusic][ConvertTrack] error - could not get apple music track")
			return nil, err
		}

		artiste := apple.Artistes[0]
		title := apple.Title
		album := apple.Album
		spotifyTrack, err := spotify.SearchTrackWithTitle(title, artiste, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][applemusic][ConvertTrack] error - could not search track with ID from spotify: %v\n", err)
			conversion.Platforms.Spotify = nil
		}
		deezerSingleTrack, err := deezer.SearchTrackWithTitle(title, artiste, album, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][applemusic][ConvertTrack] error - could not search track with ID from deezer: %v\n", err)
			conversion.Platforms.Deezer = nil
		}
		tidalTrack, err := tidal.SearchTrackWithTitle(title, artiste, red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Tidal = nil
			}
		}

		ytmusicTrack, err := ytmusic.SearchTrackWithTitle(artiste, artiste, red)
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.YTMusic = nil
			}
		}

		conversion.Platforms.Spotify = spotifyTrack
		conversion.Platforms.Deezer = deezerSingleTrack
		conversion.Platforms.Tidal = tidalTrack
		conversion.Platforms.YTMusic = ytmusicTrack
		conversion.Platforms.AppleMusic = apple

		if spotifyTrack != nil {
			//cacheKeys = append(cacheKeys, "spotify:"+spotifyTrack.ID)
			spotifyCacheKey = "spotify:" + spotifyTrack.ID
		}

		if deezerSingleTrack != nil {
			//cacheKeys = append(cacheKeys, "deezer:"+deezerSingleTrack.ID)
			deezerCacheKey = "deezer:" + deezerSingleTrack.ID
		}

		if tidalTrack != nil {
			//cacheKeys = append(cacheKeys, "tidal:"+tidalTrack.ID)
			tidalCacheKey = "tidal:" + tidalTrack.ID
		}

		if ytmusicTrack != nil {
			//cacheKeys = append(cacheKeys, "ytmusic:"+ytmusicTrack.ID)
			ytmusicCacheKey = "ytmusic:" + ytmusicTrack.ID
		}

	default:
		return nil, blueprint.ENOTIMPLEMENTED
	}

	// create a map from spotifyCacheKey and deezerCacheKey
	cacheMap := map[string]*blueprint.TrackSearchResult{
		spotifyCacheKey: conversion.Platforms.Spotify,
		deezerCacheKey:  conversion.Platforms.Deezer,
		tidalCacheKey:   conversion.Platforms.Tidal,
		ytmusicCacheKey: conversion.Platforms.YTMusic,
		appleCacheKey:   conversion.Platforms.AppleMusic,
	}

	err := CacheTracksWithID(cacheMap, red)
	if err != nil {
		log.Printf("\n[controllers][platforms][spotify][ConvertTrack] warning - could not cache tracks: %v\n", err)
	}

	return &conversion, nil
}

// ConvertPlaylist converts a playlist from one platform to another
func ConvertPlaylist(info *blueprint.LinkInfo, red *redis.Client) (*blueprint.PlaylistConversion, error) {
	var conversion blueprint.PlaylistConversion
	omittedTracks := map[string][]blueprint.OmittedTracks{}
	switch info.Platform {
	case deezer.IDENTIFIER:
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] converting playlist %s\n", info.EntityID)
		var deezerPlaylist, tracklistErr = deezer.FetchPlaylistTracklist(info.EntityID, red)
		if tracklistErr != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][error] - Could not fetch tracklist from deezer %v\n", tracklistErr)
			return nil, tracklistErr
		}

		// then for each of these playlists, search for the tracks on spotify

		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetching tracks for playlist from spotify %s\n", info.EntityID)
		spotifyTracks, omittedSpotifyTracks := spotify.FetchPlaylistSearchResult(deezerPlaylist, red)
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetchde playlist tracks from spotify %s\n", info.EntityID)

		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetching tracks for playlist from tidal %s\n", info.EntityID)
		tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(deezerPlaylist, red)
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetchde playlist tracks from tidal %s\n", info.EntityID)

		appleTracks, omittedAppleTracks := applemusic.FetchPlaylistSearchResult(deezerPlaylist, red)

		omittedTracks["spotify"] = *omittedSpotifyTracks
		omittedTracks["tidal"] = *omittedTidalTracks
		omittedTracks["apple"] = *omittedAppleTracks

		conversion.URL = deezerPlaylist.URL
		conversion.Title = deezerPlaylist.Title
		conversion.Length = deezerPlaylist.Length
		conversion.Owner = deezerPlaylist.Owner
		conversion.OmittedTracks = omittedTracks
		conversion.Cover = deezerPlaylist.Cover

		conversion.Tracks.Deezer = &deezerPlaylist.Tracks
		conversion.Tracks.Spotify = spotifyTracks
		conversion.Tracks.Tidal = tidalTracks
		conversion.Tracks.AppleMusic = appleTracks
		/**
		what the structure looks like
			{
			  "spotify": [{ Title: '', URL: ''}, { Title: '', URL: ''}],
			  "deezer": [{ Title: '', URL: ''}, { Title: '', URL: ''}]
			}
		*/
		log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer] - fetching track in the playlist with url: %v\n", deezerPlaylist.URL)
		err := CachePlaylistTracksWithID(&deezerPlaylist.Tracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer][warning] - Could not cache tracks for playlist %s: %v\n", deezerPlaylist.Title, err)
		} else {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][success] - Cached tracks for playlist %s\n", deezerPlaylist.Title)
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer] - converted playlist %v\n", deezerPlaylist.URL)
		return &conversion, nil
	case spotify.IDENTIFIER:
		log.Printf("\n[controllers][platforms][ConvertPlaylist][spotify] - converting playlist with id: %v\n", info.EntityID)
		entityID := info.EntityID

		spotifyPlaylist, _, err := spotify.FetchPlaylistTracksAndInfo(entityID, red)

		// for whatever reason, the spotify API does not return the playlist. Probably because it is private
		if err != nil && err != spotify2.ErrNoMorePages {
			log.Printf("\n[controllers][platforms][base] Error fetching playlist tracks and info from spotify: %v\n", err)
			return nil, err
		}
		log.Printf("\n[controllers][platforms][base][spotify] - fetching playlist tracks and info from deezer: %v\n", spotifyPlaylist)
		deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(spotifyPlaylist, red)
		log.Printf("\n[controllers][platforms][base][spotify] - fetched playlist tracks and info from deezer: %v\n", deezerTracks)

		tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(spotifyPlaylist, red)
		log.Printf("\n[controllers][platforms][base][spotify] - fetched playlist tracks and info from tidal: %v\n", tidalTracks)

		appleTracks, omittedAppleTracks := applemusic.FetchPlaylistSearchResult(spotifyPlaylist, red)

		omittedTracks["deezer"] = *omittedDeezerTracks
		omittedTracks["tidal"] = *omittedTidalTracks
		omittedTracks["apple"] = *omittedAppleTracks

		conversion.URL = spotifyPlaylist.URL
		conversion.Title = spotifyPlaylist.Title
		conversion.Length = spotifyPlaylist.Length
		conversion.Owner = spotifyPlaylist.Owner
		conversion.OmittedTracks = omittedTracks
		conversion.Cover = spotifyPlaylist.Cover

		conversion.Tracks.Spotify = &spotifyPlaylist.Tracks
		conversion.Tracks.Deezer = deezerTracks
		conversion.Tracks.Tidal = tidalTracks
		conversion.Tracks.AppleMusic = appleTracks

		log.Printf("\n[controllers][platforms][ConvertPlaylist][spotify] - caching tracks in playlist %v\n", spotifyPlaylist.URL)
		err = CachePlaylistTracksWithID(deezerTracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] warning - could not cache tracks: %v %v\n\n", err, deezerTracks)
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][spotify] - fetching tracks in playlist %v\n", spotifyPlaylist.URL)
		err = CachePlaylistTracksWithID(&spotifyPlaylist.Tracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] warning - could not cache tracks: %v %v\n\n", err, spotifyPlaylist.Tracks)
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][spotify] - converted playlist %v\n", spotifyPlaylist.URL)
		return &conversion, nil

	case tidal.IDENTIFIER:
		log.Printf("\n[controllers][platforms][ConvertPlaylist][tidal] - converting playlist %v\n", info.EntityID)
		_, tidalPlaylist, _, err := tidal.FetchPlaylist(info.EntityID, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] error - could not fetch playlist with ID from tidal: %v\n", err)
			return nil, err
		}
		deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(tidalPlaylist, red)
		log.Printf("\n[controllers][platforms][base][tidal] - fetched playlist tracks and info from deezer: %v\n", deezerTracks)

		spotifyTracks, omittedSpotifyTracks := spotify.FetchPlaylistSearchResult(tidalPlaylist, red)
		log.Printf("\n[controllers][platforms][base][tidal] - fetched playlist tracks and info from spotify: %v\n", spotifyTracks)

		appleTracks, omittedAppleTracks := applemusic.FetchPlaylistSearchResult(tidalPlaylist, red)

		omittedTracks["deezer"] = *omittedDeezerTracks
		omittedTracks["spotify"] = *omittedSpotifyTracks
		omittedTracks["apple"] = *omittedAppleTracks

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
		conversion.Tracks.AppleMusic = appleTracks

		log.Printf("\n[controllers][platforms][ConvertPlaylist][tidal] - caching tracks in playlist %v\n", tidalPlaylist.URL)
		err = CachePlaylistTracksWithID(conversion.Tracks.Tidal, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] warning - could not cache tracks: %v %v\n\n", err, deezerTracks)
		} else {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] success - cached tracks: %v\n\n", tidalPlaylist.Title)
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][tidal] - converted playlist %v\n", tidalPlaylist.URL)
		return &conversion, nil
	case applemusic.IDENTIFIER:
		log.Printf("\n[controllers][platforms][ConvertPlaylist][applemusic] - converting playlist %v\n", info.EntityID)
		applePlaylist, err := applemusic.FetchPlaylistTrackList(info.EntityID, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][applemusic][ConvertPlaylist] error - could not fetch playlist with ID from applemusic: %v\n", err)
			return nil, err
		}
		deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(applePlaylist, red)
		log.Printf("\n[controllers][platforms][base][applemusic] - fetched playlist tracks and info from deezer: %v\n", deezerTracks)

		spotifyTracks, omittedSpotifyTracks := spotify.FetchPlaylistSearchResult(applePlaylist, red)
		log.Printf("\n[controllers][platforms][base][applemusic] - fetched playlist tracks and info from spotify: %v\n", spotifyTracks)
		tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(applePlaylist, red)
		log.Printf("\n[controllers][platforms][base][applemusic] - fetched playlist tracks and info from tidal: %v\n", tidalTracks)

		omittedTracks["deezer"] = *omittedDeezerTracks
		omittedTracks["spotify"] = *omittedSpotifyTracks
		omittedTracks["tidal"] = *omittedTidalTracks

		conversion.URL = applePlaylist.URL
		conversion.Title = applePlaylist.Title
		conversion.Length = applePlaylist.Length
		conversion.Owner = applePlaylist.Owner
		// hated doing this but lol tsk tsk
		conversion.OmittedTracks = *&omittedTracks
		conversion.Cover = applePlaylist.Cover
		conversion.Tracks.Deezer = deezerTracks
		conversion.Tracks.Spotify = spotifyTracks
		conversion.Tracks.Tidal = tidalTracks
		conversion.Tracks.AppleMusic = &applePlaylist.Tracks
		err = CachePlaylistTracksWithID(&applePlaylist.Tracks, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][applemusic][ConvertPlaylist] warning - could not cache tracks: %v %v\n\n", err, applePlaylist.Tracks)
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][applemusic] - caching tracks in playlist %v\n", applePlaylist.URL)
		return &conversion, nil
	default:
		return nil, blueprint.ENOTIMPLEMENTED
	}
}

// CacheTracksWithID caches the results of a track conversion, under a key with a scheme of "platform:trackID"
func CacheTracksWithID(records map[string]*blueprint.TrackSearchResult, red *redis.Client) error {
	for cacheKey, data := range records {
		if data == nil {
			log.Printf("\n[controllers][platforms][base] warning - no result to cache for this platform: %v\n\n", cacheKey)
			continue
		}
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
