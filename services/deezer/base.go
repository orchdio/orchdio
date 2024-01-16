package deezer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"log"
	"net/http"
	"net/url"
	"orchdio/blueprint"
	webhook "orchdio/convoy.go"
	"orchdio/util"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/samber/lo"
	"github.com/vicanso/go-axios"
)

type SearchInfo struct {
}

// ExtractTitle retrieves the title of a track if it contains opening and closing brackets
// This is to improve the searching accuracy when searching for these tracks on platforms
//func ExtractTitle(title string) string {
//	openingBracketIndex := strings.Index(title, "(")
//	closingBracketIndex := strings.LastIndex(title, ")")
//	if openingBracketIndex != -1 && closingBracketIndex != -1 {
//		return title[:openingBracketIndex]
//	}
//	return title
//}

type Service struct {
	IntegrationID     string
	IntegrationSecret string
	RedisClient       *redis.Client
	Logger            *zap.Logger
	PgClient          *sqlx.DB
}

//NewService creates a new deezer service
func NewService(credentials *blueprint.IntegrationCredentials, pgClient *sqlx.DB, redisClient *redis.Client, logger *zap.Logger) *Service {
	return &Service{
		IntegrationID:     credentials.AppID,
		IntegrationSecret: credentials.AppSecret,
		RedisClient:       redisClient,
		Logger:            logger,
		PgClient:          pgClient,
	}
}

// FetchSingleTrack fetches a single deezer track from the URL
func (s *Service) FetchSingleTrack(link string) (*Track, error) {
	response, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][playlist][SearchTrackWithID] error - Could not fetch single track from deezer %v\n", err)

		return nil, err
	}

	singleTrack := &Track{}
	err = json.Unmarshal(response.Data, singleTrack)
	if err != nil {
		log.Printf("\n[services][deezer][playlist][SearchTrackWithID] error - Could not deserialize response %v\n", err)
		return nil, err
	}
	return singleTrack, nil
}

// SearchTrackWithID fetches the deezer result for the track being searched using the URL
func (s *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	// first, get the cached track
	//cachedKey := fmt.Sprintf("%s-%s", info.Platform, info.EntityID)
	cachedKey := "deezer:track:" + info.EntityID
	cachedTrack, err := s.RedisClient.Get(context.Background(), cachedKey).Result()
	if err != nil && err != redis.Nil {
		s.Logger.Error("[services][deezer][SearchTrackWithID][SearchTrackWithID] error - Could not get cached track", zap.Error(err))
		return nil, err
	}

	// if we have not cached this track before
	if err != nil && err == redis.Nil {
		s.Logger.Warn("[services][deezer][SearchTrackWithID][SearchTrackWithID] warning - Track has not been cached", zap.String("cached_key", cachedKey))
		dzSingleTrack, sErr := s.FetchSingleTrack(info.TargetLink)
		if sErr != nil {
			s.Logger.Error("[services][deezer][SearchTrackWithID][SearchTrackWithID] error - Could not fetch single track from deezer", zap.Error(sErr))
			return nil, sErr
		}
		var dzTrackContributors []string
		for _, contributor := range dzSingleTrack.Contributors {
			if contributor.Type == "artist" {
				dzTrackContributors = append(dzTrackContributors, contributor.Name)
			}
		}

		fetchedDeezerTrack := blueprint.TrackSearchResult{
			Explicit:      util.DeezerIsExplicit(dzSingleTrack.ExplicitContentLyrics),
			Duration:      util.GetFormattedDuration(dzSingleTrack.Duration),
			DurationMilli: dzSingleTrack.Duration * 1000,
			URL:           dzSingleTrack.Link,
			Artists:       dzTrackContributors,
			Released:      dzSingleTrack.Album.ReleaseDate,
			Title:         dzSingleTrack.Title,
			Preview:       dzSingleTrack.Preview,
			Album:         dzSingleTrack.Album.Title,
			ID:            strconv.Itoa(dzSingleTrack.ID),
			Cover:         dzSingleTrack.Album.Cover,
		}

		// serialize the result
		serializedTrack, jErr := json.Marshal(fetchedDeezerTrack)
		if jErr != nil {
			s.Logger.Warn("[services][deezer][SearchTrackWithID][SearchTrackWithID] warning - Could not serialize track", zap.Error(jErr))
		}

		// cache the result
		_ = s.RedisClient.Set(context.Background(), cachedKey, string(serializedTrack), time.Hour*24).Err()
		s.Logger.Info("[services][deezer][SearchTrackWithID][SearchTrackWithID] Track has been cached", zap.String("cached_key", cachedKey))
		return &fetchedDeezerTrack, nil
	}

	var result blueprint.TrackSearchResult
	err = json.Unmarshal([]byte(cachedTrack), &result)
	if err != nil {
		s.Logger.Warn("[services][deezer][SearchTrackWithID][SearchTrackWithID] warning - Could not deserialize cached result", zap.Error(err))
		return nil, err
	}
	return &result, nil
}

