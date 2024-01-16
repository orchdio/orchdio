package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"github.com/minchao/go-apple-music"
	"github.com/samber/lo"
	"github.com/vicanso/go-axios"
	"go.uber.org/zap"
	"log"
	"net/http"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	"os"
	"strings"
	"sync"
	"time"
)

type Service struct {
	IntegrationTeamID string
	IntegrationKeyID  string
	IntegrationAPIKey string
	RedisClient       *redis.Client
	PgClient          *sqlx.DB
	Logger            *zap.Logger
}

func NewService(credentials *blueprint.IntegrationCredentials, pgClient *sqlx.DB, redisClient *redis.Client, logger *zap.Logger) *Service {
	return &Service{
		// this is the equivalent of the team id
		IntegrationTeamID: credentials.AppID,
		// this is equivalent to the key id
		IntegrationKeyID: credentials.AppSecret,
		// this is equivalent to the apple api key
		IntegrationAPIKey: credentials.AppRefreshToken,
		RedisClient:       redisClient,
		PgClient:          pgClient,
		Logger:            logger,
	}
}

// SearchTrackWithID fetches a track from the ID using the link.
func (s *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	cacheKey := "applemusic:track:" + info.EntityID
	_, err := s.RedisClient.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithLink] Error fetching track from cache", zap.Error(err))
		return nil, err
	}

	tp := applemusic.Transport{Token: s.IntegrationAPIKey}
	client := applemusic.NewClient(tp.Client())
	tracks, response, err := client.Catalog.GetSong(context.Background(), "us", info.EntityID, nil)
	if err != nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music. Unknown error.", zap.Error(err))
		return nil, err
	}
	if response.StatusCode != 200 {
		s.Logger.Error("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music. Error response not 200", zap.Error(err), zap.String("status_code", fmt.Sprintf("%d", response.StatusCode)))
		return nil, err
	}

	if len(tracks.Data) == 0 {
		s.Logger.Warn("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music. Track length is 0", zap.Error(err))
		return nil, blueprint.ENORESULT
	}

	previewURL := ""
	t := tracks.Data[0]
	previews := *t.Attributes.Previews
	if len(previews) > 0 {
		previewURL = previews[0].Url
	}
	//if t.Attributes.Previews != nil {
	//
	//}

	// replace the cover url with the 150x150 version using regex. the original url is in the format of https://is5-ssl.mzstatic.com/image/thumb/Music124/v4/f8/0d/17/f80d17a1-c1c8-1f3d-6797-8d7e9a98539b/8720205201379.png/{w}x{h}bb.jpg
	// where {w} and {h} are the width and height of the image. we replace it with 150x150
	coverURL := strings.ReplaceAll(t.Attributes.Artwork.URL, "{w}x{h}bb.jpg", "150x150bb.jpg")

	strippedTitleInfo := util.ExtractTitle(t.Attributes.Name)
	artistes := []string{t.Attributes.ArtistName}
	if len(strippedTitleInfo.Artists) > 0 {
		artistes = append(artistes, strippedTitleInfo.Artists...)
	}

	track := &blueprint.TrackSearchResult{
		URL:           info.TargetLink,
		Artists:       lo.Uniq(artistes),
		Released:      t.Attributes.ReleaseDate,
		Duration:      util.GetFormattedDuration(int(t.Attributes.DurationInMillis / 1000)),
		DurationMilli: int(t.Attributes.DurationInMillis),
		Explicit:      false,
		Title:         t.Attributes.Name,
		Preview:       previewURL,
		Album:         t.Attributes.AlbumName,
		ID:            t.Id,
		Cover:         coverURL,
	}

	serializeTrack, err := json.Marshal(track)
	if err != nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithLink] Error serializing track", zap.Error(err))
		return nil, err
	}
	err = s.RedisClient.Set(context.Background(), cacheKey, serializeTrack, time.Hour*24).Err()
	if err != nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithLink] Error caching track", zap.Error(err))
		return nil, err
	}
	return track, nil

}

