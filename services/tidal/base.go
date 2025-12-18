package tidal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"orchdio/blueprint"
	"orchdio/constants"
	"orchdio/services/tidal/tidal_v2"
	tidal_auth "orchdio/services/tidal/tidal_v2/auth"
	"orchdio/util"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jmoiron/sqlx"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/go-redis/redis/v8"
	"github.com/nleeper/goment"
	"github.com/vicanso/go-axios"
)

const ApiUrl = "https://listen.tidal.com/v1"
const AuthBase = "https://auth.tidal.com/v1/oauth2"

type Service struct {
	DB                     *sqlx.DB
	Redis                  *redis.Client
	IntegrationCredentials *blueprint.IntegrationCredentials
	Base                   string
	App                    *blueprint.DeveloperApp
	WebhookSender          WebhookSender
	Identifier             string
}

type WebhookSender interface {
	SendTrackEvent(appID string, event *blueprint.PlaylistConversionEventTrack) bool
}

func NewService(credentials *blueprint.IntegrationCredentials, DB *sqlx.DB, red *redis.Client, devApp *blueprint.DeveloperApp, webhookSender WebhookSender) *Service {
	return &Service{
		DB:                     DB,
		Redis:                  red,
		IntegrationCredentials: credentials,
		Base:                   ApiUrl,
		App:                    devApp,
		WebhookSender:          webhookSender,
		Identifier:             constants.TidalIdentifier,
	}
}

// SearchTrackWithID searches for a track on tidal using the tidal ID
func (s *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	cacheKey := "tidal:track:" + info.EntityID
	log.Println("\n[services][tidal][SearchWithID] - cacheKey - ", cacheKey)
	cachedTrack, err := s.Redis.Get(context.Background(), cacheKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("\n[services][tidal][SearchWithID] - error - Could not fetch record from the cache. This is an unexpected error %v\n", err)
		return nil, err
	}

	if err != nil && errors.Is(err, redis.Nil) {
		log.Printf("\n[services][tidal][SearchWithID] - this track has not been cached before %v\n", err)

		tracks, rErr := s.FetchTrackWithID(info.EntityID)

		if rErr != nil {
			if errors.Is(rErr, blueprint.ErrBadRequest) {
				log.Printf("\n[services][tidal][SearchWithID] - Error fetching track conversion from TIDAL %v\n", rErr)
			}
			if errors.Is(rErr, blueprint.ErrBadCredentials) {
				log.Printf("\n[services][tidal][SearchWithID] - Error fetching track conversion from TIDAL %v\n", rErr)
			}
			return nil, rErr
		}

		var artistes []string
		for _, artist := range tracks.Artists {
			artistes = append(artistes, artist.Name)
		}

		searchResult := blueprint.TrackSearchResult{
			URL:           tracks.URL,
			Artists:       artistes,
			Released:      tracks.StreamStartDate,
			Duration:      util.GetFormattedDuration(tracks.Duration),
			DurationMilli: tracks.Duration * 1000,
			Explicit:      tracks.Explicit,
			Title:         tracks.Title,
			Preview:       "",
			Album:         tracks.Album.Title,
			ID:            strconv.Itoa(tracks.Album.ID),
			Cover:         util.BuildTidalAssetURL(tracks.Album.Cover),
		}
		serialized, sErr := json.Marshal(searchResult)
		if sErr != nil {
			log.Printf("\n[services][tidal][SearchWithID] - could not serialize track result - %v\n", sErr)
			return nil, sErr
		}

		err = s.Redis.Set(context.Background(), cacheKey, serialized, time.Hour*24).Err()
		if err != nil {
			log.Printf("\n[services][tidal][SearchWithID] - could not cache track - %v\n", err)
		} else {
			log.Printf("\n[services][tidal][SearchWithID] - track cached successfully\n")
		}
		return &searchResult, nil
	}

	var deserialized blueprint.TrackSearchResult
	err = json.Unmarshal([]byte(cachedTrack), &deserialized)
	if err != nil {
		log.Printf("\n[services][tidal][SearchWithID] - error - %v\n", err)
		return nil, err
	}
	return &deserialized, nil
}