// SearchTrackWithTitle searches for a track using the title (and artiste) on deezer
// This is typically expected to be used when the track we want to fetch is the one we just
// want to search on. That is, the other platforms that the user is trying to convert to.
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error) {
	//searchKey := fmt.Sprintf("deezer-%s-%s", artiste, title)
	cacheKey := fmt.Sprintf("deezer:%s:%s", util.NormalizeString(searchData.Artists[0]), util.ExtractTitle(searchData.Title).Title)
	// get the cached track
	if s.RedisClient.Exists(context.Background(), cacheKey).Val() == 1 {
		s.Logger.Info("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] Track has been cached", zap.String("cached_key", cacheKey))
		// deserialize the result from redis
		cachedTrack, err := s.RedisClient.Get(context.Background(), cacheKey).Result()
		if err != nil {
			s.Logger.Warn("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] warning - Could not get cached track", zap.Error(err))
			return nil, err
		}
		var deserializedTrack *blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
		if err != nil {
			s.Logger.Warn("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] warning - Could not deserialize cached track", zap.Error(err))
			return nil, err
		}
		return deserializedTrack, nil
	}

	s.Logger.Warn("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] warning - Track has not been cached", zap.String("cached_key", cacheKey))

	strippedTrackTitle := util.ExtractTitle(searchData.Title)
	searchTitle := strippedTrackTitle.Title
	// for deezer we'll not trim the artiste name. this is because it becomes way less accurate.
	// deezer has second to the lowest accuracy in terms of search results (youtube being the lowest)
	// however, just like others, we're caching the result under the normalized string, which contains trimmed artiste name
	// like so: "deezer-artistename-title". For example: "deezer-flatbushzombies-reelgirls
	_link := fmt.Sprintf("track:\"%s\" artist:\"%s\"", strings.Trim(searchTitle, " "), searchData.Artists[0])
	link := fmt.Sprintf("%s/search?q=%s", os.Getenv("DEEZER_API_BASE"), url.QueryEscape(_link))

	response, err := axios.Get(link)
	if err != nil {
		s.Logger.Error("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] error - Could not search the track on deezer", zap.Error(err))
		return nil, err
	}

	fullTrack := FullTrack{}
	err = json.Unmarshal(response.Data, &fullTrack)
	if err != nil {
		s.Logger.Error("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] error - Could not deserialize the body into the out response", zap.Error(err),
			zap.String("body", string(response.Data)))
		return nil, err
	}

	// NB: when the time comes to properly handle the results and return the best match (sometimes its like the 2nd result)
	// then, this is where to probably start.
	if len(fullTrack.Data) > 0 {
		track := fullTrack.Data[0]
		artistes := []string{track.Artist.Name}
		if len(strippedTrackTitle.Artists) > 0 {
			artistes = append(artistes, strippedTrackTitle.Artists...)
		}

		out := blueprint.TrackSearchResult{
			URL:           track.Link,
			Artists:       lo.Uniq(artistes),
			Released:      "",
			Duration:      util.GetFormattedDuration(track.Duration),
			DurationMilli: track.Duration * 1000,
			Explicit:      util.DeezerIsExplicit(track.ExplicitContentLyrics),
			Title:         track.Title,
			Preview:       track.Preview,
			Album:         track.Album.Title,
			ID:            strconv.Itoa(track.ID),
			Cover:         track.Album.Cover,
		}

		// serialize the result
		serializedTrack, jErr := json.Marshal(out)
		if jErr != nil {
			s.Logger.Warn("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] warning - Could not serialize track", zap.Error(jErr))
		}
		//newHashIdentifier := util.HashIdentifier("deezer-" + out.Artistes[0] + "-" + out.Title)
		// if the artistes are the same, the track result is most likely the same (except remixes, an artiste doesnt have two tracks with the same name)

		// cache tracks. Here we are caching both with hash identifier and with the ID of the track itself
		// this is because in some cases, we need to fetch by ID and not by title
		// cache track but with identifier. this is for when we're searching by title again and its the same
		// track as this
		if lo.Contains(out.Artists, searchData.Artists[0]) {
			cacheErr := s.RedisClient.Set(context.Background(), cacheKey, string(serializedTrack), time.Hour*24).Err()
			if cacheErr != nil {
				s.Logger.Warn("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] warning - Could not cache track", zap.Error(cacheErr))
			}
		}
		return &out, nil
	}

	s.Logger.Warn("[services][deezer][SearchTrackWithTitle][SearchTrackWithTitle] warning - Deezer search for track done but no results", zap.String("searched_with", _link))
	return nil, nil
}

