package universal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jmoiron/sqlx"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	"time"

	"github.com/go-redis/redis/v8"
	spotify2 "github.com/zmb3/spotify/v2"
)

// sumUpResultLength sums up the length of all the tracks in a slice of TrackSearchResult
func sumUpResultLength(tracks *[]blueprint.TrackSearchResult) int {
	var length int
	for _, track := range *tracks {
		length += track.DurationMilli
	}
	return length
}

// ConvertTrack fetches all the tracks converted from all the supported platforms
func ConvertTrack(info *blueprint.LinkInfo, red *redis.Client,
	pg *sqlx.DB) (*blueprint.Conversion, error) {
	var conversion blueprint.Conversion
	conversion.Entity = "track"

	// fetch the app making the request
	database := db.NewDB{DB: pg}
	app, err := database.FetchAppByAppId(info.App)
	if err != nil {
		log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not fetch app: %v\n", err)
		return nil, err
	}

	targetPlatform := info.TargetPlatform
	if targetPlatform == "" {
		log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no target platform provided\n")
		targetPlatform = "all"
	}

	if string(app.SpotifyCredentials) == "" {
		log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no spotify credentials provided\n")
		return nil, blueprint.ECREDENTIALSMISSING
	}

	spotifyIntegrationCreds, err := util.DeserializeAppCredentials(app.SpotifyCredentials)
	if err != nil {
		log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not deserialize spotify spotifyIntegrationCreds: %v", err)
		return nil, err
	}

	switch info.Platform {
	case deezer.IDENTIFIER:
		deezerTrack := deezer.SearchTrackWithLink(info, red)
		if deezerTrack == nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get deezer track")
			return nil, blueprint.ENORESULT
		}
		conversion.Platforms.Deezer = deezerTrack
		strippedTextInfo := util.ExtractTitle(deezerTrack.Title)
		tt := strippedTextInfo.Title

		if targetPlatform == "all" {
			var emptyTrack *blueprint.TrackSearchResult
			var searchMethods = map[string]func(title, artist string, red *redis.Client) (*blueprint.TrackSearchResult, error){
				"tidal":      tidal.SearchTrackWithTitle,
				"ytmusic":    ytmusic.SearchTrackWithTitle,
				"applemusic": applemusic.SearchTrackWithTitle,
			}
			emptyResult := map[string]*blueprint.TrackSearchResult{"": nil}

			for platform, method := range searchMethods {
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] info - searching for track %s on %s", strippedTextInfo.Title, platform)
				track, err := method(tt, deezerTrack.Artists[0], red)
				if err != nil {
					if err == blueprint.ENORESULT {
						emptyResult[platform] = emptyTrack
					}
					log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get %s track", platform)
					continue
				}
				switch platform {
				case "tidal":
					conversion.Platforms.Tidal = track
				case "ytmusic":
					conversion.Platforms.YTMusic = track
				case "applemusic":
					conversion.Platforms.AppleMusic = track
				}
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] info - found track %s on %s", strippedTextInfo.Title, platform)
				continue
			}

			spotifyService := spotify.NewService(spotifyIntegrationCreds.AppID, spotifyIntegrationCreds.AppSecret, red)
			spSingleTrack, err := spotifyService.SearchTrackWithTitle(tt, deezerTrack.Artists[0],
				spotifyIntegrationCreds.AppID, spotifyIntegrationCreds.AppSecret, red)
			if err != nil {
				if err == blueprint.ENORESULT {
					conversion.Platforms.Spotify = nil
				}
			}
			conversion.Platforms.Spotify = spSingleTrack
		}
		// if the target platform is not all, then we only search for the target platform
		if targetPlatform != "all" {
			// if the target platform is spotify, then we use the spotify search method. this is because the spotify search method is different from the other platforms
			if targetPlatform == "spotify" {
				log.Printf("integrations: %s, %s", spotifyIntegrationCreds.AppID, spotifyIntegrationCreds.AppSecret)
				spotifyService := spotify.NewService(spotifyIntegrationCreds.AppID, spotifyIntegrationCreds.AppSecret, red)
				spSingleTrack, err := spotifyService.SearchTrackWithTitle(tt, deezerTrack.Artists[0], spotifyIntegrationCreds.AppID, spotifyIntegrationCreds.AppSecret, red)
				if err != nil {
					if err == blueprint.ENORESULT {
						conversion.Platforms.Spotify = nil
					}
				}
				conversion.Platforms.Spotify = spSingleTrack
			} else {
				// if the target platform is not spotify, then we use the other platforms search method
				var searchMethods = map[string]func(title, artist string, red *redis.Client) (*blueprint.TrackSearchResult, error){
					"tidal":      tidal.SearchTrackWithTitle,
					"ytmusic":    ytmusic.SearchTrackWithTitle,
					"applemusic": applemusic.SearchTrackWithTitle,
				}
				searchMethod, ok := searchMethods[targetPlatform]
				if !ok {
					log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get %s track", targetPlatform)
					return nil, blueprint.ENORESULT
				}
				track, err := searchMethod(tt, deezerTrack.Artists[0], red)
				if err != nil {
					log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get %s track", targetPlatform)
					return nil, blueprint.ENORESULT
				}
				switch targetPlatform {
				case "tidal":
					conversion.Platforms.Tidal = track
				case "ytmusic":
					conversion.Platforms.YTMusic = track
				case "applemusic":
					conversion.Platforms.AppleMusic = track
				}
			}
		}
	case spotify.IDENTIFIER:
		log.Printf("[controllers][platforms][deezer][ConvertEntity] info - converting spotify track")
		spotifyService := spotify.NewService(spotifyIntegrationCreds.AppID, spotifyIntegrationCreds.AppSecret, red)
		spotifyTrack, err := spotifyService.SearchTrackWithID(info.EntityID, spotifyIntegrationCreds.AppID, spotifyIntegrationCreds.AppSecret, red)
		if err != nil {
			log.Printf("[controllers][platforms][deezer][ConvertEntity] error - could not get spotify track")
			return nil, blueprint.ENORESULT
		}
		conversion.Platforms.Spotify = spotifyTrack
		trackTitle := spotifyTrack.Title

		if targetPlatform == info.Platform {
			log.Printf("[controllers][platforms][deezer][ConvertEntity] info - trying to convert spotify track to same platform")
			break
		}

		if targetPlatform == "all" {
			log.Printf("[controllers][platforms][deezer][ConvertEntity] info - converting spotify track to all platforms")
			var searchMethods = map[string]func(title, artist string, red *redis.Client) (*blueprint.TrackSearchResult, error){
				"tidal":      tidal.SearchTrackWithTitle,
				"ytmusic":    ytmusic.SearchTrackWithTitle,
				"applemusic": applemusic.SearchTrackWithTitle,
				"deezer":     deezer.SearchTrackWithTitle,
			}
			var emptyResult = map[string]*blueprint.TrackSearchResult{"": nil}
			var emptyTrack = &blueprint.TrackSearchResult{}
			for platform, method := range searchMethods {
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] info - searching for track %s on %s", trackTitle, platform)
				track, err := method(trackTitle, spotifyTrack.Artists[0], red)
				if err != nil {
					if err == blueprint.ENORESULT {
						emptyResult[platform] = emptyTrack
					}
					log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get %s track", platform)
					continue
				}
				switch platform {
				case "tidal":
					conversion.Platforms.Tidal = track
				case "ytmusic":
					conversion.Platforms.YTMusic = track
				case "applemusic":
					conversion.Platforms.AppleMusic = track
				case "deezer":
					conversion.Platforms.Deezer = track

				}
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] info - found track %s on %s", trackTitle, platform)
				continue
			}
		} else {
			log.Printf("[controllers][platforms][deezer][ConvertEntity] info - converting spotify track to %s", targetPlatform)
			var searchMethods = map[string]func(title, artist string, red *redis.Client) (*blueprint.TrackSearchResult, error){
				"tidal":      tidal.SearchTrackWithTitle,
				"ytmusic":    ytmusic.SearchTrackWithTitle,
				"applemusic": applemusic.SearchTrackWithTitle,
				"deezer":     deezer.SearchTrackWithTitle,
			}
			//var emptyResult = map[string]*blueprint.TrackSearchResult{"": nil}
			//var emptyTrack = &blueprint.TrackSearchResult{}
			searchMethod, ok := searchMethods[targetPlatform]
			if !ok {
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get %s track", targetPlatform)
				return nil, blueprint.ENORESULT
			}
			track, err := searchMethod(trackTitle, spotifyTrack.Artists[0], red)
			if err != nil {
				if err == blueprint.ENORESULT {
					log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get %s track. No result", targetPlatform)
					break
				}

				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get %s track", targetPlatform)
				return nil, blueprint.ENORESULT
			}

			switch targetPlatform {
			case "tidal":
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] info - searching track to %s convert on TIDAL", trackTitle)
				spew.Dump(track)
				conversion.Platforms.Tidal = track
			case "ytmusic":
				conversion.Platforms.YTMusic = track
			case "applemusic":
				conversion.Platforms.AppleMusic = track
			case "deezer":
				conversion.Platforms.Deezer = track
			}
		}
	}
	log.Printf("[controllers][platforms][deezer][ConvertEntity] info - conversion done")
	return &conversion, nil
	//
	//	tidalTrack, err := tidal.SearchTrackWithTitle(trackTitle, deezerTrack.Artists[0], red)
	//
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.Tidal = nil
	//		}
	//	}
	//
	//	ytmusicTrack, err := ytmusic.SearchTrackWithTitle(trackTitle, deezerTrack.Artists[0], red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.YTMusic = nil
	//		}
	//	}
	//
	//	apple, err := applemusic.SearchTrackWithTitle(trackTitle, deezerTrack.Artists[0], red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - could not get apple music track")
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.AppleMusic = nil
	//		}
	//	}
	//
	//	conversion.Platforms.Deezer = deezerTrack
	//	conversion.Platforms.Spotify = spSingleTrack
	//	conversion.Platforms.Tidal = tidalTrack
	//	conversion.Platforms.YTMusic = ytmusicTrack
	//	conversion.Platforms.AppleMusic = apple
	//
	//	if deezerTrack != nil {
	//		deezerCacheKey = "deezer:track:" + deezerTrack.ID
	//	}
	//
	//	if tidalTrack != nil {
	//		tidalCacheKey = "tidal:track:" + tidalTrack.ID
	//	}
	//
	//	if ytmusicTrack != nil {
	//		ytmusicCacheKey = "ytmusic:track:" + ytmusicTrack.ID
	//	}
	//
	//	if spSingleTrack != nil {
	//		spotifyCacheKey = "spotify:track:" + spSingleTrack.ID
	//	}
	//
	//	if apple != nil {
	//		appleCacheKey = "applemusic:track:" + apple.ID
	//	}
	//
	//case spotify.IDENTIFIER:
	//	spSingleTrack, err := spotify.SearchTrackWithID(info.EntityID, integrationAppID, integrationAppSecret, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - could not search track with ID from spotify: %v\n", err)
	//		return nil, err
	//	}
	//
	//	dzSingleTrack, err := deezer.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artists[0], red)
	//	if err != nil && err != blueprint.ENORESULT {
	//		log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - could not search track with title '%s' on deezer. err %v\n", spSingleTrack.Title, err)
	//		return nil, err
	//	}
	//
	//	if err != nil && err == blueprint.ENORESULT {
	//		log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - could not search track with title %s on deezer. No result found\n", spSingleTrack.Title)
	//	}
	//
	//	tidalTrack, err := tidal.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artists[0], red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.Tidal = nil
	//		}
	//	}
	//
	//	ytmusicTrack, err := ytmusic.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artists[0], red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.YTMusic = nil
	//		}
	//	}
	//
	//	apple, err := applemusic.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artists[0], red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.AppleMusic = nil
	//		}
	//	}
	//
	//	conversion.Platforms.Tidal = tidalTrack
	//	conversion.Platforms.YTMusic = ytmusicTrack
	//	conversion.Platforms.Spotify = spSingleTrack
	//	conversion.Platforms.Deezer = dzSingleTrack
	//	conversion.Platforms.AppleMusic = apple
	//
	//	if dzSingleTrack != nil {
	//		deezerCacheKey = "deezer:track:" + dzSingleTrack.ID
	//	}
	//
	//	if tidalTrack != nil {
	//		tidalCacheKey = "tidal:track:" + tidalTrack.ID
	//	}
	//
	//	if ytmusicTrack != nil {
	//		ytmusicCacheKey = "ytmusic:track:" + ytmusicTrack.ID
	//	}
	//
	//	if spSingleTrack != nil {
	//		spotifyCacheKey = "spotify:track:" + spSingleTrack.ID
	//	}
	//
	//	if apple != nil {
	//		appleCacheKey = "applemusic:track:" + apple.ID
	//	}
	//
	//case tidal.IDENTIFIER:
	//	tidalTrack, err := tidal.SearchWithID(info.EntityID, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][tidal][ConvertEntity] error - could not fetch track with ID from tidal: %v\n", err)
	//		return nil, err
	//	}
	//
	//	if len(tidalTrack.Artists) == 0 {
	//		log.Printf("\n[controllers][platforms][tidal][ConvertEntity] error - could not fetch track with ID from tidal: %v\n", err)
	//		return nil, err
	//	}
	//	// then search on spotify
	//	tidalArtist := tidalTrack.Artists[0]
	//	//tidalAlbum := tidalTrack.Album
	//
	//	spotifyTrack, err := spotify.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, integrationAppID, integrationAppSecret, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][tidal][ConvertEntity] error - could not search track with ID from spotify: %v\n", err)
	//		conversion.Platforms.Spotify = nil
	//	}
	//	deezerSingleTrack, err := deezer.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][tidal][ConvertEntity] error - could not search track with ID from deezer: %v\n", err)
	//		conversion.Platforms.Deezer = nil
	//	}
	//
	//	ytmusicTrack, err := ytmusic.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.YTMusic = nil
	//		}
	//	}
	//
	//	apple, err := applemusic.SearchTrackWithTitle(tidalTrack.Title, tidalArtist, red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.AppleMusic = nil
	//		}
	//	}
	//
	//	conversion.Platforms.Spotify = spotifyTrack
	//	conversion.Platforms.Deezer = deezerSingleTrack
	//	conversion.Platforms.Tidal = tidalTrack
	//	conversion.Platforms.YTMusic = ytmusicTrack
	//	conversion.Platforms.AppleMusic = apple
	//
	//	if spotifyTrack != nil {
	//		//cacheKeys = append(cacheKeys, "spotify:"+spotifyTrack.ID)
	//		spotifyCacheKey = "spotify:track:" + spotifyTrack.ID
	//	}
	//
	//	if deezerSingleTrack != nil {
	//		//cacheKeys = append(cacheKeys, "deezer:"+deezerSingleTrack.ID)
	//		deezerCacheKey = "deezer:track:" + deezerSingleTrack.ID
	//	}
	//
	//	if tidalTrack != nil {
	//		//cacheKeys = append(cacheKeys, "tidal:"+tidalTrack.ID)
	//		tidalCacheKey = "tidal:track:" + tidalTrack.ID
	//	}
	//
	//	if ytmusicTrack != nil {
	//		//cacheKeys = append(cacheKeys, "ytmusic:"+ytmusicTrack.ID)
	//		ytmusicCacheKey = "ytmusic:track:" + ytmusicTrack.ID
	//	}
	//
	//	if apple != nil {
	//		//cacheKeys = append(cacheKeys, "applemusic:"+apple.ID)
	//		appleCacheKey = "applemusic:track:" + apple.ID
	//	}
	//
	//case ytmusic.IDENTIFIER:
	//	ytmusicTrack, err := ytmusic.SearchTrackWithLink(info, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][ytmusic][ConvertEntity] error - could not fetch track with ID from ytmusic: %v\n", err)
	//		return nil, err
	//	}
	//
	//	if len(ytmusicTrack.Artists) == 0 {
	//		log.Printf("\n[controllers][platforms][ytmusic][ConvertEntity] error - could not fetch track with ID from ytmusic: %v\n", err)
	//		return nil, err
	//	}
	//
	//	ytmusicArtiste := ytmusicTrack.Artists[0]
	//	//ytmusicAlbum := ytmusicTrack.Album
	//	spotifyTrack, err := spotify.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicArtiste, integrationAppID, integrationAppSecret, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][ytmusic][ConvertEntity] error - could not search track with ID from spotify: %v\n", err)
	//		conversion.Platforms.Spotify = nil
	//	}
	//	deezerSingleTrack, err := deezer.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicArtiste, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][ytmusic][ConvertEntity] error - could not search track with ID from deezer: %v\n", err)
	//		conversion.Platforms.Deezer = nil
	//	}
	//	tidalTrack, err := tidal.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicArtiste, red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.Tidal = nil
	//		}
	//	}
	//
	//	apple, err := applemusic.SearchTrackWithTitle(ytmusicTrack.Title, ytmusicTrack.Artists[0], red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.AppleMusic = nil
	//		}
	//	}
	//
	//	conversion.Platforms.Spotify = spotifyTrack
	//	conversion.Platforms.Deezer = deezerSingleTrack
	//	conversion.Platforms.Tidal = tidalTrack
	//	conversion.Platforms.YTMusic = ytmusicTrack
	//	conversion.Platforms.AppleMusic = apple
	//
	//	if spotifyTrack != nil {
	//		//cacheKeys = append(cacheKeys, "spotify:"+spotifyTrack.ID)
	//		spotifyCacheKey = "spotify:track:" + spotifyTrack.ID
	//	}
	//
	//	if deezerSingleTrack != nil {
	//		//cacheKeys = append(cacheKeys, "deezer:"+deezerSingleTrack.ID)
	//		deezerCacheKey = "deezer:track:" + deezerSingleTrack.ID
	//	}
	//
	//	if tidalTrack != nil {
	//		//cacheKeys = append(cacheKeys, "tidal:"+tidalTrack.ID)
	//		tidalCacheKey = "tidal:track:" + tidalTrack.ID
	//	}
	//
	//	if ytmusicTrack != nil {
	//		//cacheKeys = append(cacheKeys, "ytmusic:"+ytmusicTrack.ID)
	//		ytmusicCacheKey = "ytmusic:track:" + ytmusicTrack.ID
	//	}
	//
	//case applemusic.IDENTIFIER:
	//	apple, err := applemusic.SearchTrackWithLink(info, red)
	//	if err != nil {
	//		log.Printf("\n[controller][platforms][applemusic][ConvertEntity] error - could not get apple music track")
	//		return nil, err
	//	}
	//
	//	artiste := apple.Artists[0]
	//	title := apple.Title
	//	//album := apple.Album
	//	spotifyTrack, err := spotify.SearchTrackWithTitle(title, artiste, integrationAppID, integrationAppSecret, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][applemusic][ConvertEntity] error - could not search track with ID from spotify: %v\n", err)
	//		conversion.Platforms.Spotify = nil
	//	}
	//	deezerSingleTrack, err := deezer.SearchTrackWithTitle(title, artiste, red)
	//	if err != nil {
	//		log.Printf("\n[controllers][platforms][applemusic][ConvertEntity] error - could not search track with ID from deezer: %v\n", err)
	//		conversion.Platforms.Deezer = nil
	//	}
	//	tidalTrack, err := tidal.SearchTrackWithTitle(title, artiste, red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.Tidal = nil
	//		}
	//	}
	//
	//	ytmusicTrack, err := ytmusic.SearchTrackWithTitle(artiste, artiste, red)
	//	if err != nil {
	//		if err == blueprint.ENORESULT {
	//			conversion.Platforms.YTMusic = nil
	//		}
	//	}
	//
	//	conversion.Platforms.Spotify = spotifyTrack
	//	conversion.Platforms.Deezer = deezerSingleTrack
	//	conversion.Platforms.Tidal = tidalTrack
	//	conversion.Platforms.YTMusic = ytmusicTrack
	//	conversion.Platforms.AppleMusic = apple
	//
	//	if spotifyTrack != nil {
	//		//cacheKeys = append(cacheKeys, "spotify:"+spotifyTrack.ID)
	//		spotifyCacheKey = "spotify:track:" + spotifyTrack.ID
	//	}
	//
	//	if deezerSingleTrack != nil {
	//		//cacheKeys = append(cacheKeys, "deezer:"+deezerSingleTrack.ID)
	//		deezerCacheKey = "deezer:track:" + deezerSingleTrack.ID
	//	}
	//
	//	if tidalTrack != nil {
	//		//cacheKeys = append(cacheKeys, "tidal:"+tidalTrack.ID)
	//		tidalCacheKey = "tidal:track:" + tidalTrack.ID
	//	}
	//
	//	if ytmusicTrack != nil {
	//		//cacheKeys = append(cacheKeys, "ytmusic:"+ytmusicTrack.ID)
	//		ytmusicCacheKey = "ytmusic:track:" + ytmusicTrack.ID
	//	}
	//
	//default:
	//	return nil, blueprint.ENOTIMPLEMENTED
	//}
	//
	//// create a map from spotifyCacheKey and deezerCacheKey
	//cacheMap := map[string]*blueprint.TrackSearchResult{
	//	spotifyCacheKey: conversion.Platforms.Spotify,
	//	deezerCacheKey:  conversion.Platforms.Deezer,
	//	tidalCacheKey:   conversion.Platforms.Tidal,
	//	ytmusicCacheKey: conversion.Platforms.YTMusic,
	//	appleCacheKey:   conversion.Platforms.AppleMusic,
	//}
	//
	//err := CacheTracksWithID(cacheMap, red)
	//if err != nil {
	//	log.Printf("\n[controllers][platforms][spotify][ConvertEntity] warning - could not cache tracks: %v\n", err)
	//}

	//return &conversion, nil
}

