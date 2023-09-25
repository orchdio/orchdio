// package universal contains the logic for converting entities between platforms
// It is where cross-platform conversions and logic are handled and called

package universal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	logger2 "orchdio/logger"
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
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		Entity:  zap.String("entity", info.Entity),
		Message: zap.String("message", "converting entity").String,
		AppID:   zap.String("app_id", info.App).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)

	var conversion blueprint.Conversion
	conversion.Entity = "track"

	// fetch the app making the request
	database := db.NewDB{DB: pg}
	app, err := database.FetchAppByAppId(info.App)
	if err != nil {
		orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not fetch app", zap.Error(err))
		return nil, err
	}

	targetPlatform := info.TargetPlatform

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
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - no spotify credentials provided", zap.String("app_id", info.App))
			return nil, errors.New("spotify credentials not provided")
		}

		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt spotify credentials", zap.Error(decErr))
			return nil, decErr
		}

		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal spotify credentials", zap.Error(err))
			return nil, err
		}

		fromService = spotify.NewService(&credentials, pg, red)
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - tidal credentials not provided", zap.String("app_id", info.App))
			return nil, errors.New("tidal credentials not provided")
		}

		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt tidal credentials", zap.Error(decErr))
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal tidal credentials", zap.Error(err))
			return nil, err
		}

		if credentials.AppRefreshToken == "" {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - tidal refresh token not present in TIDAL credentials. Please update the app and try again.", zap.String("app_id", info.App))
			return nil, blueprint.EBADCREDENTIALS
		}

		fromService = tidal.NewService(&credentials, pg, red)
		//fromPlatformIntegrationCreds = credentials
	case deezer.IDENTIFIER:
		if len(app.DeezerCredentials) == 0 {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - deezer credentials not provided", zap.String("app_id", info.App))
			return nil, errors.New("deezer credentials not provided")
		}

		credBytes, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt deezer credentials", zap.Error(decErr))
			return nil, decErr
		}

		var credentials blueprint.IntegrationCredentials
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal deezer credentials", zap.Error(err))
			return nil, err
		}
		fromService = deezer.NewService(&credentials, pg, red)
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - apple music credentials not provided", zap.String("app_id", info.App))
			return nil, errors.New("apple music credentials not provided")
		}
		var credentials blueprint.IntegrationCredentials
		credBytes, decErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt apple music credentials", zap.Error(decErr), zap.String("app_id", info.App))
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal apple music credentials", zap.Error(err), zap.String("app_id", info.App))
			return nil, err
		}

		if credentials.AppRefreshToken == "" {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - apple music credentials not provided", zap.String("app_id", info.App))
			return nil, blueprint.EBADCREDENTIALS
		}

		fromService = applemusic.NewService(&credentials, pg, red)
	case ytmusic.IDENTIFIER:
		// we dont need credentials for ytmusic yet but we still need to initialize the service
		fromService = ytmusic.NewService(red)
	}

	// platform we're converting to. similar to above in functionality
	switch targetPlatform {
	case spotify.IDENTIFIER:
		if app.SpotifyCredentials == nil {
			orchdioLogger.Warn("[controllers][platforms][universal][ConvertTrack] warning - no spotify credentials provided", zap.String("app_id", info.App))
			return nil, blueprint.ECREDENTIALSMISSING
		}

		var credentials blueprint.IntegrationCredentials
		err = json.Unmarshal(app.SpotifyCredentials, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal spotify credentials", zap.Error(err), zap.String("app_id", info.App))
			return nil, err
		}

		s := spotify.NewService(&credentials, pg, red)
		toService = append(toService, map[string]interface{}{spotify.IDENTIFIER: s})
	case tidal.IDENTIFIER:
		if len(app.TidalCredentials) == 0 {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] warning - no tidal credentials provided", zap.String("app_id", info.App),
				zap.String("entity_target_type", "to_service"))
			return nil, errors.New("tidal credentials not provided")
		}

		var credentials blueprint.IntegrationCredentials
		credBytes, dErr := util.Decrypt(app.TidalCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if dErr != nil {
			log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not decrypt tidal credentials: %v\n", dErr)
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt tidal credentials", zap.Error(dErr),
				zap.String("app_id", info.App))
			return nil, dErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal tidal credentials", zap.Error(err),
				zap.String("entity_target_type", "to_service"))
			return nil, err
		}

		if credentials.AppRefreshToken == "" {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - tidal credentials not provided", zap.String("app_id", info.App),
				zap.String("entity_target_type", "to_service"))
			return nil, blueprint.EBADCREDENTIALS
		}

		s := tidal.NewService(&credentials, pg, red)
		toService = append(toService, map[string]interface{}{tidal.IDENTIFIER: s})
	case deezer.IDENTIFIER:
		var credentials blueprint.IntegrationCredentials
		if len(app.DeezerCredentials) == 0 {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - no deezer credentials provided", zap.String("app_id", info.App))
			return nil, errors.New("deezer credentials not provided")
		}
		credBytes, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt deezer credentials", zap.Error(decErr),
				zap.String("entity_target_type", "to_service"))
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal deezer credentials", zap.Error(err),
				zap.String("entity_target_type", "to_service"))
			return nil, err
		}
		s := deezer.NewService(&credentials, pg, red)
		toService = append(toService, map[string]interface{}{deezer.IDENTIFIER: s})
	case applemusic.IDENTIFIER:
		if len(app.AppleMusicCredentials) == 0 {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - no apple music credentials provided", zap.String("entity_target_type", "to_service"),
				zap.String("app_id", info.App))
			return nil, errors.New("apple music credentials not provided")
		}
		var credentials blueprint.IntegrationCredentials

		credBytes, decErr := util.Decrypt(app.AppleMusicCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt apple music credentials", zap.Error(decErr),
				zap.String("entity_target_type", "to_service"))
			return nil, decErr
		}
		err = json.Unmarshal(credBytes, &credentials)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not unmarshal apple music credentials", zap.Error(err),
				zap.String("entity_target_type", "to_service"))
			return nil, err
		}
		if credentials.AppRefreshToken == "" {
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - apple music credentials not provided", zap.String("entity_target_type", "to_service"))
			return nil, blueprint.EBADCREDENTIALS
		}

		s := applemusic.NewService(&credentials, pg, red)
		toService = append(toService, map[string]interface{}{applemusic.IDENTIFIER: s})
	case ytmusic.IDENTIFIER:
		s := ytmusic.NewService(red)
		toService = append(toService, map[string]interface{}{ytmusic.IDENTIFIER: s})
	}

	if targetPlatform == "all" {
		orchdioLogger.Warn("[controllers][platforms][universal][ConvertTrack] warning - target platform is all", zap.String("app_id", info.App),
			zap.Any("entity_info", info))
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
				if plat == ytmusic.IDENTIFIER {
					toService = append(toService, map[string]interface{}{ytmusic.IDENTIFIER: ytmusic.NewService(red)})
				}
				continue
			}
			decryptedCredentials, dErr := util.DecryptIntegrationCredentials(encryptedCred)
			if dErr != nil {
				orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack] error - could not decrypt credentials for platform", zap.Error(dErr), zap.String("platform", plat))
				return nil, dErr
			}
			switch plat {
			case ytmusic.IDENTIFIER:
				s := ytmusic.NewService(red)
				toService = append(toService, map[string]interface{}{ytmusic.IDENTIFIER: s})

			case spotify.IDENTIFIER:
				s := spotify.NewService(decryptedCredentials, pg, red)
				toService = append(toService, map[string]interface{}{spotify.IDENTIFIER: s})
			case tidal.IDENTIFIER:
				s := tidal.NewService(decryptedCredentials, pg, red)
				toService = append(toService, map[string]interface{}{tidal.IDENTIFIER: s})
			case deezer.IDENTIFIER:
				s := deezer.NewService(decryptedCredentials, pg, red)
				toService = append(toService, map[string]interface{}{deezer.IDENTIFIER: s})
			case applemusic.IDENTIFIER:
				s := applemusic.NewService(decryptedCredentials, pg, red)
				toService = append(toService, map[string]interface{}{applemusic.IDENTIFIER: s})
			}
		}
	}

	var methodSearchTrackWithID, ok = util.FetchMethodFromInterface(fromService, "SearchTrackWithID")
	if !ok {
		orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack][reflect-meta] error - could not fetch method from interface",
			zap.String("method", "SearchTrackWithID"))
		return nil, blueprint.EUNKNOWN
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
			orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack][reflect-meta] error - could not convert interface to TrackSearchResult.. Error dynamically calling fromMethod.",
				zap.String("method", "SearchTrackWithID"))
			return nil, blueprint.EUNKNOWN
		}
		fromResult = res

		// fetch the conversion methods for the target platforms.
		for idx, tS := range toService {
			if res == nil {
				continue
			}
			cp := tS
			for k, v := range cp {
				plat := k
				shadowService := v
				var methodSearchTrackWithTitle, ok2 = util.FetchMethodFromInterface(shadowService, "SearchTrackWithTitle")
				if !ok2 {
					return nil, blueprint.EUNKNOWN
				}

				// todo: implement nil check
				if res == nil && idx == 0 {
					orchdioLogger.Warn("[controllers][platforms][universal][ConvertTrack] warning - search result is nil",
						zap.String("platform", plat))
					return nil, blueprint.EUNKNOWN
				}

				if methodSearchTrackWithTitle.IsValid() {
					ins2 := make([]reflect.Value, 2)
					ins2[0] = reflect.ValueOf(res.Title)
					ins2[1] = reflect.ValueOf(res.Artists[0])
					ans2 := methodSearchTrackWithTitle.Call([]reflect.Value{ins2[0], ins2[1]})
					res2, ok3 := ans2[0].Interface().(*blueprint.TrackSearchResult)
					if !ok3 {
						log.Printf("\n[controllers][platforms][universal][ConvertTrack] error - could not convert interface to TrackSearchResult.. Error dynamically calling toMethod.\n")
						orchdioLogger.Error("[controllers][platforms][universal][ConvertTrack][reflect-meta] error - could not convert interface to TrackSearchResult.. Error dynamically calling toMethod.",
							zap.String("method", "SearchTrackWithTitle"))
						return nil, blueprint.EUNKNOWN
					}
					toResult = append(toResult, map[string]*blueprint.TrackSearchResult{plat: res2})
				} else {
					return nil, blueprint.EUNKNOWN
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

	for _, tR := range toResult {
		// copy target result into a temp cp variable to avoid concurrency issues
		cp := tR
		for k, v := range cp {
			plat := k
			shadowResult := v
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
	orchdioLogger.Info("[controllers][platforms][deezer][ConvertEntity] info - conversion done", zap.String("entity", info.Entity),
		zap.String("entity_id", info.EntityID), zap.String("app_id", info.App))
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

	// this block gets the credentials for the platform we're converting from.
	// important so that we can get the credentials for those available for the app
	// and then use to create the instance of the platform for the supported platforms available.
	// This is for the `fromService` interface
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

	// todo: refactor this to use the same decrypt credentials
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
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] Playlist conversion stuff here ")
		spew.Dump(outs)
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
