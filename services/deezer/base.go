package deezer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"strconv"
	"strings"
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

type PlatformService interface {
	SearchPlaylistWithID(id string) (*blueprint.PlaylistSearchResult, error)
	SearchTrackWithTitle(searchData *blueprint.TrackSearchData)
}

type Service struct {
	IntegrationID     string
	IntegrationSecret string
	RedisClient       *redis.Client
	App               *blueprint.DeveloperApp
	WebhookSender     WebhookSender
}

type WebhookSender interface {
	SendTrackEvent(appID string, event *blueprint.PlaylistConversionEventTrack) bool
}

// NewService creates a new deezer service
func NewService(credentials *blueprint.IntegrationCredentials, pgClient *sqlx.DB, redisClient *redis.Client, devApp *blueprint.DeveloperApp, webhookSender WebhookSender) *Service {
	return &Service{
		IntegrationID:     credentials.AppID,
		IntegrationSecret: credentials.AppSecret,
		RedisClient:       redisClient,
		App:               devApp,
		WebhookSender:     webhookSender,
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
	//cachedKey := "deezer:track:" + info.EntityID
	cacheKey := util.FormatPlaylistTrackByCacheKeyID(IDENTIFIER, info.EntityID)

	log.Printf("\n[services][deezer][SearchTrackWithID] cachedKey %v\n", cacheKey)
	if s.RedisClient.Exists(context.Background(), cacheKey).Val() == 1 {
		log.Printf("[services][deezer][SearchTrackWithID] found cached value %v\n", cacheKey)
		cachedTrack, err := s.RedisClient.Get(context.Background(), cacheKey).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			log.Printf("[services][deezer][SearchTrackWithID] Error getting cached value %v\n", err)
			return nil, err
		}

		var deserializedTrack *blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
		if err != nil {
			log.Printf("[services][deezer][SearchTrackWithID] Error unmarshalling cached value %v\n", err)
			return nil, err
		}
		return deserializedTrack, nil
	}

	//cachedTrack, err := s.RedisClient.Get(context.Background(), cacheKey).Result()
	//if err != nil && !errors.Is(err, redis.Nil) {
	//	log.Printf("\n[services][deezer][SearchTrackWithID][SearchTrackWithID] error - Could not get cached track %v\n", err)
	//	return nil, err
	//}

	// if we have not cached this track before
	//if err != nil && errors.Is(err, redis.Nil) {
	log.Printf("\n[deezer][base][SearchTrackWithID] - warning Track has not been cached\n")
	dzSingleTrack, err := s.FetchSingleTrack(info.TargetLink)
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
	serializedTrack, err := json.Marshal(fetchedDeezerTrack)
	if err != nil {
		log.Printf("\n[controllers][platforms][deezer][ConvertPlaylist] error serializing track - %v\n", err)
	}

	// cache the result
	_ = s.RedisClient.Set(context.Background(), cacheKey, string(serializedTrack), time.Hour*24).Err()
	log.Printf("\n[platforms][base][SearchTrackWithID] Track %s has been cached\n", dzSingleTrack.Title)
	return &fetchedDeezerTrack, nil
}