// FetchTrackWithID fetches a track from tidal
func (s *Service) FetchTrackWithID(id string) (*Track, error) {
	// TODO: implement refresh token fetching the access token (if expired)
	// TODO: find a way to add access token securely since i need to store somewhere (tidal auth api limitation)
	// TODO: update the access token (probably store in redis)
	accessToken, err := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][SearchTrackWithID] - error - could not fetch new TIDAL access token %v\n", err)
		return nil, err
	}

	log.Printf("\n[controllers][platforms][tidal][SearchTrackWithID] - access token - %v\n", accessToken)
	if accessToken == "" {
		log.Printf("\n[controllers][platforms][tidal][SearchTrackWithID] - error - could not fetch new TIDAL access token %v\n", err)
		return nil, blueprint.ErrBadCredentials
	}
	// first, fetch the access token hard coded in the config
	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	})
	// make a request to the tidal API
	response, err := instance.Get(fmt.Sprintf("/tracks/%s?countryCode=US", id))
	if err != nil {
		return nil, err
	}

	if response.Status >= 400 {
		log.Printf("\n[controllers][platforms][tidal][SearchTrackWithID] - TIDAL request failed with status: %v\n", response.Status)
		return nil, blueprint.ErrBadRequest
	}

	singleTrack := &Track{}
	err = json.Unmarshal(response.Data, singleTrack)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][SearchTrackWithID] - error - %v\n", err)
		return nil, blueprint.ErrBadCredentials
	}
	return singleTrack, nil
}

// SearchTrackWithTitle will perform a search on tidal for the track we want
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData, requestAuthInfo blueprint.UserAuthInfoForRequests) (*blueprint.TrackSearchResult, error) {
	cleanedArtiste := strings.ToLower(fmt.Sprintf("tidal-%s-%s", util.NormalizeString(searchData.Artists[0]), searchData.Title))
	cacheKey := util.FormatTargetPlaylistTrackByCacheKeyTitle(IDENTIFIER, cleanedArtiste, searchData.Title)

	if s.Redis.Exists(context.Background(), cacheKey).Val() == 1 {
		cachedTrack, err := s.Redis.Get(context.Background(), cacheKey).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}

		var deserialized blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(cachedTrack), &deserialized)
		if err != nil {
			log.Printf("[services][platforms][tidal][SearchTrackWithID] - could not deserialiaze cached track - %v\n", err)
			return nil, err
		}

		return &deserialized, nil
	}

	result, err := s.FetchSingleTrackByTitle(*searchData, requestAuthInfo)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][SearchTrackWithTitle] - could not search track with title '%s' on tidal - %v\n", searchData.Title, err)
		return nil, err
	}

	return result, nil
}

