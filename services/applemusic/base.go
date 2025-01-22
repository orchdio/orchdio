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
	"log"
	"net/http"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"strings"
	"sync"
	"time"
)

type Service struct {
	IntegrationTeamID string
	IntegrationKeyID  string
	IntegrationAPIKey string
	App               *blueprint.DeveloperApp
	RedisClient       *redis.Client
	PgClient          *sqlx.DB
}

type PlatformService interface {
	SearchPlaylistWithID(id string) (*blueprint.PlaylistSearchResult, error)
	SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error)
	//SearchPlaylistWithTracks(searchResult *blueprint.PlaylistSearchResult) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks, error)
}

func NewService(credentials *blueprint.IntegrationCredentials, pgClient *sqlx.DB, redisClient *redis.Client, devApp *blueprint.DeveloperApp) *Service {
	return &Service{
		// this is the equivalent of the team id
		IntegrationTeamID: credentials.AppID,
		// this is equivalent to the key id
		IntegrationKeyID: credentials.AppSecret,
		// this is equivalent to the apple api key
		IntegrationAPIKey: credentials.AppRefreshToken,
		RedisClient:       redisClient,
		PgClient:          pgClient,
		App:               devApp,
	}
}

// SearchTrackWithID fetches a track from the ID using the link.
func (s *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	cacheKey := "applemusic:track:" + info.EntityID
	_, err := s.RedisClient.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from cache: %v\n", err)
		return nil, err
	}

	log.Printf("[services][applemusic][SearchTrackWithLink] Track not found in cache, fetching from Apple Music: %v\n", info.EntityID)

	// just for dev, log if the credentials are empty
	if s.IntegrationAPIKey != "" {
		log.Printf("[services][applemusic][SearchTrackWithLink] Apple music API key is empty on decoded credentials\n")
	} else {
		log.Printf("[services][applemusic][SearchTrackWithLink] Apple music API key is not empty on decoded credentials\n")
	}

	tp := applemusic.Transport{Token: s.IntegrationAPIKey}
	client := applemusic.NewClient(tp.Client())
	tracks, response, err := client.Catalog.GetSong(context.Background(), "us", info.EntityID, nil)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}
	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}

	if len(tracks.Data) == 0 {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music: %v\n", err)
		return nil, blueprint.EnoResult
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
		log.Printf("[services][applemusic][SearchTrackWithLink] Error serializing track: %v\n", err)
		return nil, err
	}
	err = s.RedisClient.Set(context.Background(), cacheKey, serializeTrack, time.Hour*24).Err()
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error caching track: %v\n", err)
		return nil, err
	}
	return track, nil

}