// SearchTrackWithTitleChan searches for a track similar to `SearchTrackWithTitle` but uses a channel
func (s *Service) SearchTrackWithTitleChan(searchData *blueprint.TrackSearchData, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	defer wg.Done()
	result, err := s.SearchTrackWithTitle(searchData)
	if err != nil {
		//defer wg.Done()
		c <- nil
		//wg.Add(1)
		return
	}
	//defer wg.Done()
	c <- result
	//wg.Add(1)
	return
}

// FetchTracks searches for the tracks (titles) passed and returns the tracks on deezer.
// This function is used to search for tracks in the playlists the user is trying to convert, on deezer
func (s *Service) FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client,
	webhookId, taskId string) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var fetchedTracks []blueprint.TrackSearchResult
	var omittedTracks []blueprint.OmittedTracks
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	wg := sync.WaitGroup{}
	for _, track := range tracks {
		wg.Add(1)
		// in order to create the identifier that we use to recognize tracks in cache, we simply take the artiste
		// name. but the thing is that an artiste can have spaces in their name, etc. this is definitely going to not go as we expect
		// so we need to remove spaces and weird characters from the artiste name
		// this is the same for the title of the track

		//cleanedArtiste := util.NormalizeString("deezer-" + track.Artistes[0] + "-" + track.Title)
		cleanedArtiste := fmt.Sprintf("deezer-%s-%s", util.NormalizeString(track.Artistes[0]), track.Title)
		// WARNING: unhandled slice index
		// check if its been cached. if so, we grab and return it. if not, we let it search
		if s.RedisClient.Exists(context.Background(), cleanedArtiste).Val() == 1 {
			// deserialize the result from redis
			var deserializedTrack *blueprint.TrackSearchResult
			cachedResult := s.RedisClient.Get(context.Background(), cleanedArtiste).Val()
			err := json.Unmarshal([]byte(cachedResult), &deserializedTrack)
			if err != nil {
				s.Logger.Warn("[services][deezer][FetchTracks][FetchTracks] warning - Could not deserialize cached result", zap.Error(err))
				return nil, nil
			}
			fetchedTracks = append(fetchedTracks, *deserializedTrack)
			continue
		}
		searchData := blueprint.TrackSearchData{
			Title:   track.Title,
			Artists: track.Artistes,
			Album:   track.Album,
		}

		// WARNING: unhandled slice index
		go s.SearchTrackWithTitleChan(&searchData, ch, &wg, red)

		outputTracks := <-ch
		if outputTracks == nil {
			s.Logger.Warn("[services][deezer][FetchTracks][FetchTracks] warning - no track found for title", zap.String("title", track.Title))
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Title:    track.Title,
				URL:      track.URL,
				Artistes: track.Artistes,
			})
			continue
		}
		// create a new webhook event
		convoyInstance := webhook.NewConvoy()

		payload := &blueprint.PlaylistTrackConversionEvent{
			Platform: IDENTIFIER,
			Event:    "playlist:conversion:track:result",
			Data:     outputTracks,
			ID:       taskId,
		}

		cErr := convoyInstance.SendEvent(webhookId, "playlist:conversion:track:result", payload)

		if cErr != nil {
			s.Logger.Error("[services][deezer][FetchTracks][FetchTracks] error - Could not send event to convoy", zap.Error(cErr))
			return nil, nil
		}

		fetchedTracks = append(fetchedTracks, *outputTracks)
	}
	wg.Wait()
	return &fetchedTracks, &omittedTracks
}