// FetchSingleTrackByTitle fetches a track from tidal by title and artist
func (s *Service) FetchSingleTrackByTitle(searchData blueprint.TrackSearchData, authInfo blueprint.UserAuthInfoForRequests) (*blueprint.TrackSearchResult, error) {
	log.Printf("[controllers][platforms][tidal][FetchSingleTrackByTitle] - searching single track by title: %s %s\n", searchData.Title, strings.Join(searchData.Artists, ","))
	ctx := context.TODO()

	config := &clientcredentials.Config{
		ClientID:     s.IntegrationCredentials.AppID,
		ClientSecret: s.IntegrationCredentials.AppSecret,
		TokenURL:     tidal_auth.TokenURL,
	}
	token, err := config.Token(context.Background())
	if err != nil {
		log.Println("Could not fetch token URL for client credentials....")
		return nil, err
	}

	auth, err := tidal_auth.NewTidalAuthClient(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.App.RedirectURL)
	if err != nil {
		log.Println("Could not create a new instance of the TIDAL auth client", err)
		return nil, err
	}

	authClient := auth.Client(ctx, token)
	client := tidal_v2.NewTidalClient(authClient)

	// first, try the search suggestion...
	searchSuggestion, err := client.SearchSuggestions(ctx, fmt.Sprintf("%s %s", searchData.Title, strings.Join(searchData.Artists, " ")),
		"US",
		tidal_v2.IncludeInSearchSuggestion(tidal_v2.SearchIncludeDirectHits),
	)

	if err != nil {
		log.Println("Could not fetch the search suggestion...", err)
		return nil, err
	}

	var possibleMatchTrackId string
	// inside the searchSuggestion, we want to get the ID of the first suggested track result id

	for _, suggestion := range searchSuggestion.Data.Relationships.DirectHits.Data {
		if suggestion.Type == "tracks" {
			log.Println("Taking the first track result in the search suggestion", suggestion.ID)
			possibleMatchTrackId = suggestion.ID
			break
		}
	}
	// then get the track with that id
	singleTrack, err := client.GetTrack(ctx,
		possibleMatchTrackId, "US",
		tidal_v2.IncludeInTrack(
			tidal_v2.TrackIncludeAlbum,
			tidal_v2.TrackIncludeArtists,
		))

	if err != nil {
		log.Println("Could not fetching single track from TIDAL", err)
		return nil, err
	}

	var artists []string
	var album string
	for _, single := range singleTrack.Included {
		if single.Type == "artists" {
			artists = append(artists, single.Attributes.Name)
		}

		if single.Type == "albums" {
			album = single.Attributes.Title
		}
	}

	duration, err := tidal_v2.ParseISO8601Duration(singleTrack.Data.Attributes.Duration)
	if err != nil {
		log.Println("Could not parse ISO8601 Duration from TIDAL response")
	}

	durationMilli := int(duration.Milliseconds())

	// get album coverArt. we're calling album again because weirdly unfortunately enough
	// the TIDAL API does not return coverArt for single tracks.
	// todo: improve the performance of this. maybe making the tracks and albums request concurrent.
	trackAlbum, err := client.GetAlbum(
		ctx, singleTrack.Data.Relationships.Albums.Data[0].ID,
		tidal_v2.CountryCode("US"),
		tidal_v2.IncludeInAlbum(tidal_v2.AlbumIncludeCoverArt),
	)
	if err != nil {
		log.Println("Could not fetch track's album")
	}

	coverArt := trackAlbum.Included[0].Attributes.Files[0].Href
	trackResult := &blueprint.TrackSearchResult{
		URL:           singleTrack.Data.Attributes.ExternalLinks[0].Href,
		Artists:       artists,
		Released:      singleTrack.Data.Attributes.CreatedAt.Format(time.RFC3339),
		Duration:      util.GetFormattedDuration(int(duration.Milliseconds())),
		DurationMilli: durationMilli,
		Explicit:      singleTrack.Data.Attributes.Explicit,
		Title:         singleTrack.Data.Attributes.Title,
		Preview:       "",
		Album:         album,
		ID:            singleTrack.Data.ID,
		Cover:         coverArt,
	}

	return trackResult, nil
}

// fetchPlaylistInfo returns a playlist info. An internal method called in FetchPlaylistMetaInfo.
func (s *Service) fetchPlaylistInfo(id string) (*PlaylistInfo, error) {
	accessToken, err := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistInfo] - could not fetch auth token - %v\n", err)
		return nil, err
	}
	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	},
	)
	response, err := instance.Get(fmt.Sprintf("/playlists/%s?countryCode=US", id))
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistInfo] - could not fetch the playlist info for %s - %v\n", err, id)
		return nil, err
	}
	playlistInfo := &PlaylistInfo{}
	err = json.Unmarshal(response.Data, playlistInfo)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistInfo] - could not deserialize playlist info - %v\n", err)
		return nil, err
	}
	return playlistInfo, nil
}