// SearchTrackWithTitle searches for a track using the title (and artiste) on deezer
// This is typically expected to be used when the track we want to fetch is the one we just
// want to search on. That is, the other platforms that the user is trying to convert to.
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error) {
	cacheKey := util.FormatTargetPlaylistTrackByCacheKeyTitle(IDENTIFIER, util.NormalizeString(searchData.Artists[0]), searchData.Title)

	// get the cached track if track with title and belongs to artist has been searched before
	// we send webhook event for playlist track and return cached result.
	if s.RedisClient.Exists(context.Background(), cacheKey).Val() == 1 {
		cachedTrack, err := s.RedisClient.Get(context.Background(), cacheKey).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			log.Printf("[platforms][deezer][searchTrackWithID] Error getting cached track %v\n", err)
			return nil, err
		}

		var deserializedTrack *blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
		if err != nil {
			log.Printf("[platforms][base][SearchTrackWithID] Error deserializing cached result %v\n", err)
			return nil, err
		}

		// send track event here
		ok := s.WebhookSender.SendTrackEvent(s.App.WebhookAppID, &blueprint.PlaylistConversionEventTrack{
			EventType: blueprint.PlaylistConversionTrackEvent,
			Platform:  IDENTIFIER,
			TaskId:    searchData.Meta.PlaylistID,
			Track:     deserializedTrack,
		})

		if !ok {
			log.Printf("[platforms][base][SearchTrackWithID] Failed to send playlist conversion event to deezer\n")
			return nil, err
		}

		return deserializedTrack, nil
	}

	/* track has not been cached. we need to search for it */

	strippedTrackTitle := util.ExtractTitle(searchData.Title)
	searchTitle := strippedTrackTitle.Title
	// for deezer we'll not trim the artiste name. this is because it becomes way less accurate.
	// deezer has second to the lowest accuracy in terms of search results (youtube being the lowest)
	// however, just like others, we're caching the result under the normalized string, which contains trimmed artiste name
	// like so: "deezer-artistename-title". For example: "deezer-flatbushzombies-reelgirls
	link := fmt.Sprintf("%s/search?q=%s", os.Getenv("DEEZER_API_BASE"), url.QueryEscape(fmt.Sprintf("track:\"%s\" artist:\"%s\"", strings.Trim(searchTitle, " "), searchData.Artists[0])))

	response, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] error - Could not search the track on deezer: %v\n", err)
		return nil, err
	}
	fullTrack := FullTrack{}
	err = json.Unmarshal(response.Data, &fullTrack)
	if err != nil {
		// todo: handle Forbidden/rate limit errors.
		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] error - Could not deserialize the body into the out response, Error: \n%v, Body is: %v\n", err, string(response.Data))
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

		//newHashIdentifier := util.HashIdentifier("deezer-" + out.Artistes[0] + "-" + out.Title)
		// if the artistes are the same, the track result is most likely the same (except remixes, an artiste doesnt have two tracks with the same name)
		ok := util.CacheTrackByArtistTitle(&out, s.RedisClient, IDENTIFIER)
		if !ok {
			log.Printf("[services][deezer][SearchTrackWithTitle] error - Could not cache track %v\n", track.ID)
		}
		return &out, nil
	}

	log.Printf("\n[services][deezer][base][SearchTrackWithTitle] Deezer search for track done but no results. Searched with %s \n", link)
	return nil, nil
}
func (s *Service) fetchFromCache(ctx context.Context, track *blueprint.PlatformSearchTrack) (*blueprint.TrackSearchResult, error) {
	cleanedArtiste := fmt.Sprintf("deezer-%s-%s",
		util.NormalizeString(track.Artistes[0]),
		util.NormalizeString(track.Title))

	if s.RedisClient.Exists(ctx, cleanedArtiste).Val() != 1 {
		return nil, fmt.Errorf("track not found in cache")
	}

	var result blueprint.TrackSearchResult
	cachedResult := s.RedisClient.Get(ctx, cleanedArtiste).Val()
	if err := json.Unmarshal([]byte(cachedResult), &result); err != nil {
		return nil, fmt.Errorf("failed to deserialize cache: %w", err)
	}

	return &result, nil
}

//func (s *Service) cacheTrack(ctx context.Context, track *blueprint.PlatformSearchTrack, result *blueprint.TrackSearchResult) error {
//	cleanedArtiste := fmt.Sprintf("deezer-%s-%s",
//		util.NormalizeString(track.Artistes[0]),
//		util.NormalizeString(track.Title))
//
//	data, err := json.Marshal(result)
//	if err != nil {
//		return fmt.Errorf("failed to serialize track: %w", err)
//	}
//
//	return s.RedisClient.Set(ctx, cleanedArtiste, data, 24*time.Hour).Err()
//}