// SearchTrackWithTitle searches for a track using the query.
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error) {
	strippedTitleInfo := util.ExtractTitle(searchData.Title)
	// if the title is in the format of "title (feat. artiste)" then we search for the title without the feat. artiste
	log.Printf("Apple music: Searching with stripped artiste: %s. Original artiste: %s", strippedTitleInfo.Title, searchData.Artists)
	if s.RedisClient.Exists(context.Background(), searchData.Artists[0]).Val() == 1 {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Track found in cache: %v\n", searchData.Artists[0])
		track, err := s.RedisClient.Get(context.Background(), util.NormalizeString(searchData.Artists[0])).Result()
		if err != nil {
			log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from cache: %v\n", err)
			return nil, err
		}
		var result *blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(track), &result)
		if err != nil {
			log.Printf("[services][applemusic][SearchTrackWithTitle] Error unmarshalling track from cache: %v\n", err)
			return nil, err
		}
		return result, nil
	}

	log.Printf("[services][applemusic][SearchTrackWithTitle] Track not found in cache, fetching track %s from Apple Music with artist %s\n", strippedTitleInfo.Title, util.NormalizeString(searchData.Artists[0]))

	if s.IntegrationAPIKey != "" {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Apple music API key is empty on decoded credentials\n")
	} else {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Apple music API key is not empty on decoded credentials\n")
	}

	tp := applemusic.Transport{Token: s.IntegrationAPIKey}
	client := applemusic.NewClient(tp.Client())
	results, response, err := client.Catalog.Search(context.Background(), "us", &applemusic.SearchOptions{
		Term: fmt.Sprintf("%s+%s", searchData.Artists[0], strippedTitleInfo.Title),
	})

	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}

	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}

	if results.Results.Songs == nil {
		log.Printf("[services][applemusic][SearchTrackWithTitle] No result found for track %s by %s. \n", strippedTitleInfo.Title, searchData.Artists)
		return nil, blueprint.EnoResult
	}

	if len(results.Results.Songs.Data) == 0 {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, blueprint.EnoResult
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
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error serializing track: %v\n", err)
		return nil, err
	}

	if lo.Contains(track.Artists, searchData.Artists[0]) {
		err = s.RedisClient.MSet(context.Background(), map[string]interface{}{
			util.NormalizeString(searchData.Artists[0]): string(serializedTrack),
		}).Err()
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error caching track - %v\n", err)
		} else {
			log.Printf("\n[controllers][platforms][applemusic][SearchTrackWithTitle] Track %s has been cached\n", track.Title)
		}
	}

	// send webhook event
	svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	payload := &blueprint.PlaylistConversionEventTrack{
		EventType: blueprint.PlaylistConversionTrackEvent,
		Platform:  IDENTIFIER,
		TaskId:    searchData.Meta.PlaylistID,
		Track:     track,
	}

	ok := svixInstance.SendTrackEvent(s.App.WebhookAppID, payload)
	if !ok {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Could not send webhook event\n")
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
func (s *Service) FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks, error) {
	var omittedTracks []blueprint.OmittedTracks
	var results []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		identifier := fmt.Sprintf("applemusic-%s-%s", track.Artistes[0], track.Title)
		if red.Exists(context.Background(), identifier).Val() == 1 {
			var deserializedTrack *blueprint.TrackSearchResult
			track, err := red.Get(context.Background(), identifier).Result()
			if err != nil {
				log.Printf("[services][applemusic][FetchTracks] Error fetching track from cache: %v\n", err)
				return nil, nil, err
			}
			err = json.Unmarshal([]byte(track), &deserializedTrack)
			if err != nil {
				log.Printf("[services][applemusic][FetchTracks] Error unmarshalling track from cache: %v\n", err)
				return nil, nil, err
			}
			results = append(results, *deserializedTrack)
			continue
		}
		// async goes brrrr
		searchData := &blueprint.TrackSearchData{
			Title:   track.Title,
			Artists: track.Artistes,
		}
		go s.SearchTrackWithTitleChan(searchData, ch, &wg)
		chTracks := <-ch
		if chTracks == nil {
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Artistes: []string{track.Artistes[0]},
				Title:    track.Title,
			})
			continue
		}

		results = append(results, *chTracks)
	}
	wg.Wait()
	return &results, &omittedTracks, nil
}