// FetchTracksForSourcePlatform fetches the tracks from the source platform and sends each result to the channel as they come in.
func (s *Service) FetchTracksForSourcePlatform(info *blueprint.LinkInfo, playlistMeta *blueprint.PlaylistMetadata, resultChan chan blueprint.TrackSearchResult) error {
	identifierHash := fmt.Sprintf("tidal:playlist:%s", info)
	infoHash := fmt.Sprintf("tidal:snapshot:%s", info)

	if s.Redis.Exists(context.Background(), identifierHash).Val() == 1 {
		log.Println("Could not find tidal track from cache")
		// fetch the playlist playlistInfo from redis
		cachedInfo, gErr := s.Redis.Get(context.Background(), infoHash).Result()
		if gErr != nil && !errors.Is(gErr, redis.Nil) {
			log.Printf("\n[controllers][platforms][tidal][SearchPlaylistWithID] - could not fetch cached playlist playlistInfo - %v\n", gErr)
			return gErr
		}

		// deserialize the playlist playlistInfo
		var cachedLastPlayedAt string
		_ = json.Unmarshal([]byte(cachedInfo), &cachedLastPlayedAt)

		// format the timestamps on both of the playlist playlistInfo
		lastUpdated, gmErr := goment.New(cachedLastPlayedAt)
		infoLastUpdated, gmErr2 := goment.New(playlistMeta.LastUpdated)

		if gmErr != nil || gmErr2 != nil {
			log.Printf("\n[controllers][platforms][tidal][SearchPlaylistWithID] - could not parse last updated time - %v - %v\n", gmErr, gmErr2)
			return gmErr2
		}

		var result *blueprint.PlaylistSearchResult
		// fetch the cached tracks from redis.
		cachedResult, sErr := s.Redis.Get(context.Background(), identifierHash).Result()
		if sErr != nil {
			log.Printf("\n[services][tidal][FetchPlaylistTracksInfo] - ⚠️ error fetching key from redis. - %v\n", sErr)
			return sErr
		}
		// deserialize the tracks we fetched from redis
		jErr := json.Unmarshal([]byte(cachedResult), &result)
		if jErr != nil {
			log.Printf("\n[services][tidal][FetchPlaylistTracksInfo] - ⚠️ error deserializimng cache result - %v\n", jErr)
			return jErr
		}

		// if the timestamps are the same, that means that our playlist has not
		// changed, so we can return the cached result. in the other case, we
		// are doing nothing so we go on to fetch the tracks from the tidal api.
		if lastUpdated.IsSame(infoLastUpdated) {
			// send webhook event track conversion for each (cached) track here
			// fixme: send these async
			for i := range result.Tracks {
				track := &result.Tracks[i]
				resultChan <- *track
			}
			return nil
		}
	}

	// playlist has not been cached... here we do fresh tracklist fetching & processing...
	accessToken, sErr := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if sErr != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - error - %v\n", sErr)
		return sErr
	}

	playlistResult := &PlaylistTracks{}
	var pages = playlistMeta.NBTracks / 100
	if pages == 0 {
		pages = 1
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	})

	// implement pagination fetching
	for page := 0; page <= pages; page++ {
		response, err := instance.Get(fmt.Sprintf("/playlists/%s/items?offset=%d&limit=100&countryCode=US", info, page*100))
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - error - %v\n", err)
			return err
		}

		log.Println("Tried to get something here")

		res := &PlaylistTracks{}
		err = json.Unmarshal(response.Data, res)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not deserialize playlist result from tidal - %v\n", err)
			return err
		}

		log.Println("Body response from tidal")
		spew.Dump(string(response.Data))
		if len(res.Items) == 0 {
			break
		}

		log.Printf("The tidal pages are: %v", playlistResult)
		for _, item := range playlistResult.Items {
			var artistes []string
			for _, artist := range item.Item.Artists {
				artistes = append(artistes, artist.Name)
			}
			t := blueprint.TrackSearchResult{
				URL:           item.Item.Url,
				Artists:       artistes,
				Released:      item.Item.StreamStartDate,
				Duration:      util.GetFormattedDuration(item.Item.Duration),
				DurationMilli: item.Item.Duration * 1000,
				Explicit:      item.Item.Explicit,
				Title:         item.Item.Title,
				Preview:       "",
				Album:         item.Item.Album.Title,
				ID:            strconv.Itoa(item.Item.Id),
				Cover:         util.BuildTidalAssetURL(item.Item.Album.Cover),
			}

			resultChan <- t
		}
	}
	return nil
}