// SearchTrackWithTitle searches for a track using the query.
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error) {
	strippedTitleInfo := util.ExtractTitle(searchData.Title)
	// if the title is in the format of "title (feat. artiste)" then we search for the title without the feat. artiste
	//log.Printf("Apple music: Searching with stripped artiste: %s. Original artiste: %s", strippedTitleInfo.Title, artiste)
	if s.RedisClient.Exists(context.Background(), searchData.Artists[0]).Val() == 1 {
		s.Logger.Info("[services][applemusic][SearchTrackWithTitle] Track found in cache", zap.String("artiste", searchData.Artists[0]), zap.String("title", strippedTitleInfo.Title))
		track, err := s.RedisClient.Get(context.Background(), util.NormalizeString(searchData.Artists[0])).Result()
		if err != nil {
			s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error fetching track from cache", zap.Error(err))
			return nil, err
		}
		var result *blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(track), &result)
		if err != nil {
			s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error unmarshalling track from cache", zap.Error(err))
			return nil, err
		}
		return result, nil
	}

	s.Logger.Info("[services][applemusic][SearchTrackWithTitle] Track not found in cache, fetching track from Apple Music", zap.Strings("artiste", searchData.Artists), zap.String("title", strippedTitleInfo.Title))

	// todo: throw an error when the api key is empty. ensure that this might be the case. if not, remove the commented out code.
	//if s.IntegrationAPIKey == "" {
	//	log.Printf("[services][applemusic][SearchTrackWithTitle] Apple music API key is empty on decoded credentials\n")
	//} else {
	//	log.Printf("[services][applemusic][SearchTrackWithTitle] Apple music API key is not empty on decoded credentials\n")
	//}

	// to check if the problem is with the apple music library not allowing us pass "with" to improve the search results,
	// first lets try to get the hint and see if that gives more sensible results or precursor to results
	hintURL := fmt.Sprintf("https://api.music.apple.com/v1/catalog/us/search/hints?term=%s&limit=10&types=songs", url.QueryEscape(
		strings.ReplaceAll(strings.ToLower(strippedTitleInfo.Title), " ", "+")))
	axiosConfig := &axios.InstanceConfig{
		Headers: map[string][]string{
			"Authorization": {"Bearer " + s.IntegrationAPIKey},
		},
	}
	axiosClient := axios.NewInstance(axiosConfig)
	hintResponse, err := axiosClient.Get(hintURL, nil)
	if err != nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error fetching hint from Apple Music", zap.Error(err))
		return nil, err
	}

	if hintResponse.Status != http.StatusOK {
		s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error fetching hint from Apple Music", zap.Error(err))
		return nil, err
	}

	tp := applemusic.Transport{Token: s.IntegrationAPIKey}
	searchTerm := fmt.Sprintf("%s %s", searchData.Artists[0], strippedTitleInfo.Title)
	hintResponseStruct := struct {
		Results struct {
			Terms []string `json:"terms"`
		}
	}{}
	err = json.Unmarshal(hintResponse.Data, &hintResponseStruct)
	if err != nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error unmarshalling hint response", zap.Error(err))
		return nil, err
	}

	if len(hintResponseStruct.Results.Terms) > 0 {
		// find the first term that contains the artiste
		for _, term := range hintResponseStruct.Results.Terms {
			allArtists := strings.ReplaceAll(strings.ToLower(term), " ", ",")
			joinedArtists := strings.Split(allArtists, ",")
			if lo.Contains(joinedArtists, strings.ToLower(searchData.Artists[0])) {
				searchTerm = term
				break
			}
		}
	}
	client := applemusic.NewClient(tp.Client())

	results, response, err := client.Catalog.Search(context.Background(), "us", &applemusic.SearchOptions{
		Term:  searchTerm,
		Types: "songs",
		Limit: 10,
	})

	if err != nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Could not fetch search response. Unknown Error.", zap.Error(err))
		return nil, err
	}

	if response.StatusCode != 200 {
		s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music. Could not fetch search response. Bad status code", zap.Error(err),
			zap.Int("status_code", response.StatusCode))
		return nil, err
	}

	if results.Results.Songs == nil {
		s.Logger.Warn("[services][applemusic][SearchTrackWithTitle] No result found for track", zap.String("title", strippedTitleInfo.Title), zap.Strings("artists", searchData.Artists))
		return nil, blueprint.ENORESULT
	}

	if len(results.Results.Songs.Data) == 0 {
		s.Logger.Warn("[services][applemusic][SearchTrackWithTitle] No result found for track", zap.String("title", strippedTitleInfo.Title), zap.Strings("artists", searchData.Artists))
		return nil, blueprint.ENORESULT
	}

	t := results.Results.Songs.Data[0]
	previewURL := ""
	previews := *t.Attributes.Previews
	if len(previews) > 0 {
		previewURL = previews[0].Url
	}

	coverURL := strings.ReplaceAll(t.Attributes.Artwork.URL, "{w}x{h}bb.jpg", "150x150bb.jpg")

	artistes := []string{t.Attributes.ArtistName}
	if len(strippedTitleInfo.Artists) > 0 {
		artistes = append(artistes, strippedTitleInfo.Artists...)
	}
	track := &blueprint.TrackSearchResult{
		Artists:       artistes,
		Released:      t.Attributes.ReleaseDate,
		Duration:      util.GetFormattedDuration(int(t.Attributes.DurationInMillis / 1000)),
		DurationMilli: int(t.Attributes.DurationInMillis),
		Explicit:      false, // apple doesnt seem to return explicit content value for songs
		Title:         t.Attributes.Name,
		Preview:       previewURL,
		Album:         t.Attributes.AlbumName,
		ID:            t.Id,
		Cover:         coverURL,
		URL:           t.Attributes.URL,
	}
	serializedTrack, err := json.Marshal(track)
	if err != nil {
		s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error serializing track. Could not parse final result.", zap.Error(err))
		return nil, err
	}

	if lo.Contains(track.Artists, searchData.Artists[0]) {
		err = s.RedisClient.Set(context.Background(), fmt.Sprintf("applemusic:%s:%s", util.NormalizeString(searchData.Artists[0]), strippedTitleInfo.Title), string(serializedTrack), time.Hour*24).Err()
		if err != nil {
			s.Logger.Error("[services][applemusic][SearchTrackWithTitle] Error caching track", zap.Error(err))
			return nil, err
		} else {
			s.Logger.Info("[services][applemusic][SearchTrackWithTitle] Track has been cached", zap.String("title", track.Title))
		}
	}

	return track, nil
}