// SearchPlaylistWithID fetches tracks under a playlist on deezer with pagination
func (s *Service) SearchPlaylistWithID(id string) (*blueprint.PlaylistSearchResult, error) {
	log.Printf("\n[services][deezer][SearchPlaylistWithID] Fetching playlist %v\n", id)
	// todo: implement fetching more pages. test if this covers cases with more than 100, 250, 500, 1000 tracks.
	infoLink := "https://api.deezer.com/playlist/" + id + "?limit=1"
	var playlistInfo PlaylistTracksSearch
	err := s.MakeRequest(infoLink, &playlistInfo)
	if err != nil {
		log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not fetch playlist info: %v\n", err)
		return nil, err
	}

	tracks, gErr := axios.Get("https://api.deezer.com/playlist/" + id)
	if gErr != nil {
		log.Printf("[services][deezer][SearchPlaylistWithID] error - Could not fetch playlist info — Axio error: %v\n", err)
		return nil, gErr
	}

	cachedSnapshot, cacheErr := s.RedisClient.Get(context.Background(), util.FormatPlatformConversionCacheKey(id, IDENTIFIER)).Result()
	if cacheErr != nil && !errors.Is(cacheErr, redis.Nil) {
		log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not get cached snapshot for playlist %v\n", id)
		return nil, cacheErr
	}

	cachedSnapshotID, idErr := s.RedisClient.Get(context.Background(), util.FormatPlatformPlaylistSnapshotID(IDENTIFIER, id)).Result()
	if idErr != nil && !errors.Is(idErr, redis.Nil) {
		log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not get cached snapshot id for playlist %v\n", id)
		return nil, idErr
	}

	// if we have not cached this track or the snapshot has changed (that is, the playlist has been updated), then
	// we need to fetch the tracks and cache them
	if cachedSnapshotID != playlistInfo.Checksum {
		if tracks != nil {
			var trackList PlaylistTracksSearch
			err = json.Unmarshal(tracks.Data, &trackList)
			if err != nil {
				log.Println("Error deserializing result of playlist tracks search")
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
				ok := util.CacheTrackByID(result, s.RedisClient, IDENTIFIER)
				if !ok {
					log.Printf("[services][deezer][SearchPlaylistWithID] error - Could not fetch playlist info: %v\n", err)
				}
				svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)

				ok = svixInstance.SendTrackEvent(s.App.WebhookAppID, &blueprint.PlaylistConversionEventTrack{
					EventType: blueprint.PlaylistConversionTrackEvent,
					Platform:  IDENTIFIER,
					TaskId:    id,
					Track:     result,
				})

				if !ok {
					log.Printf("[services][spotify][base][SearchPlaylistWithID] error - could not send playlist track conversion event: %v\n", err)
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
				ID:     strconv.Itoa(int(trackList.Id)),
			}

			// update the snapshotID cache
			err = s.RedisClient.Set(context.Background(), util.FormatPlatformPlaylistSnapshotID(IDENTIFIER, id), trackList.Checksum, 0).Err()
			if err != nil {
				log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not cache snapshot id: %v\n", err)
			} else {
				log.Printf("\n[services][deezer][SearchPlaylistWithID] cached snapshot id: %v\n", trackList.Checksum)
			}

			// cache the whole playlist
			serializedPlaylist, mErr := json.Marshal(&reply)
			if mErr != nil {
				log.Printf("[services][deezer][SearchPlaylistWithID] error - Could not serialize playlist: %v\n", err)
				return nil, err
			}

			if err != nil {
				log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not serialize playlist: %v\n", err)
			}

			err = s.RedisClient.Set(context.Background(), util.FormatPlatformConversionCacheKey(id, IDENTIFIER), string(serializedPlaylist), 0).Err()
			if err != nil {
				log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not cache playlist: %v\n", err)
			} else {
				log.Printf("\n[services][deezer][SearchPlaylistWithID] cached playlist: %v %v %v\n", reply.Title, reply.URL, reply.Length)
			}

			// cache the checksum (snapshot id)
			err = s.RedisClient.Set(context.Background(), util.FormatPlatformPlaylistSnapshotID(IDENTIFIER, id), trackList.Checksum, 0).Err()
			if err != nil {
				log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not cache snapshot id: %v\n", err)
			} else {
				log.Printf("\n[services][deezer][SearchPlaylistWithID] cached snapshot id: %v\n", trackList.Checksum)
			}

			return &reply, nil
		}
		return nil, nil
	}

	playlistResult := &blueprint.PlaylistSearchResult{}
	err = json.Unmarshal([]byte(cachedSnapshot), playlistResult)
	if err != nil {
		log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	// send webhook event here
	svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	whResponse, whErr := svixInstance.SendEvent(s.App.WebhookAppID, blueprint.PlaylistConversionTrackEvent, playlistResult)

	if whErr != nil {
		log.Printf("\n[services][deezer][SearchPlaylistWithID] error - Could not send webhook event: %v\n", whErr)
	}

	if whResponse != nil && whErr == nil {
		// for debugging only for now
		log.Printf("[services][deezer][SearchTrackWithTitle] Webhook response: %v\n", whResponse)
	}
	return playlistResult, nil
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
		log.Printf("\n[services][deezer][CreateNewPlaylist] error - Could not create playlist: %v\n", err)
		return nil, err
	}

	if resp.Status == http.StatusBadRequest {
		log.Printf("\n[services][deezer][CreateNewPlaylist] error - Could not create playlist. Bad request: %v\n", err)
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

	out := &UserPlaylistsResponse{}
	err := s.MakeRequest(reqURL, out)
	if err != nil {
		log.Printf("\n[services][deezer][FetchUserPlaylists] error - Could not fetch user playlists: %v\n", err)
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
	var artistsResponse UserArtistsResponse
	err := s.MakeRequest(reqURL, &artistsResponse)

	if err != nil {
		log.Printf("\n[services][deezer][FetchUserArtists] error - Could not fetch user artists: %v\n", err)
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
	return &response, nil
}

// FetchLibraryAlbums fetches all the deezer library albums for a user
func (s *Service) FetchLibraryAlbums(token string) ([]blueprint.LibraryAlbum, error) {
	log.Printf("\n[services][deezer][FetchLibraryAlbums] Fetching user deezer albums\n")
	deezerApiBase := os.Getenv("DEEZER_API_BASE")
	reqURL := fmt.Sprintf("%s/user/me/albums?access_token=%s", deezerApiBase, token)
	var albumsResponse UserLibraryAlbumResponse

	err := s.MakeRequest(reqURL, &albumsResponse)
	if err != nil {
		log.Printf("\n[services][deezer][FetchLibraryAlbums] error - Could not fetch user albums: %v\n", err)
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
		Platform:        IDENTIFIER,
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