// FetchPlaylistMetaInfo returns a playlist metadata. It'll always return the latest playlist metadata infoa as we hit the tidal API.
func (s *Service) FetchPlaylistMetaInfo(info *blueprint.LinkInfo) (*blueprint.PlaylistMetadata, error) {
	_ = fmt.Sprintf("tidal:playlist:%s", info)
	log.Printf("Converting playlist with ID %s on TIDAL\n", info)

	// infoHash represents the key for the snapshot of the playlist playlistInfo, in this case
	// just a lasUpdated timestamp in string format.
	_ = fmt.Sprintf("tidal:snapshot:%s", info)

	playlistInfo, err := s.fetchPlaylistInfo(info.EntityID)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not fetch playlist playlistInfo - %v\n", err)
		return nil, err
	}

	artCover := fmt.Sprintf("https://resources.tidal.com/images/%s/1080x1080.jpg", strings.ReplaceAll(playlistInfo.SquareImage, "-", "/"))
	playlistMeta := &blueprint.PlaylistMetadata{
		// no length yet. its calculated by adding up all the track lengths in the playlist
		Length: util.GetFormattedDuration(playlistInfo.Duration),
		Title:  playlistInfo.Title,
		// no preview. might be an abstracted implementation in the future.
		Preview: "",

		// todo: fetch user's orchdio @ and/or id and enrich. might need a specific owner object. support fetching by tidal id for tidal implementation
		Owner: strconv.Itoa(playlistInfo.Creator.Id),
		// fixme: possible nil pointer
		Cover:  artCover,
		Entity: "playlist",
		URL:    playlistInfo.Url,
		// no short url here.
		// todo: try to pass the entity id here
		ShortURL:    info.TaskID,
		NBTracks:    playlistInfo.NumberOfTracks,
		Description: playlistInfo.Description,
		LastUpdated: playlistInfo.LastUpdated,
		ID:          playlistInfo.Uuid,
	}
	return playlistMeta, nil
}

func (s *Service) FetchNewAuthToken(appId, appSecret, appRefresh string) (string, error) {
	// now refresh token and get a new access token
	refreshInstance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: AuthBase,
		Headers: map[string][]string{
			"Content-Type": {"application/x-www-form-urlencoded"},
		},
	})
	// default scope. read and write user
	scope := "r_usr w_usr"
	params := url.Values{}
	params.Add("grant_type", "refresh_token")
	params.Add("refresh_token", appRefresh)
	params.Add("client_id", appId)
	params.Add("scope", scope)
	params.Add("client_secret", appSecret)

	inst, err := refreshInstance.Post("/token", params)

	// WARNING: it seems that the axios package does not handle the error when the response is not 200
	// so we need to check the status code ourselves inside the body of the response
	if err != nil {
		log.Printf("\n[services][tidal][auth][CompleteUserAuth] Error refreshing token - %v\n", err)
		return "", err
	}

	refresh := &RefreshTokenResponse{}
	err = json.Unmarshal(inst.Data, refresh)
	if err != nil {
		log.Printf("\n[services][tidal][auth][CompleteUserAuth] Error parsing refresh token response - %v\n", err)
		return "", err
	}
	return refresh.AccessToken, nil
}

type Request struct {
	Base        string
	Headers     map[string][]string
	Method      string
	Credentials *blueprint.IntegrationCredentials
}

func (s *Service) MakeRequest(link string, response interface{}) error {
	accessToken, err := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if err != nil {
		log.Printf("\n[services][tidal][MakeRequest] - error fetching new auth token - %v\n", err)
		return err
	}
	axiosInstance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: s.Base,
		Headers: map[string][]string{
			"Content-Type":  {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	})

	resp, err := axiosInstance.Get(link)
	if err != nil {
		log.Printf("\n[services][tidal][MakeRequest] - error making request - %v\n", err)
		return err
	}
	if resp.Status >= 400 {
		log.Printf("\n[services][tidal][MakeRequest] - error making request - %v\n", resp.Status)
		return err
	}
	err = json.Unmarshal(resp.Data, response)
	if err != nil {
		log.Printf("\n[services][tidal][MakeRequest] - error parsing response - %v\n", err)
		return err
	}
	return nil
}

//⚠️ Legacy code: this function uses a reverse engineered API endpoint to create a playlist on TIDAL. In the future, we will
// move to using the tidal official sdk to create playlists, if it is available. This still works and nothing is wrong with it other
// than the fact that it is not official.
//
// https://listen.tidal.com/v2/my-collection/playlists/folders/create-playlist?description=&folderId=root&isPublic=false&name=xxxxx&countryCode=US&locale=en_US&deviceType=BROWSER - create playlist PUT
// https://listen.tidal.com/v2/my-collection/playlists/folders/remove?trns=trn:playlist:a4a41a8c-a14e-4e60-b671-5f23f07a8a7d&countryCode=US&locale=en_US&deviceType=BROWSER - delete playlist. params in the format, encoded: trns:playlist:playlist_id PUT