// SearchTrackWithTitleChan searches for tracks using title and artistes but do so asynchronously.
func (s *Service) SearchTrackWithTitleChan(searchData *blueprint.TrackSearchData, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup) {
	track, err := s.SearchTrackWithTitle(searchData)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitleChan] Error fetching track: %v\n", err)
		defer wg.Done()
		c <- nil
		wg.Add(1)
		return
	}
	defer wg.Done()
	c <- track
	wg.Add(1)
	return
}

// FetchTracks asynchronously fetches a list of tracks using the track id
func (s *Service) FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client, appId string) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks, error) {
	var omittedTracks []blueprint.OmittedTracks
	var results []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		identifier := fmt.Sprintf("applemusic-%s-%s", track.Artistes[0], track.Title)

		// fetching from cache and returning after deserializing
		if red.Exists(context.Background(), identifier).Val() == 1 {
			var deserializedTrack *blueprint.TrackSearchResult
			track, err := red.Get(context.Background(), identifier).Result()
			if err != nil {
				s.Logger.Warn("[services][applemusic][FetchTracks] Error fetching track from cache", zap.Error(err))
				return nil, nil, err
			}
			err = json.Unmarshal([]byte(track), &deserializedTrack)
			if err != nil {
				s.Logger.Error("[services][applemusic][FetchTracks] Error unmarshalling track from cache", zap.Error(err))
				return nil, nil, err
			}
			results = append(results, *deserializedTrack)
			continue
		}

		searchData := &blueprint.TrackSearchData{
			Title:   track.Title,
			Artists: track.Artistes,
			Album:   track.Album,
		}
		// async goes brrrr
		go s.SearchTrackWithTitleChan(searchData, ch, &wg)
		chTracks := <-ch
		if chTracks == nil {
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Artistes: []string{track.Artistes[0]},
				Title:    track.Title,
			})
			continue
		}

		// send the webhook even to the endpoint
		// get the

		results = append(results, *chTracks)
	}
	wg.Wait()
	return &results, &omittedTracks, nil
}

