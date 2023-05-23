// package universal contains the logic for converting entities between platforms
// It is where cross-platform conversions and logic are handled and called

package universal

import (
	"context"
	"encoding/json"
	"fmt"
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
	"os"
	"reflect"
	"time"

	"github.com/go-redis/redis/v8"
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
func ConvertTrack(info *blueprint.LinkInfo, red *redis.Client, pg *sqlx.DB) (*blueprint.Conversion, error) {
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

	var fromService interface{}
	var toService interface{}

	//var fromPlatformIntegrationCreds blueprint.IntegrationCredentials
	//var toPlatformIntegrationCreds blueprint.IntegrationCredentials
	// platform we're converting from. we want to fetch the app credentials for this platform and also initialize the service
	// into the fromService interface
	switch info.Platform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no spotify credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}

		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt spotify credentials: %v\n", decErr)
			return nil, decErr
		}

		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal spotify credentials: %v\n", err)
			return nil, err
		}

		fromService = spotify.NewService(&credentials, pg, red)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no tidal credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}

		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt tidal credentials: %v\n", decErr)
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal tidal credentials: %v\n", err)
			return nil, err
		}
		fromService = tidal.NewService(&credentials, pg, red)
		//fromPlatformIntegrationCreds = credentials
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no deezer credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}

		credBytes, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt deezer credentials: %v\n", decErr)
			return nil, decErr
		}
		var credentials blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal deezer credentials: %v\n", err)
			return nil, err
		}
		fromService = deezer.NewService(&credentials, pg, red)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no apple music credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt apple music credentials: %v\n", decErr)
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal apple music credentials: %v\n", err)
			return nil, err
		}
		fromService = applemusic.NewService(&credentials, pg, red)
	case ytmusic.IDENTIFIER:
		// we dont need credentials for ytmusic yet but we still need to initialize the service
		fromService = ytmusic.NewService(red)
		//fromPlatformIntegrationCreds = credentials
	}

	// platform we're converting to. similar to above in functionality
	switch targetPlatform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no spotify credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}

		var credentials blueprint.IntegrationCredentials
		err = json.Unmarshal(app.SpotifyCredentials, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal spotify credentials: %v\n", err)
			return nil, err
		}

		toService = spotify.NewService(&credentials, pg, red)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no tidal credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, dErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt tidal credentials: %v\n", dErr)
			return nil, dErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal tidal credentials: %v\n", err)
			return nil, err
		}

		toService = tidal.NewService(&credentials, pg, red)
	case deezer.IDENTIFIER:
		var credentials blueprint.IntegrationCredentials
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no deezer credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}
		credBytes, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt deezer credentials: %v\n", decErr)
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal deezer credentials: %v\n", err)
			return nil, err
		}
		toService = deezer.NewService(&credentials, pg, red)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no apple music credentials provided\n")
			return nil, blueprint.ECREDENTIALSMISSING
		}
		var credentials blueprint.IntegrationCredentials

		credBytes, decErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt apple music credentials: %v\n", decErr)
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal apple music credentials: %v\n", err)
			return nil, err
		}
		toService = applemusic.NewService(&credentials, pg, red)
	}

	var methodSearchTrackWithID, ok = util.FetchMethodFromInterface(fromService, "SearchTrackWithID")
	if !ok {
		log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not fetch method from interface\n")
		return nil, blueprint.EUNKNOWN
	}

	var methodSearchTrackWithTitle, ok2 = util.FetchMethodFromInterface(toService, "SearchTrackWithTitle")
	if !ok2 {
		log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not fetch method from interface\n")
		return nil, blueprint.EUNKNOWN
	}

	var fromResult *blueprint.TrackSearchResult
	var toResult *blueprint.TrackSearchResult
	if methodSearchTrackWithID.IsValid() {
		ins := make([]reflect.Value, 2)
		ins[0] = reflect.ValueOf(info)
		ans := methodSearchTrackWithID.Call([]reflect.Value{ins[0]})
		res, ok1 := ans[0].Interface().(*blueprint.TrackSearchResult)
		if !ok1 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not convert interface to TrackSearchResult.. Error dynamically calling fromMethod.\n")
			return nil, blueprint.EUNKNOWN
		}
		fromResult = res
		// todo: implement nil check
		if methodSearchTrackWithTitle.IsValid() {
			ins2 := make([]reflect.Value, 2)
			ins2[0] = reflect.ValueOf(res.Title)
			ins2[1] = reflect.ValueOf(res.Artists[0])
			ans2 := methodSearchTrackWithTitle.Call([]reflect.Value{ins2[0], ins2[1]})
			res2, ok3 := ans2[0].Interface().(*blueprint.TrackSearchResult)
			if !ok3 {
				log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not convert interface to TrackSearchResult.. Error dynamically calling toMethod.\n")
				return nil, blueprint.EUNKNOWN
			}
			toResult = res2
		}
	}

	switch info.Platform {
	case spotify.IDENTIFIER:
		conversion.Platforms.Spotify = fromResult
	case tidal.IDENTIFIER:
		conversion.Platforms.Tidal = fromResult
	case applemusic.IDENTIFIER:
		conversion.Platforms.AppleMusic = fromResult
	case deezer.IDENTIFIER:
		conversion.Platforms.Deezer = fromResult
	case ytmusic.IDENTIFIER:
		conversion.Platforms.YTMusic = fromResult
	}

	switch info.TargetPlatform {
	case spotify.IDENTIFIER:
		conversion.Platforms.Spotify = toResult
	case tidal.IDENTIFIER:
		conversion.Platforms.Tidal = toResult
	case applemusic.IDENTIFIER:
		conversion.Platforms.AppleMusic = toResult
	case deezer.IDENTIFIER:
		conversion.Platforms.Deezer = toResult
	case ytmusic.IDENTIFIER:
		conversion.Platforms.YTMusic = toResult
	}

	log.Printf("[controllers][platforms][deezer][ConvertEntity] info - conversion done")
	return &conversion, nil
}