func (s *Service) CreateNewPlaylist(title, description, musicToken string, tracks []string) ([]byte, error) {
	log.Printf("\n[services][tidal][CreateNewPlaylist] - creating new playlist - %v\n", title)
	accessToken, err := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error fetching new auth token - %v\n", err)
		return nil, err
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://listen.tidal.com/v2/my-collection/playlists/folders/",
		Headers: map[string][]string{
			"Content-Type":  {"application/x-www-form-urlencoded"},
			"Authorization": {fmt.Sprintf("Bearer %s", accessToken)},
		},
	})
	p := url.Values{}
	p.Add("description", description)
	p.Add("folderId", "root")
	p.Add("isPublic", "true")
	p.Add("name", title)
	p.Add("countryCode", "US")
	p.Add("locale", "en_US")
	p.Add("deviceType", "BROWSER")

	inst, err := instance.Put("create-playlist", p)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error creating playlist - %v\n", err)
		return nil, err
	}

	if inst.Status != 200 {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error creating playlist - %v\n", err)
		return nil, err
	}

	playlist := &CreatePlaylistResponse{}
	err = json.Unmarshal(inst.Data, playlist)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error parsing playlist response - %v\n", err)
		return nil, err
	}

	// now add tracks to the playlist. the tracks are added in url encoded format, with property of trackIds and can take multiple values. the api endpoint is like: https://listen.tidal.com/v1/playlists/287fae69-37f0-40cf-b95f-52d8a3173530/items?countryCode=US&locale=en_US&deviceType=BROWSER
	instance = axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://listen.tidal.com/v1/playlists/",
		Headers: map[string][]string{
			"Content-Type":  {"application/x-www-form-urlencoded"},
			"Authorization": {fmt.Sprintf("Bearer %s", accessToken)},
			"if-none-match": {"*"},
		},
	})
	p = url.Values{}
	p.Add("trackIds", strings.Join(tracks, ","))

	p.Add("onDupes", "FAIL")
	p.Add("onArtifactNotFound", "FAIL")

	inst, err = instance.Post(fmt.Sprintf("%s/items?countryCode=US&locale=en_US&deviceType=BROWSER", playlist.Data.Uuid), p)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error adding tracks to playlist - %v\n", err)
		return nil, err
	}

	if inst.Status != 200 {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error adding tracks to playlist - %v\n", err)
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error adding tracks to playlist - %v\n", string(inst.Data))
		return nil, err
	}

	itemRes := &PlaylistItemAdditionResponse{}
	err = json.Unmarshal(inst.Data, itemRes)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error parsing playlist item addition response - %v\n", err)
		return nil, err
	}

	createdPlaylistLink := fmt.Sprintf("https://tidal.com/browse/playlist/%s", playlist.Data.Uuid)
	return []byte(createdPlaylistLink), nil
}