// SearchPlaylistWithID fetches tracks under a playlist on deezer with pagination
func (s *Service) SearchPlaylistWithID(id, webhookId, taskId string) (*blueprint.PlaylistSearchResult, error) {
	infoLink := "https://api.deezer.com/playlist/" + id + "?limit=1"
	info, err := axios.Get(infoLink)
	if err != nil {
		s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not fetch playlist info", zap.Error(err))
		return nil, err
	}
	var playlistInfo PlaylistTracksSearch
	err = json.Unmarshal(info.Data, &playlistInfo)
	if err != nil {
		s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not deserialize the body into the out response", zap.Error(err))
		return nil, err
	}

	tracks, err := axios.Get("https://api.deezer.com/playlist/" + id)

	cachedSnapshot, cacheErr := s.RedisClient.Get(context.Background(), "deezer:playlist:"+id).Result()

	if cacheErr != nil && !errors.Is(cacheErr, redis.Nil) {
		s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not get cached snapshot for playlist", zap.Error(cacheErr))
		return nil, cacheErr
	}

	cachedSnapshotID, idErr := s.RedisClient.Get(context.Background(), "deezer:snapshot:"+id).Result()
	if idErr != nil && !errors.Is(idErr, redis.Nil) {
		s.Logger.Warn("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not get cached snapshot id for playlist", zap.Error(idErr))
		return nil, idErr
	}

	// if we have not cached this track or the snapshot has changed (that is, the playlist has been updated), then
	// we need to fetch the tracks and cache them
	if cacheErr != nil && errors.Is(cacheErr, redis.Nil) || cachedSnapshotID != playlistInfo.Checksum {
		var trackList PlaylistTracksSearch
		err = json.Unmarshal(tracks.Data, &trackList)
		if err != nil {
			s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not deserialize the body into the out response", zap.Error(err))
			return nil, err
		}

		var out []blueprint.TrackSearchResult
		for _, track := range trackList.Tracks.Data {
			strippedTitleInfo := util.ExtractTitle(track.Title)
			artistes := []string{track.Artist.Name}
			if len(strippedTitleInfo.Artists) > 0 {
				artistes = append(artistes, strippedTitleInfo.Artists...)
			}

			result := &blueprint.TrackSearchResult{
				URL:     track.Link,
				Artists: lo.Uniq(artistes),
				//Released: track.r,
				Duration:      util.GetFormattedDuration(track.Duration),
				DurationMilli: track.Duration * 1000,
				Explicit:      util.DeezerIsExplicit(track.ExplicitContentLyrics),
				Title:         track.Title,
				Preview:       track.Preview,
				Album:         track.Album.Title,
				ID:            strconv.Itoa(track.Id),
				Cover:         track.Album.Cover,
			}
			// cache the track
			cacheKey := "deezer:track:" + result.ID
			serialized, jErr := json.Marshal(result)
			if jErr != nil {
				s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not serialize track", zap.Error(jErr))
				return nil, jErr
			}

			// send event to convoy. if there is an error, we just log it and continue
			convoyInstance := webhook.NewConvoy()
			cErr := convoyInstance.SendEvent(webhookId, "playlist:conversion:track:result", result)
			if cErr != nil {
				s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not send event to convoy", zap.Error(cErr))
			}

			err = s.RedisClient.Set(context.Background(), cacheKey, string(serialized), 0).Err()
			if err != nil {
				s.Logger.Warn("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not cache track", zap.Error(err))
			}
			out = append(out, *result)
		}

		reply := blueprint.PlaylistSearchResult{
			URL:    trackList.Link,
			Tracks: out,
			Title:  trackList.Title,
			Length: util.GetFormattedDuration(trackList.Duration),
			Owner:  trackList.Creator.Name,
			Cover:  trackList.Picture,
		}

		// update the snapshotID cache
		err = s.RedisClient.Set(context.Background(), "deezer:snapshot:"+id, trackList.Checksum, 0).Err()
		if err != nil {
			s.Logger.Warn("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not cache snapshot id", zap.Error(err))
		}

		// cache the whole playlist
		serializedPlaylist, jErr := json.Marshal(reply)
		if jErr != nil {
			s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not serialize playlist", zap.Error(jErr))
		}
		err = s.RedisClient.Set(context.Background(), "deezer:playlist:"+id, string(serializedPlaylist), 0).Err()
		if err != nil {
			s.Logger.Warn("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not cache playlist", zap.Error(err))
		}

		// cache the checksum (snapshot id)
		err = s.RedisClient.Set(context.Background(), "deezer:snapshot:"+id, trackList.Checksum, 0).Err()
		if err != nil {
			s.Logger.Warn("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not cache snapshot id", zap.Error(err))
		}
		return &reply, nil
	}

	playlistResult := &blueprint.PlaylistSearchResult{}
	err = json.Unmarshal([]byte(cachedSnapshot), playlistResult)
	if err != nil {
		s.Logger.Error("[services][deezer][SearchPlaylistWithID][SearchPlaylistWithID] error - Could not deserialize the body into the out response", zap.Error(err))
		return nil, err
	}
	return playlistResult, nil
}