// SearchPlaylistWithID fetches a list of tracks for a playlist and saves the last modified date to redis
func (s *Service) SearchPlaylistWithID(id, webhookId, taskId string) (*blueprint.PlaylistSearchResult, error) {
	tp := applemusic.Transport{Token: s.IntegrationAPIKey}
	client := applemusic.NewClient(tp.Client())
	playlist := &blueprint.PlaylistSearchResult{}
	duration := 0

	var tracks []blueprint.TrackSearchResult
	s.Logger.Info("[services][applemusic][SearchPlaylistWithID] Fetching playlist tracks", zap.String("id", id))
	playlistId := strings.ReplaceAll(id, "/", "")
	results, response, err := client.Catalog.GetPlaylist(context.Background(), "us", playlistId, nil)

	if err != nil {
		s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks", zap.Error(err))
		return nil, err
	}

	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][SearchPlaylistWithID][GetPlaylist] Status - %v could not fetch playlist tracks: %v\n", response.StatusCode, err)
		s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks. Response code not 200", zap.Error(err),
			zap.Int("status_code", response.StatusCode))
		return nil, blueprint.EUNKNOWN
	}

	if len(results.Data) == 0 {
		s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks. Result data is empty", zap.Error(err))
		return nil, err
	}

	playlistData := results.Data[0]

	for _, t := range playlistData.Relationships.Tracks.Data {
		tr, err := t.Parse()
		track := tr.(*applemusic.Song)
		if err != nil {
			s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error parsing track", zap.Error(err))
			return nil, err
		}

		previewURL := ""
		var previewStruct []struct {
			Url string `json:"url"`
		}

		trackAttr := track.Attributes
		duration += int(track.Attributes.DurationInMillis)
		tribute := trackAttr.Previews

		if tribute != nil {
			r, mErr := json.Marshal(tribute)
			if mErr != nil {
				s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error serializing preview url", zap.Error(mErr))
				return nil, mErr
			}
			jErr := json.Unmarshal(r, &previewStruct)
			if jErr != nil {
				s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error deserializing preview url", zap.Error(jErr))
				return nil, jErr
			}
			previewURL = previewStruct[0].Url
		}

		cover := ""
		if playlistData.Attributes.Artwork != nil {
			cover = strings.ReplaceAll(playlistData.Attributes.Artwork.URL, "{w}x{h}bb.jpg", "300x300bb.jpg")
		}

		tracks = append(tracks, blueprint.TrackSearchResult{
			URL:           trackAttr.URL,
			Artists:       []string{trackAttr.ArtistName},
			Released:      trackAttr.ReleaseDate,
			Duration:      util.GetFormattedDuration(int(trackAttr.DurationInMillis / 1000)),
			DurationMilli: int(trackAttr.DurationInMillis),
			Explicit:      false,
			Title:         trackAttr.Name,
			Preview:       previewURL,
			Album:         trackAttr.AlbumName,
			ID:            track.Id,
			Cover:         cover,
		})
	}
	playlistCover := ""
	if playlistData.Attributes.Artwork != nil {
		playlistCover = strings.ReplaceAll(playlistData.Attributes.Artwork.URL, "{w}x{h}cc.jpg", "300x300cc.jpg")
	}

	// in order to add support for paginated playlists, we need to somehow get the total number of tracks in the playlist and or get the next page of tracks
	// if there is next page, we keep fetching (in a loop) until we get all the tracks and break out of the loop. This is similar to how we fetch paginated playlists
	// for spotify. However, the difference is that the library we use for Apple Music does not support pagination. So in order to get paginated tracks, for now, we first make
	// a GetPlaylist call to get playlist tracks (and data) and first 100 tracks (if playlist is more than 100 tracks). Then we make another call that fetches the tracks for the playlists, offset by 100 or length of the first call.
	// but this time we specify the limit of 700. Downside is that we may not have playlists that have less than 300 tracks. THIS IS AN EDGE/UNLIKELY CASE. GARDEN THIS FOR NOW.
	// TODO: refactor this when patch to library is implemented and merged (or switch to implementation in our fork) until the PR from the fork is merged.
	p := url.Values{}
	p.Add("limit", "300")
	p.Add("offset", fmt.Sprintf("%d", len(playlistData.Relationships.Tracks.Data)))
	ax := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://api.music.apple.com",
		Headers: map[string][]string{
			"Authorization": {fmt.Sprintf("Bearer %s", s.IntegrationAPIKey)},
		},
	})

	_allTracksRes, tErr := ax.Get(fmt.Sprintf("/v1/catalog/us/playlists/%s/tracks", playlistId), p)
	if tErr != nil {
		s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks from apple music", zap.Error(tErr))
		return nil, tErr
	}

	// if the response is a 404 and the length of the tracks we got earlier is 0, that means we really cant get the playlist tracks
	if _allTracksRes.Status == 404 {
		if len(tracks) == 0 {
			s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Could not fetch playlist tracks. Response is 404 and length of result is 0.", zap.Error(err))
			return nil, blueprint.EUNKNOWN
		}
		result := blueprint.PlaylistSearchResult{
			Title:   playlistData.Attributes.Name,
			Tracks:  tracks,
			URL:     playlistData.Attributes.URL,
			Length:  util.GetFormattedDuration(duration / 1000),
			Preview: "",
			Owner:   playlistData.Attributes.CuratorName,
			Cover:   playlistCover,
		}
		return &result, nil
	}

	if _allTracksRes.Status != 200 {
		s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks. Response code not 200", zap.Error(err),
			zap.Int("status_code", _allTracksRes.Status))
		return nil, err
	}

	var allTracksRes UnlimitedPlaylist
	err = json.Unmarshal(_allTracksRes.Data, &allTracksRes)

	if err != nil {
		s.Logger.Error("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks", zap.Error(err))
		return nil, err
	}

	for _, d := range allTracksRes.Data {
		singleTrack := d
		previewURL := ""
		tribute := d.Attributes.Previews
		if len(tribute) > 0 {
			previewURL = tribute[0].Url
		}
		duration += d.Attributes.DurationInMillis
		coverURL := ""
		if singleTrack.Attributes.Artwork.Height > 0 {
			coverURL = singleTrack.Attributes.Artwork.Url
		}

		coverURL = strings.ReplaceAll(singleTrack.Attributes.Artwork.Url, "{w}x{h}bb.jpg", "300x300bb.jpg")
		track := &blueprint.TrackSearchResult{
			URL:           singleTrack.Attributes.Url,
			Artists:       []string{singleTrack.Attributes.ArtistName},
			Released:      singleTrack.Attributes.ReleaseDate,
			Duration:      util.GetFormattedDuration(singleTrack.Attributes.DurationInMillis / 1000),
			DurationMilli: singleTrack.Attributes.DurationInMillis,
			Explicit:      false,
			Title:         singleTrack.Attributes.Name,
			Preview:       previewURL,
			Album:         singleTrack.Attributes.AlbumName,
			ID:            singleTrack.Id,
			Cover:         coverURL,
		}
		tracks = append(tracks, *track)
	}

	// save the last updated at to redis under the key "applemusic:playlist:<id>"
	err = s.RedisClient.Set(context.Background(), fmt.Sprintf("applemusic:playlist:%s", id), playlistData.Attributes.LastModifiedDate, 0).Err()
	if err != nil {
		s.Logger.Warn("[services][applemusic][SearchPlaylistWithID] Error setting cache last updated at", zap.Error(err))
		return nil, err
	}

	playlist = &blueprint.PlaylistSearchResult{
		Title:   playlistData.Attributes.Name,
		Tracks:  tracks,
		URL:     playlistData.Attributes.URL,
		Length:  util.GetFormattedDuration(duration / 1000),
		Preview: "",
		Owner:   playlistData.Attributes.CuratorName,
		Cover:   playlistCover,
	}

	s.Logger.Info("[services][applemusic][SearchPlaylistWithID] Done fetching playlist tracks", zap.String("title", playlist.Title))
	return playlist, nil
}