// FetchUserPlaylists - fetches the user's playlists
func (s *Service) FetchLibraryPlaylists(refreshToken string) ([]blueprint.UserPlaylist, error) {
	log.Printf("\n[services][tidal][FetchUserPlaylists] - fetching user playlists\n")

	accessToken, err := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if err != nil {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching new auth token - %v\n", err)
		return nil, err
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://listen.tidal.com",
		Headers: map[string][]string{
			"Content-Type":  {"application/x-www-form-urlencoded"},
			"Authorization": {fmt.Sprintf("Bearer %s", accessToken)},
		},
	})

	p := url.Values{}
	p.Add("countryCode", "US")
	p.Add("locale", "en_US")
	p.Add("deviceType", "BROWSER")
	p.Add("limit", "50")
	p.Add("order", "DATE")
	p.Add("orderDirection", "DESC")
	p.Add("folderId", "root")

	endpoint := "/v2/my-collection/playlists/folders?folderId=root&countryCode=US&locale=en_US&deviceType=BROWSER&limit=50&order=DATE&orderDirection=DESC"

	inst, err := instance.Get(endpoint, p)
	if err != nil {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching user playlists - %v\n", err)
		return nil, err
	}

	if inst.Status != 200 {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching user playlists - %v\n", string(inst.Data))
		return nil, err
	}

	playlists := &UserPlaylistResponse{}
	err = json.Unmarshal(inst.Data, playlists)

	if err != nil {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error parsing playlists response - %v\n", err)
		return nil, err
	}

	for {
		if playlists.Cursor == "" {
			continue
		}
		endpoint := fmt.Sprintf("/v2/my-collection/playlists/folders?folderId=root&countryCode=US&locale=en_US&deviceType=BROWSER&limit=50&order=DATE&orderDirection=DESC&cursor=%s", playlists.Cursor)
		res, err := instance.Get(endpoint, p)
		if err != nil {
			log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching user playlists - %v\n", err)
			return nil, err
		}
		// deserialize the response
		resp := &UserPlaylistResponse{}
		err = json.Unmarshal(res.Data, &resp)
		if err != nil {
			log.Printf("\n[services][tidal][FetchUserPlaylists] - error parsing playlists response - %v\n", err)
			return nil, err
		}
		if resp.Cursor == "" {
			log.Printf("\n[services][tidal][FetchUserPlaylists] - no more playlists to fetch. All playlist in library fetched\n")
			break
		}
		playlists.Cursor = resp.Cursor
		playlists.Items = append(playlists.Items, resp.Items...)
	}
	log.Printf("\n[services][tidal][FetchUserPlaylists] - fetched %d playlists\n", len(playlists.Items))
	if playlists == nil {
		log.Printf("[platforms][tidal][FetchUserPlaylists] error - error fetching tidal playlists %v\n", err)
	}

	log.Printf("[platforms][FetchPlatformPlaylists] tidal playlists fetched successfully")
	// create a slice of UserLibraryPlaylists
	var userPlaylists []blueprint.UserPlaylist
	for _, playlist := range playlists.Items {
		if playlist.ItemType != "PLAYLIST" {
			log.Printf("[platforms][FetchPlatformPlaylists] Item is not a playlist data, skipping...\n")
			continue
		}

		data := playlist.Data
		userPlaylists = append(userPlaylists, blueprint.UserPlaylist{
			ID:            data.UUID,
			Title:         data.Title,
			Public:        util.TidalIsPrivate(data.SharingLevel),
			Collaborative: util.TidalIsCollaborative(data.ContentBehavior),
			NbTracks:      data.NumberOfTracks,
			URL:           playlist.Data.URL,
			Cover:         playlist.Data.Image,
			CreatedAt:     data.Created,
			Owner:         playlist.Data.Creator.Name,
		})
	}
	return userPlaylists, nil
}

// FetchUserArtists - fetches the user's artists
func (s *Service) FetchUserArtists(userId string) (*blueprint.UserLibraryArtists, error) {
	// for tidal, we're fetching maximum of 500 artists. this is due to the fact that there's no
	// tidal access for now except for me and also makes implementation easier (even if we get tidal users today)
	// also the tidal api itself uses 500 as the limit in the browser.
	log.Printf("\n[services][tidal][FetchUserArtists] - fetching user artists\n")

	link := fmt.Sprintf("/v1/users/%s/favorites/artists?offset=0&limit=50&order=DATE&orderDirection=DESC&countryCode=US&locale=en_US&deviceType=BROWSER", userId)

	artistResponse := &UserArtistsResponse{}
	//err := NewTidalRequest("https://listen.tidal.com", map[string][]string{
	//	"Content-Type": {"application/x-www-form-urlencoded"},
	//}, "GET").MakeRequest(link, artistResponse)
	err := s.MakeRequest(link, artistResponse)
	if err != nil {
		log.Printf("\n[services][tidal][FetchUserArtists] - error fetching user artists - %v\n", err)
		return nil, err
	}

	var artists []blueprint.UserArtist
	for _, artist := range artistResponse.Items {
		artists = append(artists, blueprint.UserArtist{
			Name:  artist.Item.Name,
			ID:    strconv.Itoa(artist.Item.Id),
			Cover: artist.Item.Picture,
			URL:   artist.Item.Url,
		})
	}

	response := blueprint.UserLibraryArtists{
		Payload: artists,
		Total:   artistResponse.TotalNumberOfItems,
	}
	return &response, nil
}

func (s *Service) FetchListeningHistory(refreshToken string) ([]blueprint.TrackSearchResult, error) {

	return nil, blueprint.ErrNotImplemented
}