// SearchPlaylistWithTracks fetches the tracks for a playlist based on the search result
// from another platform
func (s *Service) SearchPlaylistWithTracks(p *blueprint.PlaylistSearchResult, webhookId, taskId string) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Artistes: track.Artists,
			Title:    track.Title,
			ID:       track.ID,
			URL:      track.URL,
		})
	}
	deezerTracks, omittedTracks := s.FetchTracks(trackSearch, s.RedisClient, webhookId, taskId)
	return deezerTracks, omittedTracks
}

// CreateNewPlaylist creates a new playlist for a user on their deezer account
func (s *Service) CreateNewPlaylist(title, userDeezerId, token string, tracks []string) ([]byte, error) {
	deezerAPIBase := os.Getenv("DEEZER_API_BASE")
	reqURL := fmt.Sprintf("%s/user/%s/playlists?access_token=%s&request_method=post", deezerAPIBase, userDeezerId, token)
	p := url.Values{}
	p.Add("title", title)
	out := &PlaylistCreationResponse{}
	_ = axios.NewInstance(&axios.InstanceConfig{
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
	})

	resp, err := axios.Get(reqURL, p)
	if err != nil {
		s.Logger.Error("[services][deezer][CreateNewPlaylist][CreateNewPlaylist] error - Could not create playlist", zap.Error(err))
		return nil, err
	}

	if resp.Status == http.StatusBadRequest {
		//if (strings.Contains(string(resp.Data), "ser"))
		s.Logger.Error("[services][deezer][CreateNewPlaylist][CreateNewPlaylist] error - Could not create playlist. Bad request", zap.Error(err))
		return nil, errors.New("bad request")
	}

	log.Printf("\n[services][deezer][CreateNewPlaylist] response: %v\n", string(resp.Data))

	err = json.Unmarshal(resp.Data, out)

	if err != nil {
		log.Printf("\n[services][deezer][CreateNewPlaylist] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	createResponse := struct {
		ID int `json:"id"`
	}{}
	err = json.Unmarshal(resp.Data, &createResponse)
	if err != nil {
		log.Printf("\n[services][deezer][CreateNewPlaylist] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	// convert createResponse ID to string
	playlistID := strconv.Itoa(createResponse.ID)
	// convert playlistID to []byte
	playlistIDBytes := []byte(playlistID)

	allTracks := strings.Join(tracks, ",")
	updatePlaylistURL := fmt.Sprintf("%s/playlist/%d/tracks?access_token=%s&request_method=post", deezerAPIBase, out.ID, token)
	p = url.Values{}
	p.Add("songs", allTracks)
	resp, err = axios.Get(updatePlaylistURL, p)
	if err != nil {
		log.Printf("\n[services][deezer][CreateNewPlaylist] error - Could not update playlist: %v\n", err)
		return nil, err
	}

	// HACK: for some reason, if our playlist contains invalid track ids, deezer will return a 200 error but the response body
	// will contain an error message. We need to check for this and return an error if it happens.
	if resp.Status == http.StatusOK {
		// check for the error message
		if strings.Contains(string(resp.Data), "error") {
			log.Printf("\n[services][deezer][CreateNewPlaylist] error - Could not update playlist. Bad request: %v\n", err)
			return nil, errors.New("bad request")
		}
	}

	if resp.Status == http.StatusInternalServerError {
		log.Printf("\n[services][deezer][CreateNewPlaylist] error - Could not create playlist. Internal server error: %v\n", err)
		return nil, errors.New("internal server error")
	}

	log.Printf("\n[services][deezer][CreateNewPlaylist] created playlist: %v\n", string(resp.Data))

	return playlistIDBytes, nil
}

// FetchUserPlaylists fetches all the playlists for a user
func (s *Service) FetchUserPlaylists(token string) (*UserPlaylistsResponse, error) {
	deezerAPIBase := os.Getenv("DEEZER_API_BASE")
	// DEEZER PLAYLIST LIMIT IS 250 FOR NOW. THIS IS ORCHDIO IMPOSED AND IT IS
	// 1. TO EASE IMPLEMENTATION
	// 2. TO MAKE IT "PREMIUM" IN THE FUTURE  (i.e. if we want to charge for more playlists), makes it easier to enforce/assimilate from now
	reqURL := fmt.Sprintf("%s/user/me/playlists?access_token=%s&limit=250", deezerAPIBase, token)
	axios.NewInstance(&axios.InstanceConfig{
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
	})

	log.Printf("\n[services][deezer][FetchUserPlaylists] request url: %v\n", reqURL)

	resp, err := axios.Get(reqURL, nil)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserPlaylists] error - Could not fetch user playlists: %v\n", err)
		return nil, err
	}

	if resp.Status == http.StatusBadRequest {
		log.Printf("\n[services][deezer][FetchUserPlaylists] error - Could not fetch user playlists. Bad request: %v\n", err)
		return nil, errors.New("bad request")
	}

	// deserialize the response body into the out response
	out := &UserPlaylistsResponse{}
	err = json.Unmarshal(resp.Data, out)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserPlaylists] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	return out, nil
}

