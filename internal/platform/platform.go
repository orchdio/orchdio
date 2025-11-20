package platform_internal

import (
	"encoding/json"
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
)

type PlatformService interface {
	SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error)
	SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error)
	FetchPlaylistMetaInfo(info *blueprint.LinkInfo) (*blueprint.PlaylistMetadata, error)
	FetchTracksForSourcePlatform(info *blueprint.LinkInfo, playlistMeta *blueprint.PlaylistMetadata, result chan blueprint.TrackSearchResult) error
	FetchLibraryAlbums(refreshToken string) ([]blueprint.LibraryAlbum, error)
	FetchListeningHistory(refreshToken string) ([]blueprint.TrackSearchResult, error)
	FetchUserArtists(refreshToken string) (*blueprint.UserLibraryArtists, error)
	FetchLibraryPlaylists(refreshToken string) ([]blueprint.UserPlaylist, error)
	FetchUserInfo(refreshToken string) (*blueprint.UserPlatformInfo, error)
}

type PlatformServiceFactory struct {
	Pg            *sqlx.DB
	Red           *redis.Client
	App           *blueprint.DeveloperApp
	WebhookSender svixwebhook.SvixInterface
}

func NewPlatformServiceFactory(pg *sqlx.DB, red *redis.Client, app *blueprint.DeveloperApp, webhookSender svixwebhook.SvixInterface) *PlatformServiceFactory {
	return &PlatformServiceFactory{pg, red, app, webhookSender}
}

func (pf *PlatformServiceFactory) GetPlatformService(platform string) (PlatformService, error) {
	credentials, err := pf.getCredentials(platform)
	if err != nil {
		log.Printf("%v\n", err)
		return nil, err
	}
	// webhookSender := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)

	switch platform {
	case spotify.IDENTIFIER:
		return spotify.NewService(credentials, pf.Pg, pf.Red, pf.App, pf.WebhookSender), nil

	case deezer.IDENTIFIER:
		return deezer.NewService(credentials, pf.Pg, pf.Red, pf.App, pf.WebhookSender), nil

	case applemusic.IDENTIFIER:
		return applemusic.NewService(credentials, pf.Pg, pf.Red, pf.App), nil

	case tidal.IDENTIFIER:
		return tidal.NewService(credentials, pf.Pg, pf.Red, pf.App, pf.WebhookSender), nil

	case ytmusic.IDENTIFIER:
		// note: ytmusic does not require credentials (yet)
		return ytmusic.NewService(pf.Red, pf.App), nil
	default:
		return nil, fmt.Errorf("platform service not found in platform service: %s", platform)
	}

}

func (pf *PlatformServiceFactory) GetPlatformServices(platforms []string) ([]PlatformService, error) {
	var platformServices []PlatformService
	for _, i := range platforms {
		service, gErr := pf.GetPlatformService(i)
		if gErr != nil {
			log.Print("Debug: internal — platform — getplatformservices: could not get platform service")
			log.Println(gErr)
			return nil, gErr
		}
		platformServices = append(platformServices, service)
	}
	return platformServices, nil
}

func (pf *PlatformServiceFactory) getCredentials(platform string) (*blueprint.IntegrationCredentials, error) {
	// fixme(HACK): if the platform is ytmusic, we dont want to decrypt because its nil — no credentials to decrypt

	if platform == ytmusic.IDENTIFIER {
		return nil, nil
	}
	var encryptedCredentials []byte
	// fixme(note): ytmusic does not require credentials yet so there is nothing to set
	switch platform {
	case spotify.IDENTIFIER:
		if pf.App.SpotifyCredentials == nil {
			return nil, fmt.Errorf("spotify credentials not initialized or does not exist")
		}
		encryptedCredentials = pf.App.SpotifyCredentials
	case tidal.IDENTIFIER:
		if pf.App.TidalCredentials == nil {
			return nil, fmt.Errorf("tidal credentials not initialized or does not exist")
		}
		encryptedCredentials = pf.App.TidalCredentials
	case deezer.IDENTIFIER:
		if pf.App.DeezerCredentials == nil {
			return nil, fmt.Errorf("deezer credentials not initialized or does not exist")
		}
		encryptedCredentials = pf.App.DeezerCredentials
	case applemusic.IDENTIFIER:
		if pf.App.AppleMusicCredentials == nil {
			return nil, fmt.Errorf("applemusic credentials not initialized or does not exist")
		}
		encryptedCredentials = pf.App.AppleMusicCredentials
	default:
		return nil, fmt.Errorf("unsupported platform %s", platform)
	}

	credentialBytes, err := util.Decrypt(encryptedCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	var credentials blueprint.IntegrationCredentials
	err = json.Unmarshal(credentialBytes, &credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}
	return &credentials, nil
}