// SearchPlaylistWithTracks fetches the tracks for a playlist based on the search result
// from another platform
func (s *Service) SearchPlaylistWithTracks(p *blueprint.PlaylistSearchResult, webhookId, taskId string) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Title:    track.Title,
			Artistes: track.Artists,
		})
	}
	tracks, omittedTracks, err := s.FetchTracks(trackSearch, s.RedisClient, webhookId)
	if err != nil {
		s.Logger.Error("[services][applemusic][FetchPlaylistTrackResultsSearchPlaylistTracks] Error fetching tracks", zap.Error(err))
		return nil, nil
	}
	return tracks, omittedTracks
}

func (s *Service) CreateNewPlaylist(title, description, musicToken string, tracks []string) ([]byte, error) {
	s.Logger.Info("[services][applemusic][CreateNewPlaylist] Creating new playlist", zap.String("title", title))
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY"), MusicUserToken: musicToken}

	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("[services][applemusic][CreateNewPlaylist] Error creating playlist. Recovered from panic", zap.Any("error", r))
		}
	}()

	client := applemusic.NewClient(tp.Client())
	defer func() {
		if err := recover(); err != nil {
			s.Logger.Error("[services][applemusic][CreateNewPlaylist] Error creating playlist. Recovered from panic: CreateLibraryPlaylist", zap.Any("error", err))
			return
		}
	}()
	playlist, response, err := client.Me.CreateLibraryPlaylist(context.Background(), applemusic.CreateLibraryPlaylist{
		Attributes: applemusic.CreateLibraryPlaylistAttributes{
			Name:        title,
			Description: description,
		},
		Relationships: nil,
	}, nil)

	if err != nil {
		s.Logger.Error("[services][applemusic][CreateNewPlaylist][error] Could not add playlist to user library.", zap.Error(err))
	}

	if response.Response.StatusCode == 403 {
		s.Logger.Error("[services][applemusic][CreateNewPlaylist][error] - unauthorized", zap.Error(err))
		return nil, blueprint.EFORBIDDEN
	}

	if response.Response.StatusCode == 401 {
		s.Logger.Error("[services][applemusic][CreateNewPlaylist][error] - unauthorized", zap.Error(err))
		return nil, blueprint.EUNAUTHORIZED
	}

	if response.Response.StatusCode == 400 {
		s.Logger.Error("[services][applemusic][CreateNewPlaylist][error] - bad request", zap.Error(err))
		return nil, blueprint.EBADREQUEST
	}

	// add the tracks to the playlist
	var playlistTracks []applemusic.CreateLibraryPlaylistTrack
	for _, track := range tracks {
		playlistTracks = append(playlistTracks, applemusic.CreateLibraryPlaylistTrack{
			Id:   track,
			Type: "songs",
		})
	}

	playlistData := applemusic.CreateLibraryPlaylistTrackData{
		Data: playlistTracks,
	}
	response, err = client.Me.AddLibraryTracksToPlaylist(context.Background(), playlist.Data[0].Id, playlistData)
	if err != nil {
		s.Logger.Error("[services][applemusic][CreateNewPlaylist][error] - could not add tracks to playlist", zap.Error(err))
		return nil, err
	}

	if response.StatusCode >= 400 {
		s.Logger.Error("[services][applemusic][CreateNewPlaylist][error] - could not add tracks to playlist", zap.Error(err))
		return nil, err
	}

	log.Printf("[services][applemusic][CreateNewPlaylist] Successfully created playlist: %v\n", playlist.Data[0].Href)
	s.Logger.Info("[services][applemusic][CreateNewPlaylist] Successfully created playlist.", zap.String("title", title),
		zap.String("link", playlist.Data[0].Href))
	return []byte(fmt.Sprintf("https://music.apple.com/us/playlist/%s", playlist.Data[0].Id)), nil
}