// FetchUserArtists fetches all the artists for a user
func (s *Service) FetchUserArtists(token string) (*blueprint.UserLibraryArtists, error) {
	// DEEZER ARTIST LIMIT IS 250 FOR NOW. THIS IS ORCHDIO IMPOSED AND IT IS to make implementation easier
	// plus not as much deezer users and even so, we could make it premium in the future
	deezerApiBase := os.Getenv("DEEZER_API_BASE")
	reqURL := fmt.Sprintf("%s/user/me/artists?access_token=%s", deezerApiBase, token)
	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: deezerApiBase,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
	})

	resp, err := instance.Get(reqURL, nil)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserArtists] error - Could not fetch user artists: %v\n", err)
		return nil, err
	}

	if resp.Status == http.StatusBadRequest {
		log.Printf("\n[services][deezer][FetchUserArtists] error - Could not fetch user artists. Bad request: %v\n", err)
		return nil, err
	}

	if resp.Status >= 400 {
		log.Printf("\n[services][deezer][FetchUserArtists] error - Could not fetch user artists. Bad request: %v\n", err)
		return nil, err
	}

	var artistsResponse UserArtistsResponse
	err = json.Unmarshal(resp.Data, &artistsResponse)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserArtists] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	var artists []blueprint.UserArtist
	for _, artist := range artistsResponse.Data {
		artists = append(artists, blueprint.UserArtist{
			ID:    strconv.Itoa(artist.Id),
			Name:  artist.Name,
			Cover: artist.Picture,
			URL:   artist.Link,
		})
	}

	response := blueprint.UserLibraryArtists{
		Payload: artists,
		Total:   artistsResponse.Total,
	}
	log.Printf("\n[services][deezer][FetchUserArtists] Fetched user deezer artists: %v\n", response)
	return &response, nil
}

