package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/constants"
	"orchdio/db"
	"orchdio/services/soundcloud/soundcloud_sdk"
	soundcloud_auth "orchdio/services/soundcloud/soundcloud_sdk/auth"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
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

func (sc *Service) FetchPlaylistMetaInfo(info *blueprint.LinkInfo, authToken *blueprint.AuthToken) (*blueprint.PlaylistMetadata, error) {
	return nil, nil
}

func (sc *Service) FetchTracksForSourcePlatform(info *blueprint.LinkInfo, authToken *blueprint.AuthToken, playlistMeta *blueprint.PlaylistMetadata, result chan blueprint.TrackSearchResult) error {
	return nil
}

func (sc *Service) FetchUserArtists(refreshToken string) (*blueprint.UserLibraryArtists, error) {
	return nil, nil
}

func (sc *Service) FetchUserInfo(authInfo blueprint.UserAuthInfoForRequests) (*blueprint.UserPlatformInfo, error) {
	return nil, nil
}

func (sc *Service) SearchTrackWithID(info *blueprint.LinkInfo, authInfo *blueprint.AuthToken) (*blueprint.TrackSearchResult, error) {
	return nil, nil
}

func (sc *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData, requestAuthInfo blueprint.UserAuthInfoForRequests) (*blueprint.TrackSearchResult, error) {

	// key format:
	// soundcloud:track:<track_title>:artists- <artists,here>
	cacheKey := util.FormatTargetPlaylistTrackByCacheKeyTitle(constants.SoundCloudIdentifier, strings.Join(searchData.Artists, ","), searchData.Title)
	log.Printf("Soundcloud: search for track with the key %s", cacheKey)

	ctx := context.TODO()
	if sc.RedisClient.Exists(ctx, cacheKey).Val() == 1 {
		log.Println("Found track in cache...")
		var result *blueprint.TrackSearchResult
		cacheResult, err := sc.RedisClient.Get(ctx, cacheKey).Result()
		if err != nil {
			log.Print("Soundcloud: could not fetch cached track")
			return nil, err
		}

		err = json.Unmarshal([]byte(cacheResult), &result)
		if err != nil {
			log.Printf("Could not deserialize cache result to struct: %v", err)
			return nil, err
		}

		return result, nil
	}

	scTrack, err := sc.fetchSingleTrack(searchData)
	if err != nil {
		log.Printf("Could not fetch single track with the search data passed: %v", searchData)
		return nil, err
	}

	// log.Println("Single result from fetchTrack is")
	// spew.Dump(scTrack)

	parseReleasedAt, err := time.Parse("2006/01/02 15:04:05 -0700", scTrack.CreatedAt)
	if err != nil {
		log.Printf("Could not format track createdAt. Original datestamp: %v", scTrack.CreatedAt)
		return nil, err
	}

	formattedReleasedAt := parseReleasedAt.Format("2006-01-02")
	durationMilli := scTrack.Duration

	var artists []string

	if scTrack.MetadataArtist != nil {
		artists = strings.Split(*scTrack.MetadataArtist, ",")
	} else {
		artists = searchData.Artists
	}

	result := &blueprint.TrackSearchResult{
		URL:           scTrack.URI,
		Artists:       artists,
		Released:      formattedReleasedAt,
		Duration:      util.GetFormattedDuration(durationMilli / 1000),
		DurationMilli: durationMilli,
		// soundcloud doesnt return explicit content value in the API
		Explicit: false,
		Title:    scTrack.Title,
		Preview:  "",
		Album:    "",
		ID:       strconv.FormatInt(scTrack.ID, 10),
		Cover:    scTrack.ArtworkURL,
	}
	return result, nil
}