// ConvertPlaylist converts a playlist from one platform to another

func ConvertPlaylist(info *blueprint.LinkInfo, red *redis.Client, integrationAppID, integrationAppSecret string) (*blueprint.PlaylistConversion, error) {
	var conversion blueprint.PlaylistConversion

	// todo: handle ebadrequest error where this function is called
	if info.TargetPlatform == "" {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] no target platform specified %s\n", info.EntityID)
		return nil, blueprint.EBADREQUEST
	}

	switch info.Platform {
	case deezer.IDENTIFIER:
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] converting playlist %s\n", info.EntityID)
		var deezerPlaylist, tracklistErr = deezer.FetchPlaylistTracksAndInfo(info.EntityID, red)
		if tracklistErr != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][error] - Could not fetch tracklist from deezer %v\n", tracklistErr)
			return nil, tracklistErr
		}
		conversion.Meta.URL = deezerPlaylist.URL
		conversion.Meta.Title = deezerPlaylist.Title
		conversion.Meta.Length = deezerPlaylist.Length
		conversion.Meta.Owner = deezerPlaylist.Owner
		conversion.Meta.Cover = deezerPlaylist.Cover

		/**
		what the structure looks like
			{
			  "spotify": [{ Title: '', URL: ''}, { Title: '', URL: ''}],
			  "deezer": [{ Title: '', URL: ''}, { Title: '', URL: ''}]
			}
		*/
		conversion.Platforms.Deezer = &blueprint.PlatformPlaylistTrackResult{
			Tracks:        &deezerPlaylist.Tracks,
			Length:        sumUpResultLength(&deezerPlaylist.Tracks),
			OmittedTracks: nil,
		}
		conversion.Platforms.Spotify = nil
		conversion.Platforms.Tidal = nil
		conversion.Platforms.AppleMusic = nil

		log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer] - fetching track in the playlist with url: %v\n", deezerPlaylist.URL)
		err := CachePlaylistTracksWithID(&deezerPlaylist.Tracks, "deezer", red)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer][warning] - Could not cache deezer tracks for playlist %s: %v\n", deezerPlaylist.Title, err)
		}

		if info.TargetPlatform == spotify.IDENTIFIER {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetching tracks for playlist from spotify %s\n", info.EntityID)

			spotifyService := spotify.NewService(integrationAppID, integrationAppSecret, red)
			spotifyTracks, omittedSpotifyTracks := spotifyService.FetchPlaylistSearchResult(deezerPlaylist, red, integrationAppID, integrationAppSecret)
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetchde playlist tracks from spotify %s\n", info.EntityID)
			conversion.Platforms.Spotify = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        spotifyTracks,
				Length:        sumUpResultLength(spotifyTracks),
				OmittedTracks: *omittedSpotifyTracks,
			}
			err := CachePlaylistTracksWithID(spotifyTracks, "spotify", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer][warning] - Could not cache spotify tracks for playlist %s: %v\n", deezerPlaylist.Title, err)
			}

			return &conversion, nil
		}

		if info.TargetPlatform == tidal.IDENTIFIER {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetching tracks for playlist from tidal %s\n", info.EntityID)
			tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(deezerPlaylist, red)
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetchde playlist tracks from tidal %s\n", info.EntityID)

			conversion.Platforms.Tidal = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        tidalTracks,
				Length:        sumUpResultLength(tidalTracks),
				OmittedTracks: *omittedTidalTracks,
			}
			err := CachePlaylistTracksWithID(tidalTracks, "tidal", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer][warning] - Could not cache tidal tracks for playlist %s: %v\n", deezerPlaylist.Title, err)
			}

			return &conversion, nil
		}

		if info.TargetPlatform == applemusic.IDENTIFIER {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetching tracks for playlist from apple music %s\n", info.EntityID)
			appleTracks, omittedAppleTracks := applemusic.FetchPlaylistSearchResult(deezerPlaylist, red)
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist][deezer] fetchde playlist tracks from apple music %s\n", info.EntityID)
			conversion.Platforms.AppleMusic = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        appleTracks,
				Length:        sumUpResultLength(appleTracks),
				OmittedTracks: *omittedAppleTracks,
			}
			err := CachePlaylistTracksWithID(appleTracks, "applemusic", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][ConvertPlaylist][deezer][warning] - Could not cache apple music tracks for playlist %s: %v\n", deezerPlaylist.Title, err)
			}

			return &conversion, nil
		}

		return &conversion, nil
	case spotify.IDENTIFIER:
		log.Printf("\n[controllers][platforms][ConvertPlaylist][spotify] - converting playlist with id: %v\n", info.EntityID)
		entityID := info.EntityID
		spotifyService := spotify.NewService(integrationAppID, integrationAppSecret, red)
		spotifyPlaylist, _, err := spotifyService.FetchPlaylistTracksAndInfo(entityID, integrationAppID, integrationAppSecret, red)

		// for whatever reason, the spotify API does not return the playlist. Probably because it is private
		if err != nil && err != spotify2.ErrNoMorePages {
			log.Printf("\n[controllers][platforms][base] Error fetching playlist tracks and info from spotify: %v\n", err)
			return nil, err
		}

		conversion.Meta.URL = spotifyPlaylist.URL
		conversion.Meta.Title = spotifyPlaylist.Title
		conversion.Meta.Length = spotifyPlaylist.Length
		conversion.Meta.Owner = spotifyPlaylist.Owner
		conversion.Meta.Cover = spotifyPlaylist.Cover

		conversion.Platforms.Spotify = &blueprint.PlatformPlaylistTrackResult{
			Tracks:        &spotifyPlaylist.Tracks,
			Length:        sumUpResultLength(&spotifyPlaylist.Tracks),
			OmittedTracks: nil,
		}
		conversion.Platforms.Tidal = nil
		conversion.Platforms.AppleMusic = nil
		conversion.Platforms.Deezer = nil

		log.Printf("\n[controllers][platforms][ConvertPlaylist][spotify] - fetching tracks in playlist %v\n", spotifyPlaylist.URL)
		err = CachePlaylistTracksWithID(&spotifyPlaylist.Tracks, "spotify", red)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] warning - could not cache spotify tracks: %v %v\n\n", err, spotifyPlaylist.Tracks)
		}

		if info.TargetPlatform == deezer.IDENTIFIER {
			log.Printf("\n[controllers][platforms][base][spotify] - fetching playlist tracks and info from deezer: %v\n", spotifyPlaylist)
			deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(spotifyPlaylist, red)
			conversion.Platforms.Deezer = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        deezerTracks,
				Length:        sumUpResultLength(deezerTracks),
				OmittedTracks: *omittedDeezerTracks,
			}
			log.Printf("\n[controllers][platforms][ConvertPlaylist][spotify] - caching tracks in playlist %v\n", spotifyPlaylist.URL)
			err = CachePlaylistTracksWithID(deezerTracks, "deezer", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] warning - could not cache deezer tracks: %v %v\n\n", err, deezerTracks)
			}

			return &conversion, nil
		}

		if info.TargetPlatform == tidal.IDENTIFIER {
			log.Printf("\n[controllers][platforms][base][spotify] - fetching playlist tracks and info from tidal: %v\n", spotifyPlaylist)
			tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(spotifyPlaylist, red)
			conversion.Platforms.Tidal = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        tidalTracks,
				Length:        sumUpResultLength(tidalTracks),
				OmittedTracks: *omittedTidalTracks,
			}

			err = CachePlaylistTracksWithID(tidalTracks, "tidal", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] warning - could not cache tidal tracks: %v %v\n\n", err, tidalTracks)
			}

			return &conversion, nil
		}

		if info.TargetPlatform == applemusic.IDENTIFIER {

			appleTracks, omittedAppleTracks := applemusic.FetchPlaylistSearchResult(spotifyPlaylist, red)
			conversion.Platforms.AppleMusic = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        appleTracks,
				Length:        sumUpResultLength(appleTracks),
				OmittedTracks: *omittedAppleTracks,
			}
			err = CachePlaylistTracksWithID(appleTracks, "applemusic", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] warning - could not cache apple music tracks: %v %v\n\n", err, appleTracks)
			}

			return &conversion, nil
		}
		return &conversion, nil

	case tidal.IDENTIFIER:
		log.Printf("\n[controllers][platforms][ConvertPlaylist][tidal] - converting playlist %v\n", info.EntityID)
		_, tidalPlaylist, _, err := tidal.FetchPlaylist(info.EntityID, red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] error - could not fetch playlist with ID from tidal: %v\n", err)
			return nil, err
		}
		conversion.Meta.URL = tidalPlaylist.URL
		conversion.Meta.Title = tidalPlaylist.Title
		conversion.Meta.Length = tidalPlaylist.Length
		conversion.Meta.Owner = tidalPlaylist.Owner
		conversion.Meta.Cover = tidalPlaylist.Cover

		conversion.Platforms.Tidal = &blueprint.PlatformPlaylistTrackResult{
			Tracks:        &tidalPlaylist.Tracks,
			Length:        sumUpResultLength(&tidalPlaylist.Tracks),
			OmittedTracks: nil,
		}

		conversion.Platforms.Deezer = nil
		conversion.Platforms.Spotify = nil
		conversion.Platforms.AppleMusic = nil

		err = CachePlaylistTracksWithID(conversion.Platforms.Tidal.Tracks, "tidal", red)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] warning - could not cache tracks for playlist %s: %v %v\n\n", tidalPlaylist.Title, err, tidalPlaylist.Tracks)
		}

		if info.TargetPlatform == deezer.IDENTIFIER {
			deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(tidalPlaylist, red)
			log.Printf("\n[controllers][platforms][base][tidal] - fetched playlist tracks and info from deezer: %v\n", deezerTracks)

			conversion.Platforms.Deezer = &blueprint.PlatformPlaylistTrackResult{Tracks: deezerTracks,
				Length: sumUpResultLength(deezerTracks), OmittedTracks: *omittedDeezerTracks}

			err = CachePlaylistTracksWithID(deezerTracks, "deezer", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] warning - could not cache deezer tracks: %v %v\n\n", err, deezerTracks)
			}
			return &conversion, nil
		}

		if info.TargetPlatform == spotify.IDENTIFIER {
			spotifyService := spotify.NewService(integrationAppID, integrationAppSecret, red)
			spotifyTracks, omittedSpotifyTracks := spotifyService.FetchPlaylistSearchResult(tidalPlaylist, red, integrationAppID, integrationAppSecret)
			conversion.Platforms.Spotify = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        spotifyTracks,
				Length:        sumUpResultLength(spotifyTracks),
				OmittedTracks: *omittedSpotifyTracks,
			}
			err = CachePlaylistTracksWithID(spotifyTracks, "spotify", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] warning - could not cache tracks: %v %v\n\n", err, spotifyTracks)
			}
			return &conversion, nil
		}

		if info.TargetPlatform == applemusic.IDENTIFIER {
			appleTracks, omittedAppleTracks := applemusic.FetchPlaylistSearchResult(tidalPlaylist, red)
			conversion.Platforms.AppleMusic = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        appleTracks,
				Length:        sumUpResultLength(appleTracks),
				OmittedTracks: *omittedAppleTracks,
			}

			log.Printf("\n[controllers][platforms][ConvertPlaylist][tidal] - converted playlist %v\n", tidalPlaylist.URL)
			err = CachePlaylistTracksWithID(appleTracks, "applemusic", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][tidal][ConvertPlaylist] warning - could not cache tracks: %v %v\n\n", err, appleTracks)
			}
			return &conversion, nil
		}

		return &conversion, nil

	case applemusic.IDENTIFIER:
		log.Printf("\n[controllers][platforms][ConvertPlaylist][applemusic] - converting playlist %v\n", info.EntityID)
		applePlaylist, err := applemusic.FetchPlaylistTrackList(info.EntityID, red)

		if err != nil {
			log.Printf("\n[controllers][platforms][applemusic][ConvertPlaylist] error - could not fetch playlist with ID from applemusic. perhaps its not public: %v\n", err)
			return nil, err
		}

		if applePlaylist == nil {
			log.Printf("\n[controllers][platforms][applemusic][ConvertPlaylist] error - could not fetch playlist with ID from applemusic: %v\n", err)
			return nil, blueprint.ENORESULT
		}

		// playlist meta
		conversion.Meta.URL = applePlaylist.URL
		conversion.Meta.Title = applePlaylist.Title
		conversion.Meta.Length = applePlaylist.Length
		conversion.Meta.Owner = applePlaylist.Owner
		conversion.Meta.Cover = applePlaylist.Cover

		// platform tracks
		conversion.Platforms.AppleMusic = &blueprint.PlatformPlaylistTrackResult{Tracks: &applePlaylist.Tracks, Length: sumUpResultLength(&applePlaylist.Tracks)}
		conversion.Platforms.Tidal = nil
		conversion.Platforms.Spotify = nil
		conversion.Platforms.Deezer = nil

		err = CachePlaylistTracksWithID(conversion.Platforms.AppleMusic.Tracks, "applemusic", red)
		if err != nil {
			log.Printf("\n[controllers][platforms][applemusic][ConvertPlaylist] warning - could not cache tracks for playlist %s: %v %v\n\n", applePlaylist.Title, err, applePlaylist.Tracks)
		}

		if info.TargetPlatform == deezer.IDENTIFIER {
			deezerTracks, omittedDeezerTracks := deezer.FetchPlaylistSearchResult(applePlaylist, red)
			conversion.Platforms.Deezer = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        deezerTracks,
				Length:        sumUpResultLength(deezerTracks),
				OmittedTracks: *omittedDeezerTracks,
			}

			err = CachePlaylistTracksWithID(deezerTracks, "deezer", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] warning - could not cache deezer tracks: %v %v\n\n", err, deezerTracks)
			}
			return &conversion, nil
		}

		if info.TargetPlatform == spotify.IDENTIFIER {
			spotifyService := spotify.NewService(integrationAppID, integrationAppSecret, red)
			spotifyTracks, omittedSpotifyTracks := spotifyService.FetchPlaylistSearchResult(applePlaylist, red, integrationAppID, integrationAppSecret)
			log.Printf("\n[controllers][platforms][base][applemusic] - fetched playlist tracks and info from spotify: %v\n", spotifyTracks)
			conversion.Platforms.Spotify = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        spotifyTracks,
				Length:        sumUpResultLength(spotifyTracks),
				OmittedTracks: *omittedSpotifyTracks,
			}

			err = CachePlaylistTracksWithID(spotifyTracks, "spotify", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] warning - could not cache spotify tracks: %v %v\n\n", err, spotifyTracks)
			}
			return &conversion, nil
		}

		if info.TargetPlatform == tidal.IDENTIFIER {
			tidalTracks, omittedTidalTracks := tidal.FetchTrackWithResult(applePlaylist, red)
			conversion.Platforms.Tidal = &blueprint.PlatformPlaylistTrackResult{
				Tracks:        tidalTracks,
				Length:        sumUpResultLength(tidalTracks),
				OmittedTracks: *omittedTidalTracks,
			}

			err = CachePlaylistTracksWithID(tidalTracks, "tidal", red)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] warning - could not cache tidal tracks: %v %v\n\n", err, tidalTracks)
			}
			return &conversion, nil
		}
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
			log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - could not cache track on %s: %v\n", cacheKey, err)
			return err
		}
		log.Printf("\n[controllers][platforms][universal][playlist][CacheTracksWithID] cache - track %s cached on %s\n", data.Title, cacheKey)
	}
	return nil
}

// CachePlaylistTracksWithID caches the results of each of the tracks from a playlist conversion, under the same key scheme as CacheTracksWithID
func CachePlaylistTracksWithID(tracks *[]blueprint.TrackSearchResult, platform string, red *redis.Client) error {
	for _, data := range *tracks {
		// stringify data
		dataJSON, err := json.Marshal(data)
		if err != nil {
			log.Printf("\n[controllers][platforms][base] Error marshalling track result data to JSON: %v\n", err)
			return err
		}
		if err := red.Set(context.Background(), fmt.Sprintf("%s:track:%s", platform, data.ID), string(dataJSON), time.Hour*24).Err(); err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - could not cache track on %s: %v\n", "spotify:"+data.ID, err)
			return err
		}
	}
	log.Printf("\n[controllers][platforms][CachePlaylistTracksWithID] cache - track ")
	return nil
}