// SearchPlaylistWithID fetches a list of tracks for a playlist and saves the last modified date to redis
func (s *Service) SearchPlaylistWithID(id string) (*blueprint.PlaylistSearchResult, error) {
	tp := applemusic.Transport{Token: s.IntegrationAPIKey}
	client := applemusic.NewClient(tp.Client())
	duration := 0

	var tracks []blueprint.TrackSearchResult

	log.Printf("[services][applemusic][SearchPlaylistWithID] Fetching playlist tracks: %v\n", id)
	playlistId := strings.ReplaceAll(id, "/", "")
	log.Printf("[services][applemusic][SearchPlaylistWithID] Playlist id: %v\n", playlistId)
	results, response, err := client.Catalog.GetPlaylist(context.Background(), "us", playlistId, nil)

	if err != nil {
		log.Printf("[services][applemusic][SearchPlaylistWithID][error] - could not fetch playlist tracks:")
		return nil, err
	}

	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][SearchPlaylistWithID][GetPlaylist] Status - %v could not fetch playlist tracks: %v\n", response.StatusCode, err)
		return nil, blueprint.ErrUnknown
	}

	if len(results.Data) == 0 {
		log.Printf("[services][applemusic][SearchPlaylistWithID] result data is empty. Could not fetch playlist tracks: %v\n", err)
		return nil, err
	}

	playlistData := results.Data[0]

	for _, t := range playlistData.Relationships.Tracks.Data {
		tr, err := t.Parse()
		track := tr.(*applemusic.Song)
		if err != nil {
			log.Printf("[services][applemusic][SearchPlaylistWithID] Error parsing track: %v\n", err)
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
			r, err := json.Marshal(tribute)
			if err != nil {
				log.Printf("[services][applemusic][SearchPlaylistWithID] Error serializing preview url: %v\n", err)
				return nil, err
			}
			err = json.Unmarshal(r, &previewStruct)
			if err != nil {
				log.Printf("[services][applemusic][SearchPlaylistWithID] Error deserializing preview url: %v\n", err)
				return nil, err
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

	log.Printf("[services][applemusic][SearchPlaylistWithID] Request configs ")

	_allTracksRes, tErr := ax.Get(fmt.Sprintf("/v1/catalog/us/playlists/%s/tracks", playlistId), p)

	if tErr != nil {
		log.Printf("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks from apple music %v\n", tErr.Error())
		return nil, tErr
	}

	// if the response is a 404 and the length of the tracks we got earlier is 0, that means we really cant get the playlist tracks
	if _allTracksRes.Status == 404 {
		if len(tracks) == 0 {
			log.Printf("[services][applemusic][SearchPlaylistWithID] Could not fetch playlist tracks: %v\n", err)
			return nil, blueprint.ErrUnknown
		}
		result := blueprint.PlaylistSearchResult{
			Title:   playlistData.Attributes.Name,
			Tracks:  tracks,
			URL:     playlistData.Attributes.URL,
			Length:  util.GetFormattedDuration(duration / 1000),
			Preview: "",
			Owner:   playlistData.Attributes.CuratorName,
			Cover:   playlistCover,
			ID:      playlistData.Id,
		}
		return &result, nil
	}

	if _allTracksRes.Status != 200 {
		log.Printf("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks: %v\n", string(_allTracksRes.Data))
		log.Printf("original req url %v", _allTracksRes.Request.URL)
		return nil, err
	}

	var allTracksRes UnlimitedPlaylist
	err = json.Unmarshal(_allTracksRes.Data, &allTracksRes)

	if err != nil {
		log.Printf("[services][applemusic][SearchPlaylistWithID] Error fetching playlist tracks: %v\n", err)
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
		log.Printf("[services][applemusic][SearchPlaylistWithID] Error setting last updated at: %v\n", err)
		return nil, err
	}

	playlist := &blueprint.PlaylistSearchResult{
		Title:   playlistData.Attributes.Name,
		Tracks:  tracks,
		URL:     playlistData.Attributes.URL,
		Length:  util.GetFormattedDuration(duration / 1000),
		Preview: "",
		Owner:   playlistData.Attributes.CuratorName,
		Cover:   playlistCover,
		ID:      playlistData.Id,
	}

	log.Printf("[services][applemusic][SearchPlaylistWithID] Done fetching playlist tracks: %v\n", playlist)
	return playlist, nil
}

// SearchPlaylistWithTracks fetches the tracks for a playlist based on the search result
// from another platform
func (s *Service) SearchPlaylistWithTracks(p *blueprint.PlaylistSearchResult) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Title:    track.Title,
			Artistes: track.Artists,
		})
	}
	tracks, omittedTracks, err := s.FetchTracks(trackSearch, s.RedisClient)
	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistTrackResultsSearchPlaylistTracks] Error fetching tracks: %v\n", err)
		return nil, nil
	}
	return tracks, omittedTracks
}

func (s *Service) CreateNewPlaylist(title, description, musicToken string, tracks []string) ([]byte, error) {
	log.Printf("[services][applemusic][CreateNewPlaylist] Creating new playlist: %v\n", title)
	log.Printf("App Applemusic token is: %v\n", musicToken)
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY"), MusicUserToken: musicToken}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[services][applemusic][CreateNewPlaylist] TP client creation here %v\n", r)
		}
	}()

	client := applemusic.NewClient(tp.Client())
	defer func() {
		if err := recover(); err != nil {
			log.Printf("[services][applemusic][CreateNewPlaylist] Error creating playlist: %v\n", err)
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
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - could not create new playlist: %v\n", err)
	}

	if response.Response.StatusCode == 403 {
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - unauthorized: %v\n", err)
		return nil, blueprint.ErrForbidden
	}

	if response.Response.StatusCode == 401 {
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - unauthorized: %v\n", err)
		return nil, blueprint.ErrUnAuthorized
	}

	if response.Response.StatusCode == 400 {
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - bad request: %v\n", err)
		return nil, blueprint.ErrBadRequest
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
		log.Printf("[services][applemusic][CreateNewPlaylist] Error adding tracks to playlist: %v\n", err)
		return nil, err
	}

	if response.StatusCode >= 400 {
		log.Printf("[services][applemusic][CreateNewPlaylist] Error adding tracks to playlist: %v\n", err)
		return nil, err
	}

	log.Printf("[services][applemusic][CreateNewPlaylist] Successfully created playlist: %v\n", playlist.Data[0].Href)
	return []byte(fmt.Sprintf("https://music.apple.com/us/playlist/%s", playlist.Data[0].Id)), nil
}