// FetchUserPlaylists fetches the user's playlists
func (s *Service) FetchUserPlaylists(token string) ([]UserPlaylistResponse, error) {
	s.Logger.Info("[services][applemusic][FetchUserPlaylists] Fetching user playlists")
	tp := applemusic.Transport{Token: s.IntegrationAPIKey, MusicUserToken: token}
	client := applemusic.NewClient(tp.Client())
	// get the user's playlists
	p, _, err := client.Me.GetAllLibraryPlaylists(context.Background(), &applemusic.PageOptions{
		Limit: 100,
	})
	if err != nil {
		s.Logger.Error("[services][applemusic][FetchUserPlaylists] Error getting user playlists", zap.Error(err))
		return nil, err
	}
	for {
		if p.Next == "" {
			break
		}
		pr, _, mErr := client.Me.GetAllLibraryPlaylists(context.Background(), &applemusic.PageOptions{
			Offset: len(p.Data),
		})
		if err != nil {
			s.Logger.Error("[services][applemusic][FetchUserPlaylists] Error getting user playlists", zap.Error(mErr))
			return nil, mErr
		}

		if len(pr.Data) == 0 {
			log.Printf("[services][applemusic][FetchUserPlaylists] Fetched all user playlist data\n")
			s.Logger.Info("[services][applemusic][FetchUserPlaylists] Fetched all user playlist data")
			break
		}
		p.Next = pr.Next
		p.Data = append(p.Data, pr.Data...)
	}

	var playlists []UserPlaylistResponse
	// get each of the playlist information for all the playlists in the user's library
	for _, playlist := range p.Data {
		s.Logger.Info("[services][applemusic][FetchUserPlaylists] Getting catalog info for", zap.String("title", playlist.Attributes.Name))
		//playlistIds = append(playlistIds, playlist.Id)
		inst := axios.NewInstance(&axios.InstanceConfig{
			BaseURL: "https://api.music.apple.com/v1",
			Headers: http.Header{
				"Authorization":         []string{fmt.Sprintf("Bearer %s", s.IntegrationAPIKey)},
				"Music-User-MusicToken": []string{token},
			},
		})
		link := fmt.Sprintf("/me/library/playlists/%s/catalog", playlist.Id)
		resp, sErr := inst.Get(link, nil)
		if sErr != nil {
			s.Logger.Error("[services][applemusic][FetchUserPlaylists] Error getting user playlist catalog info", zap.Error(sErr))
			return nil, sErr
		}

		var info PlaylistCatalogInfoResponse
		err = json.Unmarshal(resp.Data, &info)
		if err != nil {
			s.Logger.Error("[services][applemusic][FetchUserPlaylists] Error getting deserializing playlist catalog info", zap.Error(err))
			return nil, err
		}
		data := info.Data[0]

		// get the playlist info itself
		playlistResponse, gErr := inst.Get(fmt.Sprintf("/me/library/playlists/%s", playlist.Id), nil)
		if gErr != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting user playlist info: %v\n", gErr)
			return nil, gErr
		}
		playlistInfo := &PlaylistInfoResponse{}
		err = json.Unmarshal(playlistResponse.Data, playlistInfo)
		if err != nil {
			s.Logger.Error("[services][applemusic][FetchUserPlaylists] Error getting deserializing playlist info", zap.Error(err))
			return nil, err
		}

		// get playlist tracks
		playlistTracksResponse, gErr2 := inst.Get(fmt.Sprintf("/me/library/playlists/%s/tracks", playlist.Id), nil)
		if gErr2 != nil {
			s.Logger.Error("[services][applemusic][FetchUserPlaylists] Error getting user playlist tracks", zap.Error(gErr2))
			return nil, gErr2
		}
		playlistTracks := &PlaylistTracksResponse{}
		err = json.Unmarshal(playlistTracksResponse.Data, playlistTracks)
		if err != nil {
			s.Logger.Error("[services][applemusic][FetchUserPlaylists] Error getting deserializing playlist tracks", zap.Error(err))
			return nil, err
		}

		r := UserPlaylistResponse{
			ID:            data.ID,
			Title:         data.Attributes.Name,
			Public:        playlistInfo.Data[0].Attributes.IsPublic,
			Description:   playlistInfo.Data[0].Attributes.Description.Standard,
			Collaborative: playlistInfo.Data[0].Attributes.CanEdit,
			Cover:         strings.ReplaceAll(data.Attributes.Artwork.URL, "{w}x{h}bb", "300x300bb"),
			CreatedAt:     playlistInfo.Data[0].Attributes.DateAdded.String(),
			Owner:         data.Attributes.CuratorName,
			NbTracks:      playlistTracks.Meta.Total,
			URL:           info.Data[0].Attributes.Url,
		}
		playlists = append(playlists, r)
	}
	return playlists, nil
}

