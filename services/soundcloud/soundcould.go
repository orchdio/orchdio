package soundcloud

import (
	"orchdio/blueprint"
	"orchdio/constants"
	svixwebhook "orchdio/webhooks/svix"

	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
)

const AuthBase = "https://api.soundcloud.com/oauth2"

type Service struct {
	IntegrationAppID     string
	IntegrationAppSecret string
	RedisClient          *redis.Client
	PgClient             *sqlx.DB
	App                  *blueprint.DeveloperApp
	WebhookSender        svixwebhook.SvixInterface
	Identifier           string
}

func NewService(credentials *blueprint.IntegrationCredentials, pgClient *sqlx.DB, redisClient *redis.Client, devApp *blueprint.DeveloperApp, webhookSender svixwebhook.SvixInterface) *Service {
	return &Service{
		IntegrationAppID:     credentials.AppID,
		IntegrationAppSecret: credentials.AppSecret,
		RedisClient:          redisClient,
		PgClient:             pgClient,
		App:                  devApp,
		WebhookSender:        webhookSender,
		Identifier:           constants.SoundCloudIdentifier,
	}
}

func (sc *Service) FetchLibraryAlbums(refreshToken string) ([]blueprint.LibraryAlbum, error) {
	return nil, nil
}

func (sc *Service) FetchLibraryPlaylists(refreshToken string) ([]blueprint.UserPlaylist, error) {
	return nil, nil
}

func (sc *Service) FetchListeningHistory(refreshToken string) ([]blueprint.TrackSearchResult, error) {
	return nil, nil
}

func (sc *Service) FetchPlaylistMetaInfo(info *blueprint.LinkInfo) (*blueprint.PlaylistMetadata, error) {
	return nil, nil
}

func (sc *Service) FetchTracksForSourcePlatform(info *blueprint.LinkInfo, playlistMeta *blueprint.PlaylistMetadata, result chan blueprint.TrackSearchResult) error {
	return nil
}

func (sc *Service) FetchUserArtists(refreshToken string) (*blueprint.UserLibraryArtists, error) {
	return nil, nil
}

func (sc *Service) FetchUserInfo(authInfo blueprint.UserAuthInfoForRequests) (*blueprint.UserPlatformInfo, error) {
	return nil, nil
}

func (sc *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	return nil, nil
}

func (sc *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData, requestAuthInfo blueprint.UserAuthInfoForRequests) (*blueprint.TrackSearchResult, error) {
	return nil, nil
}

// func (s *Soundcloud) FetchAuthURL() string {

// 	return ""
// }

//func (s *Soundcloud) SearchTrackWithID(id string) {
//	var soundcloudAccessToken = os.Getenv("SOUNDCLOUD_ACCESS_TOKEN")
//
//}
//
//func fetchNewAccessToken() (string, error) {
//	refreshInstance := axios.NewInstance(&axios.InstanceConfig{
//		BaseURL: AuthBase,
//		Headers: map[string][]string{
//			"Content-Type": {"application/x-www-form-urlencoded"},
//		},
//	})
//	grantType := "refresh_token"
//	params := url.Values{}
//	params.Add("client_id", os.Getenv("SOUNDCLOUD_CLIENT_ID"))
//	params.Add("client_secret", os.Getenv("SOUNDCLOUD_CLIENT_SECRET"))
//	params.Add("refresh_token", os.Getenv("SOUNDCLOUD_REFRESH_TOKEN"))
//	return "", nil
//}
