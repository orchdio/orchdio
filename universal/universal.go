// package universal contains the logic for converting entities between platforms
// It is where cross-platform conversions and logic are handled and called

package universal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"reflect"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jmoiron/sqlx"

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
	var toService []map[string]interface{}

	//var fromPlatformIntegrationCreds blueprint.IntegrationCredentials
	//var toPlatformIntegrationCreds blueprint.IntegrationCredentials
	// platform we're converting from. we want to fetch the app credentials for this platform and also initialize the service
	// into the fromService interface
	// todo: refactor these to use the util.DecryptCredentials function
	switch info.Platform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no spotify credentials provided\n")
			return nil, errors.New("spotify credentials not provided")
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

		fromService = spotify.NewService(&credentials, pg, red, app)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no tidal credentials provided\n")
			return nil, errors.New("tidal credentials not provided")
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

		spew.Dump(&credentials)
		if credentials.AppRefreshToken == "" {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - tidal refresh token not present in TIDAL credentials. Please update the app and try again.\n")
			return nil, blueprint.ErrBadCredentials
		}

		fromService = tidal.NewService(&credentials, pg, red, app)
		//fromPlatformIntegrationCreds = credentials
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no deezer credentials provided\n")
			return nil, errors.New("deezer credentials not provided")
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
		fromService = deezer.NewService(&credentials, pg, red, app)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no apple music credentials provided\n")
			return nil, errors.New("apple music credentials not provided")
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

		if credentials.AppRefreshToken == "" {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - apple music credentials not provided\n")
			return nil, blueprint.ErrBadCredentials
		}

		fromService = applemusic.NewService(&credentials, pg, red, app)
	case ytmusic.IDENTIFIER:
		// we dont need credentials for ytmusic yet but we still need to initialize the service
		fromService = ytmusic.NewService(red, app)
		//fromPlatformIntegrationCreds = credentials
	}

	// platform we're converting to. similar to above in functionality
	switch targetPlatform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no spotify credentials provided\n")
			return nil, blueprint.ErrCredentialsMissing
		}

		var credentials blueprint.IntegrationCredentials
		err = json.Unmarshal(app.SpotifyCredentials, &credentials)
		if err != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not unmarshal spotify credentials: %v\n", err)
			return nil, err
		}

		s := spotify.NewService(&credentials, pg, red, app)
		toService = append(toService, map[string]interface{}{spotify.IDENTIFIER: s})
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no tidal credentials provided\n")
			return nil, errors.New("tidal credentials not provided")
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

		if credentials.AppRefreshToken == "" {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - tidal credentials not provided\n")
			return nil, blueprint.ErrBadCredentials
		}

		s := tidal.NewService(&credentials, pg, red, app)
		toService = append(toService, map[string]interface{}{tidal.IDENTIFIER: s})
	case deezer.IDENTIFIER:
		var credentials blueprint.IntegrationCredentials
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no deezer credentials provided\n")
			return nil, errors.New("deezer credentials not provided")
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
		s := deezer.NewService(&credentials, pg, red, app)
		toService = append(toService, map[string]interface{}{deezer.IDENTIFIER: s})
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no apple music credentials provided\n")
			return nil, errors.New("apple music credentials not provided")
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
		if credentials.AppRefreshToken == "" {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - apple music credentials not provided\n")
			return nil, blueprint.ErrBadCredentials
		}

		s := applemusic.NewService(&credentials, pg, red, app)
		toService = append(toService, map[string]interface{}{applemusic.IDENTIFIER: s})
	case ytmusic.IDENTIFIER:
		s := ytmusic.NewService(red, app)
		toService = append(toService, map[string]interface{}{ytmusic.IDENTIFIER: s})
	}

	if targetPlatform == "all" {
		log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - target platform is all\n")
		var appCredentials = map[string][]byte{
			spotify.IDENTIFIER:    app.SpotifyCredentials,
			tidal.IDENTIFIER:      app.TidalCredentials,
			deezer.IDENTIFIER:     app.DeezerCredentials,
			applemusic.IDENTIFIER: app.AppleMusicCredentials,
			ytmusic.IDENTIFIER:    nil,
		}

		for t, appCred := range appCredentials {
			// copy the key and value to avoid concurrency issues. dont trust the original map.
			plat := t
			// ytmusic doesnt require credentials yet so we skip it

			encryptedCred := appCred
			if len(encryptedCred) == 0 || encryptedCred == nil {
				log.Printf("\n[controllers][platforms][universal][ConvertTrack] warning - no credentials provided for %s\n", plat)
				if plat == ytmusic.IDENTIFIER {
					toService = append(toService, map[string]interface{}{ytmusic.IDENTIFIER: ytmusic.NewService(red, app)})
				}
				continue
			}
			decryptedCredentials, dErr := util.DecryptIntegrationCredentials(encryptedCred)
			if dErr != nil {
				log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt credentials: %v\n", dErr)
				return nil, dErr
			}
			switch plat {
			case ytmusic.IDENTIFIER:
				s := ytmusic.NewService(red, app)
				toService = append(toService, map[string]interface{}{ytmusic.IDENTIFIER: s})

			case spotify.IDENTIFIER:
				s := spotify.NewService(decryptedCredentials, pg, red, app)
				toService = append(toService, map[string]interface{}{spotify.IDENTIFIER: s})
			case tidal.IDENTIFIER:
				s := tidal.NewService(decryptedCredentials, pg, red, app)
				toService = append(toService, map[string]interface{}{tidal.IDENTIFIER: s})
			case deezer.IDENTIFIER:
				s := deezer.NewService(decryptedCredentials, pg, red, app)
				toService = append(toService, map[string]interface{}{deezer.IDENTIFIER: s})
			case applemusic.IDENTIFIER:
				s := applemusic.NewService(decryptedCredentials, pg, red, app)
				toService = append(toService, map[string]interface{}{applemusic.IDENTIFIER: s})
			}
		}
	}

	var methodSearchTrackWithID, ok = util.FetchMethodFromInterface(fromService, "SearchTrackWithID")
	if !ok {
		log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not fetch method from interface\n")
		return nil, blueprint.ErrUnknown
	}

	var fromResult *blueprint.TrackSearchResult
	var toResult []map[string]*blueprint.TrackSearchResult

	// DANGEROUS WATERS! TREAD WITH CAUTION - DYNAMICALLY CALLING METHODS
	if methodSearchTrackWithID.IsValid() {
		ins := make([]reflect.Value, 2)
		ins[0] = reflect.ValueOf(info)
		ans := methodSearchTrackWithID.Call([]reflect.Value{ins[0]})
		res, ok1 := ans[0].Interface().(*blueprint.TrackSearchResult)
		if !ok1 {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not convert interface to TrackSearchResult.. Error dynamically calling fromMethod.\n")
			return nil, blueprint.ErrUnknown
		}
		fromResult = res

		// fetch the conversion methods for the target platforms.
		for _, serviceInstance := range toService {
			if res == nil {
				continue
			}
			instance := serviceInstance
			for platform, service := range instance {
				plat := platform
				shadowService := service
				var methodSearchTrackWithTitle, ok2 = util.FetchMethodFromInterface(shadowService, "SearchTrackWithTitle")
				if !ok2 {
					return nil, blueprint.ErrUnknown
				}

				if methodSearchTrackWithTitle.IsValid() {
					searchData := &blueprint.TrackSearchData{
						Title:   res.Title,
						Artists: res.Artists,
					}

					params := make([]reflect.Value, 1)
					params[0] = reflect.ValueOf(searchData)
					searchTrackWithTitleResults := methodSearchTrackWithTitle.Call([]reflect.Value{params[0]})
					res2, ok3 := searchTrackWithTitleResults[0].Interface().(*blueprint.TrackSearchResult)
					if !ok3 {
						log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not convert interface to TrackSearchResult.. Error dynamically calling toMethod.\n")
						return nil, blueprint.ErrUnknown
					}
					toResult = append(toResult, map[string]*blueprint.TrackSearchResult{plat: res2})
				} else {
					return nil, blueprint.ErrUnknown
				}
			}
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

	for _, trackResult := range toResult {
		// copy target result into a temp shadowTrackResult variable to avoid concurrency issues
		shadowTrackResult := trackResult
		for p, result := range shadowTrackResult {
			plat := p
			shadowResult := result
			switch plat {
			case spotify.IDENTIFIER:
				conversion.Platforms.Spotify = shadowResult
			case tidal.IDENTIFIER:
				conversion.Platforms.Tidal = shadowResult
			case applemusic.IDENTIFIER:
				conversion.Platforms.AppleMusic = shadowResult
			case deezer.IDENTIFIER:
				conversion.Platforms.Deezer = shadowResult
			case ytmusic.IDENTIFIER:
				conversion.Platforms.YTMusic = shadowResult
			}
		}
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
		return nil, blueprint.ErrBadRequest
	}
	var fromService, toService interface{}

	// this block gets the credentials for the platform we're converting from.
	// important so that we can get the credentials for those available for the app
	// and then use to create the instance of the platform for the supported platforms available.
	// This is for the `fromService` interface
	switch info.Platform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no spotify credentials\n")
			return nil, blueprint.ErrBadRequest
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
		fromService = spotify.NewService(&credentials, pg, red, app)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no tidal credentials\n")
			return nil, blueprint.ErrBadRequest
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
		fromService = tidal.NewService(&credentials, pg, red, app)
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no deezer credentials\n")
			return nil, blueprint.ErrBadRequest
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
		fromService = deezer.NewService(&credentials, pg, red, app)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no applemusic credentials\n")
			return nil, blueprint.ErrBadRequest
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
		fromService = applemusic.NewService(&credentials, pg, red, app)
	}

	// todo: refactor this to use the same decrypt credentials
	switch targetPlatform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no spotify credentials\n")
			return nil, blueprint.ErrBadRequest
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
		toService = spotify.NewService(&credentials, pg, red, app)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no tidal credentials\n")
			return nil, blueprint.ErrBadRequest
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
		toService = tidal.NewService(&credentials, pg, red, app)
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no deezer credentials\n")
			return nil, blueprint.ErrBadRequest
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
		toService = deezer.NewService(&credentials, pg, red, app)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - no applemusic credentials\n")
			return nil, blueprint.ErrBadRequest
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
		toService = applemusic.NewService(&credentials, pg, red, app)

	}

	var methodSearchPlaylistWithID, ok = util.FetchMethodFromInterface(fromService, "SearchPlaylistWithID")
	if !ok {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not fetch method from interface\n")
		return nil, blueprint.ErrUnknown
	}

	var methodSearchPlaylistWithTracks, ok2 = util.FetchMethodFromInterface(toService, "SearchPlaylistWithTracks")
	if !ok2 {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error - could not fetch method from interface\n")
		return nil, blueprint.ErrUnknown
	}
	// this is the result of searching for a playlist on a platform, using the resource id (spotify playlist for example)
	var idSearchResult *blueprint.PlaylistSearchResult
	// the omitted tracks when searching for a new track. these are track info from the "source" platform (fromService)
	var omittedTracks *[]blueprint.OmittedTracks
	// the real result of searching for the playlist tracks on another platform.
	var tracksSearchResult *[]blueprint.TrackSearchResult

	if methodSearchPlaylistWithID.IsValid() {
		params := make([]reflect.Value, 1)
		params[0] = reflect.ValueOf(info.EntityID)
		playlistSearchWithIDResults := methodSearchPlaylistWithID.Call(params)
		if len(playlistSearchWithIDResults) > 0 {
			if playlistSearchWithIDResults[0].Interface() == nil {
				return nil, blueprint.EnoResult
			}
			// for playlist results, the second result returned from method call is a pointer to the playlist search result from source platform
			if playlistSearchWithIDResults[0].Interface() != nil {
				idSearchResult = playlistSearchWithIDResults[0].Interface().(*blueprint.PlaylistSearchResult)
			}
			// then use the above playlist info to search for srcPlatformTracks, on target platform
			if methodSearchPlaylistWithTracks.IsValid() {
				searchParams := make([]reflect.Value, 1)
				searchParams[0] = reflect.ValueOf(idSearchResult)

				conversion.Meta.URL = idSearchResult.URL
				conversion.Meta.Title = idSearchResult.Title
				conversion.Meta.Length = idSearchResult.Length
				conversion.Meta.Owner = idSearchResult.Owner
				conversion.Meta.Cover = idSearchResult.Cover

				svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
				_, whErr := svixInstance.SendEvent(app.WebhookAppID, blueprint.PlaylistConversionMetadataEvent, &blueprint.PlaylistConversionEventMetadata{
					Platform: info.Platform,
					Meta:     &conversion.Meta,
				})

				if whErr != nil {
					log.Printf("\n[controllers][platforms][ConvertPlaylist] error - could not send webhook event: %v\n", whErr)
				}

				playlistSearchWithTracksResults := methodSearchPlaylistWithTracks.Call(searchParams)
				if len(playlistSearchWithTracksResults) > 0 {
					if playlistSearchWithTracksResults[0].Interface() == nil {
						return nil, blueprint.EnoResult
					}
					// the first result returned from the method call is a pointer to an array of track search results from target platform
					tracksSearchResult = playlistSearchWithTracksResults[0].Interface().(*[]blueprint.TrackSearchResult)
					// the second result returned from the method call is a pointer to the omitted srcPlatformTracks from the playlist
					if playlistSearchWithTracksResults[1].Interface() != nil {
						omittedTracks = playlistSearchWithTracksResults[1].Interface().(*[]blueprint.OmittedTracks)
					}
				}
			}
		}
	}

	if idSearchResult == nil {
		return nil, blueprint.EnoResult
	}

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