// FetchUserArtists fetches the user's artists
func (s *Service) FetchUserArtists(token string) (*blueprint.UserLibraryArtists, error) {
	// get the user's artists
	inst := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://api.music.apple.com/v1",
		Headers: http.Header{
			"Authorization":         []string{fmt.Sprintf("Bearer %s", s.IntegrationAPIKey)},
			"Music-User-MusicToken": []string{token},
		},
	})

	// first, fetch all artists. the limit is 100 and since we want to fetch all of them, we need to loop
	// TODO: change to pagination instead of looping if/when the need arises
	resp, err := inst.Get("/me/library/artists?limit=100", nil)
	if err != nil {
		s.Logger.Error("[services][applemusic][FetchUserArtists] Error getting user's library artists.", zap.Error(err))
		return nil, err
	}

	var artists UserArtistsResponse
	err = json.Unmarshal(resp.Data, &artists)
	if err != nil {
		s.Logger.Error("[services][applemusic][FetchUserArtists] Error deserializing user artists.", zap.Error(err))
		return nil, err
	}

	// fetch the remaining artists
	if artists.Meta.Total > len(artists.Data) {
		for {
			if artists.Next == "" {
				break
			}
			moreResp, gErr := inst.Get(fmt.Sprintf("/me/library/artists?limit=100&offset=%d", len(artists.Data)), nil)
			if gErr != nil {
				s.Logger.Error("[services][applemusic][FetchUserArtists] Error getting user artists.", zap.Error(gErr))
				return nil, gErr
			}
			var remainingArtists UserArtistsResponse
			err = json.Unmarshal(moreResp.Data, &remainingArtists)
			if err != nil {
				s.Logger.Error("[services][applemusic][FetchUserArtists] Error deserializing user artists.", zap.Error(err))
				return nil, err
			}
			artists.Data = append(artists.Data, remainingArtists.Data...)
			if len(remainingArtists.Data) == 0 {
				s.Logger.Warn("[services][applemusic][FetchUserArtists] No more artists to fetch")
				break
			}
			artists.Next = remainingArtists.Next
			artists.Data = append(artists.Data, remainingArtists.Data...)
		}
	}

	var userArtists []blueprint.UserArtist
	for _, a := range artists.Data {
		// get the artist info itself
		artistResponse, gErr := inst.Get(fmt.Sprintf("/me/library/artists/%s/catalog", a.Id), nil)
		if gErr != nil {
			s.Logger.Error("[services][applemusic][FetchUserArtists] Error getting user artist info.", zap.Error(gErr))
			return nil, gErr
		}
		artistInfo := &UserArtistInfoResponse{}
		mErr := json.Unmarshal(artistResponse.Data, artistInfo)
		if mErr != nil {
			log.Printf("[services][applemusic][FetchUserArtists] Error getting deserializing artist info: %v", mErr)
		}
		data := artistInfo.Data[0]
		artist := blueprint.UserArtist{
			ID:    data.Id,
			Name:  data.Attributes.Name,
			Cover: strings.ReplaceAll(data.Attributes.Artwork.Url, "{w}x{h}bb", "300x300bb"),
			URL:   data.Attributes.Url,
		}
		userArtists = append(userArtists, artist)
	}
	userArtistResponse := blueprint.UserLibraryArtists{
		Payload: userArtists,
		Total:   artists.Meta.Total,
	}
	return &userArtistResponse, nil
}

