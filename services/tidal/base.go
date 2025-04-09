package tidal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/go-redis/redis/v8"
	"github.com/nleeper/goment"
	"github.com/vicanso/go-axios"
)

const ApiUrl = "https://listen.tidal.com/v1"
const AuthBase = "https://auth.tidal.com/v1/oauth2"

type PlatformService interface {
	SearchPlaylistWithID(id string) (*blueprint.PlaylistSearchResult, error)
	SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error)
}

type Service struct {
	DB                     *sqlx.DB
	Redis                  *redis.Client
	IntegrationCredentials *blueprint.IntegrationCredentials
	Base                   string
	App                    *blueprint.DeveloperApp
	WebhookSender          WebhookSender
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
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error) {
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

		ok := s.WebhookSender.SendTrackEvent(searchData.Meta.PlaylistID, &blueprint.PlaylistConversionEventTrack{
			EventType: blueprint.PlaylistConversionTrackEvent,
			Platform:  IDENTIFIER,
			TaskId:    searchData.Meta.PlaylistID,
			Track:     &deserialized,
		})
		if !ok {
			log.Printf("[services][platforms][tidal][SearchTrackWithID] - could not send track event for TIDAL %v\n", err)
		}

		return &deserialized, nil
	}

	result, err := s.FetchSingleTrackByTitle(searchData.Title, searchData.Artists[0])
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][SearchTrackWithTitle] - could not search track with title '%s' on tidal - %v\n", searchData.Title, err)
		return nil, err
	}

	svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)

	// here is where we select the best match. Right now, we just select the first result on the list
	// but ideally if for example we want to filter more "generic" tracks, we can do that here
	// etc.
	if len(result.Tracks.Items) > 0 {
		var track = result.Tracks.Items[0]
		var artistes []string
		for _, artist := range track.Artists {
			artistes = append(artistes, artist.Name)
		}

		tidalTrack := &blueprint.TrackSearchResult{
			URL:      track.Url,
			Artists:  artistes,
			Released: track.StreamStartDate,
			Duration: util.GetFormattedDuration(track.Duration),
			// format the duration in milliseconds. seems to be in seconds from TIDAL
			DurationMilli: track.Duration * 1000,
			Explicit:      track.Explicit,
			Title:         track.Title,
			Preview:       "",
			Album:         track.Album.Title,
			ID:            strconv.Itoa(track.Id),
			Cover:         util.BuildTidalAssetURL(track.Album.Cover),
		}
		ok := util.CacheTrackByArtistTitle(tidalTrack, s.Redis, IDENTIFIER)
		if !ok {
			log.Printf("[services][tidal][SearchTrackWithID] - could not cache track - %v\n", err)
		}

		ok2 := svixInstance.SendTrackEvent(s.App.WebhookAppID, &blueprint.PlaylistConversionEventTrack{
			EventType: blueprint.PlaylistConversionTrackEvent,
			Platform:  IDENTIFIER,
			// TaskId:    searchData.,
			Track: tidalTrack,
		})

		if !ok2 {
			log.Printf("Could not send track event for TIDAL\n")
		} else {
			log.Printf("Successfully sent playlist conversion track event for TIDAL\n")
		}

		return tidalTrack, nil
	}
	return nil, blueprint.EnoResult

}

// FetchSingleTrackByTitle fetches a track from tidal by title and artist
func (s *Service) FetchSingleTrackByTitle(title, artiste string) (*SearchResult, error) {
	log.Printf("[controllers][platforms][tidal][FetchSingleTrackByTitle] - searching single track by title: %s %s\n", title, artiste)
	accessToken, err := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrackByTitle] - error - %v\n", err)
		return nil, err
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	})

	strippedTrackTitleInfo := util.ExtractTitle(title)

	query := url.QueryEscape(fmt.Sprintf("%s %s", artiste, strippedTrackTitleInfo.Title))

	log.Printf("[controllers][[platforms][tidal][FetchSingleTrackByTitle]  - Search URL %s\n", fmt.Sprintf("%s %s", artiste, title))

	response, err := instance.Get(fmt.Sprintf("/search/top-hits?query=%s&countryCode=US&limit=2&offset=0&types=TRACKS", query))
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrackByTitle] - error - %v\n", err)
		return nil, err
	}
	searchResult := &SearchResult{}
	err = json.Unmarshal(response.Data, searchResult)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrackByTitle] - could not deserialize search response from tidal - %v\n", err)
		return nil, err
	}
	return searchResult, nil
}