// FetchUserPlaylists fetches the user's playlists
func (s *Service) FetchUserPlaylists(token string) ([]UserPlaylistResponse, error) {
	log.Printf("[services][applemusic][FetchUserPlaylists] Fetching user playlists\n")
	tp := applemusic.Transport{Token: s.IntegrationAPIKey, MusicUserToken: token}
	client := applemusic.NewClient(tp.Client())
	// get the user's playlists
	p, _, err := client.Me.GetAllLibraryPlaylists(context.Background(), &applemusic.PageOptions{
		Limit: 100,
	})
	if err != nil {
		log.Printf("[services][applemusic][FetchUserPlaylists] Error getting user playlists: %v\n", err)
		return nil, err
	}
	for {
		if p.Next == "" {
			break
		}
		pr, _, err := client.Me.GetAllLibraryPlaylists(context.Background(), &applemusic.PageOptions{
			Offset: len(p.Data),
		})
		if err != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting user playlists: %v\n", err)
			return nil, err
		}

		log.Printf("[services][applemusic][FetchUserPlaylists] Playlist: %v\n", len(pr.Data))
		if len(pr.Data) == 0 {
			log.Printf("[services][applemusic][FetchUserPlaylists] Fetched all user playlist data\n")
			break
		}
		p.Next = pr.Next
		p.Data = append(p.Data, pr.Data...)
	}

	var playlists []UserPlaylistResponse
	// get each of the playlist information for all the playlists in the user's library
	for _, playlist := range p.Data {
		log.Printf("[services][applemusic][FetchUserPlaylists] Getting catalog info for: %v\n", playlist.Attributes.Name)
		//playlistIds = append(playlistIds, playlist.Id)
		inst := axios.NewInstance(&axios.InstanceConfig{
			BaseURL: "https://api.music.apple.com/v1",
			Headers: http.Header{
				"Authorization":         []string{fmt.Sprintf("Bearer %s", s.IntegrationAPIKey)},
				"Music-User-MusicToken": []string{token},
			},
		})
		link := fmt.Sprintf("/me/library/playlists/%s/catalog", playlist.Id)
		resp, err := inst.Get(link, nil)
		if err != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting user playlist catalog info: %v\n", err)
			return nil, err
		}

		var info PlaylistCatalogInfoResponse
		err = json.Unmarshal(resp.Data, &info)
		if err != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting deserializing playlist catalog info: %v\n", err)
			return nil, err
		}
		data := info.Data[0]

		// get the playlist info itself
		playlistResponse, err := inst.Get(fmt.Sprintf("/me/library/playlists/%s", playlist.Id), nil)
		if err != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting user playlist info: %v\n", err)
			return nil, err
		}
		playlistInfo := &PlaylistInfoResponse{}
		err = json.Unmarshal(playlistResponse.Data, playlistInfo)
		if err != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting deserializing playlist info: %v\n", err)
			return nil, err
		}

		// get playlist tracks
		playlistTracksResponse, err := inst.Get(fmt.Sprintf("/me/library/playlists/%s/tracks", playlist.Id), nil)
		if err != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting user playlist tracks: %v\n", err)
			return nil, err
		}
		playlistTracks := &PlaylistTracksResponse{}
		err = json.Unmarshal(playlistTracksResponse.Data, playlistTracks)
		if err != nil {
			log.Printf("[services][applemusic][FetchUserPlaylists] Error getting deserializing playlist tracks: %v\n", err)
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
		log.Printf("[services][applemusic][FetchUserArtists] Error getting user artists: %v\n", err)
		return nil, err
	}

	var artists UserArtistsResponse
	err = json.Unmarshal(resp.Data, &artists)
	if err != nil {
		log.Printf("[services][applemusic][FetchUserArtists] Error deserializing user artists: %v\n", err)
		return nil, err
	}

	// fetch the remaining artists
	if artists.Meta.Total > len(artists.Data) {
		log.Printf("[services][applemusic][FetchUserArtists] Fetching remaining artists\n")
		for {
			if artists.Next == "" {
				break
			}
			moreResp, err := inst.Get(fmt.Sprintf("/me/library/artists?limit=100&offset=%d", len(artists.Data)), nil)
			if err != nil {
				log.Printf("[services][applemusic][FetchUserArtists] Error getting user artists: %v\n", err)
				return nil, err
			}
			var remainingArtists UserArtistsResponse
			err = json.Unmarshal(moreResp.Data, &remainingArtists)
			if err != nil {
				log.Printf("[services][applemusic][FetchUserArtists] Error deserializing user artists: %v\n", err)
				return nil, err
			}
			artists.Data = append(artists.Data, remainingArtists.Data...)
			if len(remainingArtists.Data) == 0 {
				log.Printf("[services][applemusic][FetchUserArtists] No more artists to fetch\n")
				break
			}
			artists.Next = remainingArtists.Next
			artists.Data = append(artists.Data, remainingArtists.Data...)
		}
	}

	var userArtists []blueprint.UserArtist
	for _, a := range artists.Data {
		// get the artist info itself
		artistResponse, err := inst.Get(fmt.Sprintf("/me/library/artists/%s/catalog", a.Id), nil)
		if err != nil {
			log.Printf("[services][applemusic][FetchUserArtists] Error getting user artist info: %v\n", err)
			return nil, err
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
func FetchLibraryAlbums(apikey, token string) ([]blueprint.LibraryAlbum, error) {
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
		log.Printf("[services][applemusic][FetchLibraryAlbums] Error getting user albums: %v\n", err)
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
		log.Printf("[services][applemusic][FetchLibraryAlbums] Fetching remaining albums\n")
		for {
			if len(albums.Data) == albums.Meta.Total {
				break
			}
			moreResp, err := inst.Get(fmt.Sprintf("/me/library/albums?limit=100&offset=%d", len(albums.Data)), nil)
			if err != nil {
				log.Printf("[services][applemusic][FetchLibraryAlbums] Error getting user albums: %v\n", err)
				return nil, err
			}
			var remainingAlbums UserAlbumsResponse
			err = json.Unmarshal(moreResp.Data, &remainingAlbums)
			if err != nil {
				log.Printf("[services][applemusic][FetchLibraryAlbums] Error deserializing user albums: %v\n", err)
				return nil, err
			}
			albums.Data = append(albums.Data, remainingAlbums.Data...)
			if len(remainingAlbums.Data) == 0 {
				log.Printf("[services][applemusic][FetchLibraryAlbums] No more albums to fetch\n")
				break
			}
			albums.Data = append(albums.Data, remainingAlbums.Data...)
		}
	}

	var userAlbums []blueprint.LibraryAlbum
	for _, a := range albums.Data {
		// get playlist catalog info
		catResponse, err := inst.Get(fmt.Sprintf("/me/library/albums/%s/catalog", a.Id), nil)
		if err != nil {
			log.Printf("[services][applemusic][FetchLibraryAlbums] Error getting user album info: %v\n", err)
			return nil, err
		}

		var catInfo UserAlbumsCatalogResponse
		err = json.Unmarshal(catResponse.Data, &catInfo)
		if err != nil {
			log.Printf("[services][applemusic][FetchLibraryAlbums] Error deserializing user album info: %v\n", err)
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
func FetchTrackListeningHistory(apikey, token string) ([]blueprint.TrackSearchResult, error) {
	inst := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://api.music.apple.com/v1",
		Headers: http.Header{
			"Authorization":         []string{fmt.Sprintf("Bearer %s", apikey)},
			"Music-User-MusicToken": []string{token},
		},
	})

	resp, err := inst.Get("/me/recent/played/tracks", nil)
	if err != nil {
		log.Printf("[services][applemusic][FetchListeningHistory] Error getting listening history: %v\n", err)
		return nil, err
	}

	var historyResponse UserTracksListeningHistoryResponse
	err = json.Unmarshal(resp.Data, &historyResponse)
	if err != nil {
		log.Printf("[services][applemusic][FetchListeningHistory] Error deserializing listening history: %v\n", err)
		return nil, err
	}

	// limit it to 10 iterations. that should fetch about 100 and a few tracks
	for i := 0; i < 4; i++ {
		if historyResponse.Next == "" {
			break
		}
		nextResp, err := inst.Get(historyResponse.Next, nil)
		if err != nil {
			log.Printf("[services][applemusic][FetchListeningHistory] Error getting listening history: %v\n", err)
			return nil, err
		}
		var nextHistoryResponse UserTracksListeningHistoryResponse
		err = json.Unmarshal(nextResp.Data, &nextHistoryResponse)
		if err != nil {
			log.Printf("[services][applemusic][FetchListeningHistory] Error deserializing tracks listening history: %v\n", err)
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