// FetchLibraryAlbums fetches the user's library albums
func (s *Service) FetchLibraryAlbums(apikey, token string) ([]blueprint.LibraryAlbum, error) {
	inst := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://api.music.apple.com/v1",
		Headers: http.Header{
			"Authorization":         []string{fmt.Sprintf("Bearer %s", apikey)},
			"Music-User-MusicToken": []string{token},
		},
	})

	// fetch first 100 albums
	resp, err := inst.Get("/me/library/albums?limit=100", nil)
	if err != nil {
		s.Logger.Error("[services][applemusic][FetchLibraryAlbums] Error getting user albums.", zap.Error(err))
		return nil, err
	}

	var albums UserAlbumsResponse
	err = json.Unmarshal(resp.Data, &albums)
	if err != nil {
		log.Printf("[services][applemusic][FetchLibraryAlbums] Error deserializing user albums: %v\n", err)
		return nil, err
	}

	// fetch the remaining albums
	if albums.Meta.Total > len(albums.Data) {
		for {
			if len(albums.Data) == albums.Meta.Total {
				break
			}
			moreResp, gErr := inst.Get(fmt.Sprintf("/me/library/albums?limit=100&offset=%d", len(albums.Data)), nil)
			if gErr != nil {
				s.Logger.Error("[services][applemusic][FetchLibraryAlbums] Error getting user albums.", zap.Error(gErr))
				return nil, gErr
			}
			var remainingAlbums UserAlbumsResponse
			err = json.Unmarshal(moreResp.Data, &remainingAlbums)
			if err != nil {
				s.Logger.Error("[services][applemusic][FetchLibraryAlbums] Error deserializing user albums.", zap.Error(err))
				return nil, err
			}
			albums.Data = append(albums.Data, remainingAlbums.Data...)
			if len(remainingAlbums.Data) == 0 {
				s.Logger.Warn("[services][applemusic][FetchLibraryAlbums] No more albums to fetch")
				break
			}
			albums.Data = append(albums.Data, remainingAlbums.Data...)
		}
	}

	var userAlbums []blueprint.LibraryAlbum
	for _, a := range albums.Data {
		// get playlist catalog info
		catResponse, gErr := inst.Get(fmt.Sprintf("/me/library/albums/%s/catalog", a.Id), nil)
		if err != nil {
			s.Logger.Error("[services][applemusic][FetchLibraryAlbums] Error getting user album info.", zap.Error(gErr))
			return nil, gErr
		}

		var catInfo UserAlbumsCatalogResponse
		err = json.Unmarshal(catResponse.Data, &catInfo)
		if err != nil {
			s.Logger.Error("[services][applemusic][FetchLibraryAlbums] Error deserializing user album info.", zap.Error(err))
			return nil, err
		}

		userAlbums = append(userAlbums, blueprint.LibraryAlbum{
			ID:          a.Id,
			Title:       a.Attributes.Name,
			URL:         catInfo.Data[0].Attributes.Url,
			ReleaseDate: a.Attributes.ReleaseDate,
			Explicit:    false,
			TrackCount:  a.Attributes.TrackCount,
			Cover:       strings.ReplaceAll(catInfo.Data[0].Attributes.Artwork.Url, "{w}x{h}bb", "300x300bb"),
			Artists:     []string{a.Attributes.ArtistName},
		})
	}

	return userAlbums, nil
}

// FetchTrackListeningHistory fetches all the recently listened to tracks for a user
func (s *Service) FetchTrackListeningHistory(apikey, token string) ([]blueprint.TrackSearchResult, error) {
	inst := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://api.music.apple.com/v1",
		Headers: http.Header{
			"Authorization":         []string{fmt.Sprintf("Bearer %s", apikey)},
			"Music-User-MusicToken": []string{token},
		},
	})

	resp, err := inst.Get("/me/recent/played/tracks", nil)
	if err != nil {
		s.Logger.Error("[services][applemusic][FetchListeningHistory] Error getting listening history.", zap.Error(err))
		return nil, err
	}

	var historyResponse UserTracksListeningHistoryResponse
	err = json.Unmarshal(resp.Data, &historyResponse)
	if err != nil {
		s.Logger.Error("[services][applemusic][FetchListeningHistory] Error deserializing listening history.", zap.Error(err))
		return nil, err
	}

	// limit it to 10 iterations. that should fetch about 100 and a few tracks
	for i := 0; i < 4; i++ {
		if historyResponse.Next == "" {
			break
		}
		nextResp, gErr := inst.Get(historyResponse.Next, nil)
		if gErr != nil {
			s.Logger.Error("[services][applemusic][FetchListeningHistory] Error getting listening history.", zap.Error(gErr))
			return nil, gErr
		}
		var nextHistoryResponse UserTracksListeningHistoryResponse
		err = json.Unmarshal(nextResp.Data, &nextHistoryResponse)
		if err != nil {
			s.Logger.Error("[services][applemusic][FetchListeningHistory] Error deserializing tracks listening history.", zap.Error(err))
			return nil, err
		}
		historyResponse.Data = append(historyResponse.Data, nextHistoryResponse.Data...)
		historyResponse.Next = nextHistoryResponse.Next
	}

	var tracks []blueprint.TrackSearchResult
	for _, t := range historyResponse.Data {
		preview := ""
		if len(t.Attributes.Previews) > 0 {
			preview = t.Attributes.Previews[0].Url
		}
		tracks = append(tracks, blueprint.TrackSearchResult{
			URL:           t.Attributes.Url,
			Artists:       []string{t.Attributes.ArtistName},
			Released:      t.Attributes.ReleaseDate,
			Duration:      util.GetFormattedDuration(t.Attributes.DurationInMillis / 1000),
			DurationMilli: t.Attributes.DurationInMillis,
			Explicit:      t.Attributes.ContentRating == "explicit",
			Title:         t.Attributes.Name,
			Preview:       preview,
			Album:         t.Attributes.AlbumName,
			ID:            t.Id,
			Cover:         strings.ReplaceAll(t.Attributes.Artwork.Url, "{w}x{h}bb", "300x300bb"),
		})
	}

	return tracks, nil
}