func (s *Service) Tokens(authInfo blueprint.UserAuthInfoForRequests) (*soundcloud_auth.Authenticator, *oauth2.Token, error) {
	// auth := spotifyauth.New(
	// 	spotifyauth.WithClientID(s.IntegrationAppID),
	// 	spotifyauth.WithClientSecret(s.IntegrationAppSecret),
	// 	spotifyauth.WithRedirectURL(s.App.RedirectURL),
	// )
	auth, err := soundcloud_auth.NewSoundCloudAuthClient(s.IntegrationAppID, s.IntegrationAppSecret, s.App.RedirectURL)
	if err != nil {
		log.Println("Could not create new instance of soundcloud auth client in the tidal user space")
		return nil, nil, err
	}

	hasExpired, err := util.HasTokenExpired(authInfo.Token.ExpiresIn)
	if err != nil {
		return nil, nil, err
	}

	var tokens *oauth2.Token

	if hasExpired {
		log.Println("User soundcloud token has expired.. going to refresh and get new tokens")
		// refresh the token
		refreshedTokens, err := auth.RefreshToken(context.TODO(), &oauth2.Token{
			RefreshToken: authInfo.Token.RefreshToken,
		})
		if err != nil {
			return nil, nil, err
		}
		tokens = refreshedTokens
		dbN := db.NewDB{DB: s.PgClient}
		expiresIn := time.Now().Add(time.Hour).Format(time.RFC3339)

		encryptedRefreshToken, err := util.Encrypt([]byte(tokens.RefreshToken), []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			return nil, nil, err
		}

		updateErr := dbN.UpdateUserAppAuthTokens(authInfo.User, tokens.AccessToken, expiresIn, constants.SoundCloudIdentifier, encryptedRefreshToken)
		if updateErr != nil {
			return nil, nil, err
		}
	} else {
		tokens = &oauth2.Token{
			AccessToken:  authInfo.Token.AccessToken,
			RefreshToken: authInfo.Token.RefreshToken,
		}
	}

	return auth, tokens, nil
}

func (sc *Service) fetchSingleTrack(searchData *blueprint.TrackSearchData) (*soundcloud_sdk.Track, error) {

	config := &clientcredentials.Config{
		ClientID:     sc.IntegrationAppID,
		ClientSecret: sc.IntegrationAppSecret,
		TokenURL:     soundcloud_auth.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		log.Println("Soundcloud auth error: could not fetch client credentials token")
		return nil, err
	}

	log.Println("==================================================================")
	log.Println("Generated client credentials token for soundcloud is", token)
	log.Println("==================================================================")
	ctx := context.TODO()

	scAuth, scErr := soundcloud_auth.NewSoundCloudAuthClient(sc.IntegrationAppID, sc.IntegrationAppSecret, sc.App.RedirectURL)
	if scErr != nil {
		log.Println("Could not create new soundcloud client credentials flow auth")
		return nil, scErr
	}

	httpClient := scAuth.Client(ctx, token)
	client := soundcloud_sdk.NewSoundcloudClient(httpClient)
	query := fmt.Sprintf("%s %s", searchData.Title, strings.Join(searchData.Artists, " "))
	// todo: try searching by genre, to test implement passing request options
	results, sErr := client.Search(ctx, query)
	if sErr != nil {
		log.Printf("Could not fetch search result for query: %s", query)
		log.Println(sErr)
		return nil, sErr
	}

	var possibleTrackId string
	// iterate over results and the build final result based on the most likely match from all results
	for _, result := range *results {
		if result.MetadataArtist != nil {
			metadataArtist := *result.MetadataArtist
			if lo.Contains(searchData.Artists, metadataArtist) {
				possibleTrackId = result.URN
				break
			}
		} else {
			// fallback to taking the first result
			possibleTrackId = (*results)[0].URN
			break
		}
	}

	// then fetch the single track with the possible matched id
	singleTrack, err := client.GetTrack(ctx, possibleTrackId)
	if err != nil {
		log.Printf("Could not fetch single track with ID %s", possibleTrackId)
		return nil, err
	}

	return singleTrack, nil
}
