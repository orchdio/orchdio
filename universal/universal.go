// package universal contains the logic for converting entities between platforms
// It is where cross-platform conversions and logic are handled and called

package universal

import (
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	platforminternal "orchdio/internal/platform"
	serviceinternal "orchdio/internal/service"
	svixwebhook "orchdio/webhooks/svix"
	"os"

	"github.com/jmoiron/sqlx"

	"github.com/go-redis/redis/v8"
)

// ConvertTrack fetches all the tracks converted from all the supported platforms
func ConvertTrack(info *blueprint.LinkInfo, red *redis.Client, pg *sqlx.DB) (*blueprint.TrackConversion, error) {
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

	webhookSender := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	platformsServiceFactory := platforminternal.NewPlatformServiceFactory(pg, red, app, webhookSender)
	serviceFactory := serviceinternal.NewServiceFactory(platformsServiceFactory)

	convertedTrack, pErr := serviceFactory.ConvertTrack(info)
	if pErr != nil {
		log.Printf("[controllers][platforms][universal][ConvertTrack] error - could not convert track: %v\n", err)
		return nil, err
	}

	return convertedTrack, nil
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

	webhookSender := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	platformsServiceFactory := platforminternal.NewPlatformServiceFactory(pg, red, app, webhookSender)
	serviceFactory := serviceinternal.NewServiceFactory(platformsServiceFactory)

	xConversion, xErr := serviceFactory.AsynqConvertPlaylist(info)
	if xErr != nil {
		log.Printf("Error converting playlist here in universal")
		return nil, xErr
	}
	return xConversion, nil
}