// ConvertPlaylist converts a playlist from one platform to another
func ConvertPlaylist(info *blueprint.LinkInfo, red *redis.Client, pg *sqlx.DB) (*blueprint.PlaylistConversion, error) {
	var conversion blueprint.PlaylistConversion
	conversion.Meta.Entity = "playlist"

	database := db.NewDB{DB: pg}
	app, err := database.FetchAppByAppId(info.App)
	if err != nil {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not fetch app %s\n", err)
		return nil, err
	}
	targetPlatform := info.TargetPlatform
	if targetPlatform == "" {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] no target platform specified %s\n", info.EntityID)
		return nil, blueprint.EBADREQUEST
	}
	var fromService, toService interface{}

	switch info.Platform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no spotify credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt spotify credentials\n")
			return nil, decErr
		}
		if err := json.Unmarshal(credBytes, &credentials); err != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal spotify credentials\n")
			return nil, err
		}
		fromService = spotify.NewService(&credentials, pg, red)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no tidal credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt tidal credentials\n")
			return nil, decErr
		}
		if pErr := json.Unmarshal(credBytes, &credentials); pErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal tidal credentials\n")
			return nil, pErr
		}
		fromService = tidal.NewService(&credentials, pg, red)
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no deezer credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt deezer credentials\n")
			return nil, decErr
		}
		if pErr := json.Unmarshal(credBytes, &credentials); pErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal deezer credentials\n")
			return nil, pErr
		}
		fromService = deezer.NewService(&credentials, pg, red)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no applemusic credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt applemusic credentials\n")
			return nil, decErr
		}
		if pErr := json.Unmarshal(credBytes, &credentials); pErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal applemusic credentials\n")
			return nil, pErr
		}
		fromService = applemusic.NewService(&credentials, pg, red)
	}

	switch targetPlatform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no spotify credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt spotify credentials\n")
			return nil, decErr
		}
		if pErr := json.Unmarshal(credBytes, &credentials); pErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal spotify credentials\n")
			return nil, pErr
		}
		toService = spotify.NewService(&credentials, pg, red)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no tidal credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt tidal credentials\n")
			return nil, decErr
		}
		if pErr := json.Unmarshal(credBytes, &credentials); pErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal tidal credentials\n")
			return nil, pErr
		}
		toService = tidal.NewService(&credentials, pg, red)
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no deezer credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt deezer credentials\n")
			return nil, decErr
		}
		if pErr := json.Unmarshal(credBytes, &credentials); pErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal deezer credentials\n")
			return nil, pErr
		}
		toService = deezer.NewService(&credentials, pg, red)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no applemusic credentials\n")
			return nil, blueprint.EBADREQUEST
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not decrypt applemusic credentials\n")
			return nil, decErr
		}
		if pErr := json.Unmarshal(credBytes, &credentials); pErr != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not unmarshal applemusic credentials\n")
			return nil, pErr
		}
		toService = applemusic.NewService(&credentials, pg, red)

	}

	var methodSearchPlaylistWithID, ok = util.FetchMethodFromInterface(fromService, "SearchPlaylistWithID")
	if !ok {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not fetch method from interface\n")
		return nil, blueprint.EUNKNOWN
	}

	var methodSearchPlaylistWithTracks, ok2 = util.FetchMethodFromInterface(toService, "SearchPlaylistWithTracks")
	if !ok2 {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not fetch method from interface\n")
		return nil, blueprint.EUNKNOWN
	}
	var idSearchResult *blueprint.PlaylistSearchResult
	var omittedTracks *[]blueprint.OmittedTracks
	var tracksSearchResult *[]blueprint.TrackSearchResult

	if methodSearchPlaylistWithID.IsValid() {
		ins := make([]reflect.Value, 1)
		ins[0] = reflect.ValueOf(info.EntityID)
		outs := methodSearchPlaylistWithID.Call(ins)
		if len(outs) > 0 {
			if outs[0].Interface() == nil {
				return nil, blueprint.ENORESULT
			}
			// for playlist results, the second result returned from method call is a pointer to the playlist search result from source platform
			if outs[0].Interface() != nil {
				idSearchResult = outs[0].Interface().(*blueprint.PlaylistSearchResult)
			}
			// then use the above playlist info to search for srcPlatformTracks, on target platform
			if methodSearchPlaylistWithTracks.IsValid() {
				ins2 := make([]reflect.Value, 1)
				ins2[0] = reflect.ValueOf(idSearchResult)
				outs2 := methodSearchPlaylistWithTracks.Call(ins2)
				if len(outs2) > 0 {
					if outs2[0].Interface() == nil {
						return nil, blueprint.ENORESULT
					}
					// the first result returned from the method call is a pointer to an array of track search results from target platform
					tracksSearchResult = outs2[0].Interface().(*[]blueprint.TrackSearchResult)
					// the second result returned from the method call is a pointer to the omitted srcPlatformTracks from the playlist
					if outs2[1].Interface() != nil {
						omittedTracks = outs2[1].Interface().(*[]blueprint.OmittedTracks)
					}
				}
			}
		}
	}

	if idSearchResult == nil {
		return nil, blueprint.ENORESULT
	}

	conversion.Meta.URL = idSearchResult.URL
	conversion.Meta.Title = idSearchResult.Title
	conversion.Meta.Length = idSearchResult.Length
	conversion.Meta.Owner = idSearchResult.Owner
	conversion.Meta.Cover = idSearchResult.Cover

	srcPlatformTracks := &blueprint.PlatformPlaylistTrackResult{
		Tracks:        &idSearchResult.Tracks,
		Length:        sumUpResultLength(&idSearchResult.Tracks),
		OmittedTracks: omittedTracks,
	}

	targetPlatformTracks := &blueprint.PlatformPlaylistTrackResult{
		Length:        sumUpResultLength(tracksSearchResult),
		Tracks:        tracksSearchResult,
		OmittedTracks: omittedTracks,
	}

	switch info.Platform {
	case deezer.IDENTIFIER:
		conversion.Platforms.Deezer = srcPlatformTracks
		pErr := CachePlaylistTracksWithID(&idSearchResult.Tracks, deezer.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	case spotify.IDENTIFIER:
		conversion.Platforms.Spotify = srcPlatformTracks
		pErr := CachePlaylistTracksWithID(&idSearchResult.Tracks, spotify.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	case applemusic.IDENTIFIER:
		conversion.Platforms.AppleMusic = srcPlatformTracks
		pErr := CachePlaylistTracksWithID(&idSearchResult.Tracks, applemusic.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	case tidal.IDENTIFIER:
		conversion.Platforms.Tidal = srcPlatformTracks
		pErr := CachePlaylistTracksWithID(&idSearchResult.Tracks, tidal.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	}

	switch info.TargetPlatform {
	case deezer.IDENTIFIER:
		conversion.Platforms.Deezer = targetPlatformTracks
		pErr := CachePlaylistTracksWithID(tracksSearchResult, deezer.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	case spotify.IDENTIFIER:
		conversion.Platforms.Spotify = targetPlatformTracks
		pErr := CachePlaylistTracksWithID(tracksSearchResult, spotify.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	case applemusic.IDENTIFIER:
		conversion.Platforms.AppleMusic = targetPlatformTracks
		pErr := CachePlaylistTracksWithID(tracksSearchResult, applemusic.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	case tidal.IDENTIFIER:
		conversion.Platforms.Tidal = targetPlatformTracks
		pErr := CachePlaylistTracksWithID(tracksSearchResult, tidal.IDENTIFIER, red)
		if pErr != nil {
			return nil, pErr
		}
	}
	return &conversion, nil
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