// FetchPlaylistInfo returns a playlist info[main] Error processing task handler not found for task
func (s *Service) FetchPlaylistInfo(id string) (*PlaylistInfo, error) {
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

func (s *Service) FetchTracksForSourcePlatform(info *blueprint.LinkInfo, playlistMeta *blueprint.PlaylistMetadata, resultChan chan blueprint.TrackSearchResult) error {
	log.Println("Going to asynchronously fetch the tracks from spotify now and send each result to the channel as they come in")
	identifierHash := fmt.Sprintf("tidal:playlist:%s", info)
	infoHash := fmt.Sprintf("tidal:snapshot:%s", info)

	if s.Redis.Exists(context.Background(), identifierHash).Val() == 1 {
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

		res := &PlaylistTracks{}
		err = json.Unmarshal(response.Data, res)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not deserialize playlist result from tidal - %v\n", err)
			return err
		}
		if len(res.Items) == 0 {
			break
		}

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

func (s *Service) FetchPlaylistMetaInfo(info *blueprint.LinkInfo) (*blueprint.PlaylistMetadata, error) {
	_ = fmt.Sprintf("tidal:playlist:%s", info)
	log.Printf("Converting playlist with ID %s on TIDAL\n", info)

	// infoHash represents the key for the snapshot of the playlist playlistInfo, in this case
	// just a lasUpdated timestamp in string format.
	_ = fmt.Sprintf("tidal:snapshot:%s", info)

	playlistInfo, err := s.FetchPlaylistInfo(info.EntityID)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not fetch playlist playlistInfo - %v\n", err)
		return nil, err
	}

	svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	playlistMeta := &blueprint.PlaylistMetadata{
		// no length yet. its calculated by adding up all the track lengths in the playlist
		Length: util.GetFormattedDuration(playlistInfo.Duration),
		Title:  playlistInfo.Title,
		// no preview. might be an abstracted implementation in the future.
		Preview: "",

		// todo: fetch user's orchdio @ and/or id and enrich. might need a specific owner object. support fetching by tidal id for tidal implementation
		Owner: strconv.Itoa(playlistInfo.Creator.Id),
		// fixme: possible nil pointer
		Cover:  playlistInfo.SquareImage,
		Entity: "playlist",
		URL:    playlistInfo.Url,
		// no short url here.
		// todo: try to pass the entity id here
		ShortURL:    info.TaskID,
		NBTracks:    playlistInfo.NumberOfTracks,
		Description: playlistInfo.Description,
		LastUpdated: playlistInfo.LastUpdated,
	}

	// todo: send playlist conversion metadata event here
	ok := svixInstance.SendPlaylistMetadataEvent(&blueprint.LinkInfo{
		Platform: IDENTIFIER,
		EntityID: info.EntityID,
	}, &blueprint.PlaylistConversionEventMetadata{
		Platform:  IDENTIFIER,
		Meta:      playlistMeta,
		EventType: blueprint.PlaylistConversionMetadataEvent,
	})

	if !ok {
		log.Printf("Could not send playlist metadata event - %v\n", err)
	} else {
		log.Printf("Successfully sent playlist metadata event for TIDAL conversion - %v\n", err)
	}

	return playlistMeta, nil
}

// SearchPlaylistWithID fetches a specific playlist based on the id. It returns the playlist search result,
// a bool to indicate if the playlist has been updated since the last time a call was made
// and an error if there is one
func (s *Service) SearchPlaylistWithID(info *blueprint.LinkInfo) (*blueprint.PlaylistSearchResult, error) {
	identifierHash := fmt.Sprintf("tidal:playlist:%s", info)
	log.Printf("Converting playlist with ID %s on TIDAL\n", info)

	// infoHash represents the key for the snapshot of the playlist playlistInfo, in this case
	// just a lasUpdated timestamp in string format.
	infoHash := fmt.Sprintf("tidal:snapshot:%s", info)

	playlistInfo, err := s.FetchPlaylistInfo(info.EntityID)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not fetch playlist playlistInfo - %v\n", err)
		return nil, err
	}

	svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	playlistMeta := &blueprint.PlaylistMetadata{
		// no length yet. its calculated by adding up all the track lengths in the playlist
		Length: util.GetFormattedDuration(playlistInfo.Duration),
		Title:  playlistInfo.Title,
		// no preview. might be an abstracted implementation in the future.
		Preview: "",

		// todo: fetch user's orchdio @ and/or id and enrich. might need a specific owner object. support fetching by tidal id for tidal implementation
		Owner: strconv.Itoa(playlistInfo.Creator.Id),
		// fixme: possible nil pointer
		Cover:  playlistInfo.SquareImage,
		Entity: "playlist",
		URL:    playlistInfo.Url,
		// no short url here.
		// todo: try to pass the entity id here
		ShortURL:    info.TaskID,
		NBTracks:    playlistInfo.NumberOfTracks,
		Description: playlistInfo.Description,
	}

	// todo: send playlist conversion metadata event here
	ok := svixInstance.SendPlaylistMetadataEvent(&blueprint.LinkInfo{
		Platform: IDENTIFIER,
		EntityID: info.EntityID,
	}, &blueprint.PlaylistConversionEventMetadata{
		Platform:  IDENTIFIER,
		Meta:      playlistMeta,
		EventType: blueprint.PlaylistConversionMetadataEvent,
	})

	if !ok {
		log.Printf("Could not send playlist metadata event - %v\n", err)
	} else {
		log.Printf("Successfully sent playlist metadata event for TIDAL conversion - %v\n", err)
	}

	// if we have already cached the playlist playlistInfo.
	// The assumption here is that the playlist playlistInfo and the playlist tracks are always both cached every time
	if s.Redis.Exists(context.Background(), identifierHash).Val() == 1 {
		// fetch the playlist playlistInfo from redis
		cachedInfo, gErr := s.Redis.Get(context.Background(), infoHash).Result()
		if gErr != nil && !errors.Is(gErr, redis.Nil) {
			log.Printf("\n[controllers][platforms][tidal][SearchPlaylistWithID] - could not fetch cached playlist playlistInfo - %v\n", err)
			return nil, err
		}

		// deserialize the playlist playlistInfo
		var cachedLastPlayedAt string
		_ = json.Unmarshal([]byte(cachedInfo), &cachedLastPlayedAt)

		// format the timestamps on both of the playlist playlistInfo
		lastUpdated, gmErr := goment.New(cachedLastPlayedAt)
		infoLastUpdated, gmErr2 := goment.New(playlistInfo.LastUpdated)

		if gmErr != nil || gmErr2 != nil {
			log.Printf("\n[controllers][platforms][tidal][SearchPlaylistWithID] - could not parse last updated time - %v - %v\n", gmErr, gmErr2)
			return nil, err
		}

		var result *blueprint.PlaylistSearchResult
		// fetch the cached tracks from redis.
		cachedResult, sErr := s.Redis.Get(context.Background(), identifierHash).Result()
		if sErr != nil {
			log.Printf("\n[services][tidal][FetchPlaylistTracksInfo] - ⚠️ error fetching key from redis. - %v\n", sErr)
			return nil, sErr
		}
		// deserialize the tracks we fetched from redis
		jErr := json.Unmarshal([]byte(cachedResult), &result)
		if jErr != nil {
			log.Printf("\n[services][tidal][FetchPlaylistTracksInfo] - ⚠️ error deserializimng cache result - %v\n", jErr)
			return nil, jErr
		}

		// if the timestamps are the same, that means that our playlist has not
		// changed, so we can return the cached result. in the other case, we
		// are doing nothing so we go on to fetch the tracks from the tidal api.
		if lastUpdated.IsSame(infoLastUpdated) {
			// send webhook event track conversion for each (cached) track here
			// fixme: send these async
			for i := range result.Tracks {
				track := &result.Tracks[i]
				ok1 := svixInstance.SendTrackEvent(info.App, &blueprint.PlaylistConversionEventTrack{
					EventType: blueprint.PlaylistConversionTrackEvent,
					Platform:  IDENTIFIER,
					TaskId:    info.EntityID,
					Track:     track,
				})

				if !ok1 {
					log.Printf("Could not send playlist track event for track %s for tidal platform.\n", track.Title)
				} else {
					log.Printf("Successfully sent playlist track event for TIDAL conversion - %v\n", track)
				}
			}
			return result, nil
		}
	}

	accessToken, sErr := s.FetchNewAuthToken(s.IntegrationCredentials.AppID, s.IntegrationCredentials.AppSecret, s.IntegrationCredentials.AppRefreshToken)
	if sErr != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - error - %v\n", sErr)
		return nil, err
	}
	playlistResult := &PlaylistTracks{}

	var pages = playlistInfo.NumberOfTracks / 100
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
			return nil, err
		}
		res := &PlaylistTracks{}
		err = json.Unmarshal(response.Data, res)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not deserialize playlist result from tidal - %v\n", err)
			return nil, err
		}
		if len(res.Items) == 0 {
			break
		}
		playlistResult.Items = append(playlistResult.Items, res.Items...)
	}

	var tracks []blueprint.TrackSearchResult
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

		tracks = append(tracks, t)
	}
	// then convert to a blueprint.PlaylistSearchResult
	result := &blueprint.PlaylistSearchResult{
		Title:   playlistInfo.Title,
		Tracks:  tracks, // TODO: playlistResult.Items,
		URL:     playlistInfo.Url,
		Length:  util.GetFormattedDuration(playlistInfo.Duration),
		Preview: "",
		Owner:   strconv.Itoa(playlistInfo.Creator.Id), // TODO: implement fetching the user with this ID and populating it here,
		Cover:   util.BuildTidalAssetURL(playlistInfo.SquareImage),
		ID:      playlistInfo.Uuid,
	}

	ok2 := svixInstance.SendTrackEvent(s.App.WebhookAppID, &blueprint.PlaylistConversionEventTrack{
		EventType: blueprint.PlaylistConversionTrackEvent,
		Platform:  IDENTIFIER,
		TaskId:    info.EntityID,
		Track:     nil,
	})

	if !ok2 {
		log.Printf("Could not send playlist track event for TIDAL platform- \n")
	} else {
		log.Printf("Successfully sent playlist track event for TIDAL platform- \n")
	}

	ser, _ := json.Marshal(result)
	// cache the result
	err = s.Redis.Set(context.Background(), identifierHash, ser, 0).Err()
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not cache playlist for %s into redis - %v\n", err, playlistInfo.Title)
	} else {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - cached playlist into redis - %v\n", playlistInfo.Title)
	}

	infoSer, _ := json.Marshal(playlistInfo.LastUpdated)
	err = s.Redis.Set(context.Background(), infoHash, infoSer, 0).Err()
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not cache playlist playlistInfo for %s playlistInfo into redis - %v\n", err, playlistInfo.Title)
	} else {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - cached playlist playlistInfo into redis - %v\n", playlistInfo.Title)
	}
	return result, err
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
func (s *Service) FetchUserPlaylists() (*UserPlaylistResponse, error) {
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

	endpoint := fmt.Sprintf("/v2/my-collection/playlists/folders?folderId=root&countryCode=US&locale=en_US&deviceType=BROWSER&limit=50&order=DATE&orderDirection=DESC")

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
	return playlists, nil
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