// FetchLibraryAlbums fetches all the deezer library albums for a user
func (s *Service) FetchLibraryAlbums(token string) ([]blueprint.LibraryAlbum, error) {
	log.Printf("\n[services][deezer][FetchLibraryAlbums] Fetching user deezer albums\n")
	deezerApiBase := os.Getenv("DEEZER_API_BASE")
	reqURL := fmt.Sprintf("%s/user/me/albums?access_token=%s", deezerApiBase, token)
	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: deezerApiBase,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
	})

	resp, err := instance.Get(reqURL, nil)
	if err != nil {
		log.Printf("\n[services][deezer][FetchLibraryAlbums] error - Could not fetch user albums: %v\n", err)
		return nil, err
	}

	if resp.Status >= 201 {
		log.Printf("\n[services][deezer][FetchLibraryAlbums] error - Could not fetch user albums. Bad request: %v\n", err)
		return nil, err
	}

	var albumsResponse UserLibraryAlbumResponse
	err = json.Unmarshal(resp.Data, &albumsResponse)
	if err != nil {
		log.Printf("\n[services][deezer][FetchLibraryAlbums] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	var albums []blueprint.LibraryAlbum
	for _, album := range albumsResponse.Data {
		albums = append(albums, blueprint.LibraryAlbum{
			ID:          strconv.Itoa(album.Id),
			Title:       album.Title,
			URL:         album.Link,
			ReleaseDate: album.ReleaseDate,
			Explicit:    album.ExplicitLyrics,
			TrackCount:  album.NbTracks,
			Artists:     []string{album.Artist.Name},
			Cover:       album.Cover,
		})
	}

	return albums, nil
}

func (s *Service) MakeRequest(url string, result interface{}) error {
	deezerApiBase := os.Getenv("DEEZER_API_BASE")
	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: deezerApiBase,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
	})
	resp, err := instance.Get(url, nil)
	if err != nil {
		log.Printf("\n[services][deezer][MakeRequest] error - Could not fetch result: %v\n", err)
		return err
	}

	if resp.Status >= 201 {
		log.Printf("\n[services][deezer][MakeRequest] error - Could not fetch result. Bad request: %v\n", err)
		return err
	}

	err = json.Unmarshal(resp.Data, &result)
	if err != nil {
		log.Printf("\n[services][deezer][MakeRequest] error - Could not deserialize the body into the out response: %v\n", err)
		return err
	}
	return nil
}

// FetchTracksListeningHistory fetches all the deezer tracks listening history for a user
func (s *Service) FetchTracksListeningHistory(token string) ([]blueprint.TrackSearchResult, error) {
	log.Printf("\n[services][deezer][FetchTracksListeningHistory] Fetching user deezer tracks listening history\n")
	link := fmt.Sprintf("user/me/history?access_token=%s", token)
	var history UserTrackListeningHistoryResponse
	err := s.MakeRequest(link, &history)
	if err != nil {
		log.Printf("\n[services][deezer][FetchTracksListeningHistory] error - Could not fetch user tracks listening history: %v\n", err)
		return nil, err
	}

	var tracks []blueprint.TrackSearchResult
	for _, track := range history.Data {

		tracks = append(tracks, blueprint.TrackSearchResult{
			URL:           track.Link,
			Artists:       []string{track.Artist.Name},
			Duration:      util.GetFormattedDuration(track.Duration),
			DurationMilli: track.Duration * 1000,
			Explicit:      track.ExplicitLyrics,
			Title:         track.Title,
			Preview:       track.Preview,
			Album:         track.Album.Title,
			ID:            strconv.Itoa(track.Id),
			Cover:         track.Album.Cover,
		})
	}

	return tracks, nil
}

// FetchUserInfo fetches all the deezer user info for a user
func (s *Service) FetchUserInfo(token string) (*blueprint.UserPlatformInfo, error) {
	log.Printf("\n[services][deezer][FetchUserInfo] Fetching user deezer info\n")
	link := fmt.Sprintf("user/me?access_token=%s", token)
	var info ProfileInfo
	err := s.MakeRequest(link, &info)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserInfo] error - Could not fetch user info: %v\n", err)
		return nil, err
	}

	log.Printf("\n[services][deezer][FetchUserInfo] Fetched user deezer info: %v\n", info)

	// fetch user's options. options are extra fields that provide information about the user but not part of
	// the user's profile information
	// https://developers.deezer.com/api/user/options
	optionsLink := fmt.Sprintf("user/me/options?access_token=%s", token)
	var options ProfileOptions
	err = s.MakeRequest(optionsLink, &options)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserInfo] error - Could not fetch user options: %v\n", err)
		return nil, err
	}

	userInfo := blueprint.UserPlatformInfo{
		Platform:        "deezer",
		Username:        info.Name,
		ProfilePicture:  info.Picture,
		ExplicitContent: util.DeezerIsExplicitContent(info.ExplicitContentLevel),
		PlatformID:      strconv.Itoa(info.Id),
		PlatformSubPlan: util.DeezerSubscriptionPlan(map[string]interface{}{
			"ads_audio":   options.AdsAudio,
			"ads_display": options.AdsDisplay,
			"streaming":   options.Streaming,
			"radio_skips": options.RadioSkips,
		}),
		Url: info.Link,
	}
	return &userInfo, nil
}
