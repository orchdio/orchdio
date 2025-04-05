package ytmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"orchdio/blueprint"
	"orchdio/util"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/raitonoberu/ytmusic"
)

const IDENTIFIER = "ytmusic"

func FetchSingleTrack(id string) (*ytmusic.TrackItem, error) {
	r, err := ytmusic.GetWatchPlaylist(id)
	if err != nil {
		log.Printf("[services][ytmusic] Error fetching track: %v\n", err)
		return nil, err
	}

	result := r[0]
	return result, nil
}

type PlatformService interface {
	SearchPlaylistWithID(id string) (*blueprint.PlaylistSearchResult, error)
	SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error)
}

type Service struct {
	RedisClient          *redis.Client
	IntegrationAppSecret string
	IntegrationAppID     string
	App                  *blueprint.DeveloperApp
}

func NewService(redisClient *redis.Client, devApp *blueprint.DeveloperApp) *Service {
	return &Service{
		RedisClient: redisClient,
		App:         devApp,
		// fixme(note): we dont need this for now.
		//IntegrationAppID:     integrationAppID,
		//IntegrationAppSecret: integrationAppSecret,
	}
}

func (s *Service) FetchTracksForSourcePlatform(info *blueprint.LinkInfo, playlistMeta *blueprint.PlaylistMetadata, result chan blueprint.TrackSearchResult) error {
	log.Println("YTmusic not implemented yet...")
	return blueprint.ErrNotImplemented
}

func (s *Service) FetchPlaylistMetaInfo(info *blueprint.LinkInfo) (*blueprint.PlaylistMetadata, error) {
	// todo: implement playlist meta info fetching
	return nil, nil
}

func (s *Service) SearchPlaylistWithID(info *blueprint.LinkInfo) (*blueprint.PlaylistSearchResult, error) {
	// todo: implement playlist searching with id
	return nil, nil
}
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error) {

	cleanedArtiste := fmt.Sprintf("ytmusic-%s-%s", util.NormalizeString(searchData.Artists[0]), searchData.Title)

	if s.RedisClient.Exists(context.Background(), cleanedArtiste).Val() == 1 {
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Track found in cache: %v\n", cleanedArtiste)
		cachedTrack, err := s.RedisClient.Get(context.Background(), cleanedArtiste).Result()
		if err != nil {
			log.Printf("[services][ytmusic][SearchTrackWithTitle] Error fetching track from cache: %v\n", err)
			return nil, err
		}
		var result blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(cachedTrack), &result)
		if err != nil {
			log.Printf("[services][ytmusic][SearchTrackWithTitle] Error unmarshalling cached track: %v\n", err)
			return nil, err
		}

		// send webhook event here
		//svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
		//payload := &blueprint.PlaylistConversionEventTrack{
		//	Platform: IDENTIFIER,
		//	Track:    &result,
		//}
		//ok := svixInstance.SendTrackEvent(s.App.WebhookAppID, payload)
		//if !ok {
		//	log.Printf("[services][ytmusic][SearchTrackWithTitle] Error sending webhook event: %v\n", err)
		//}
		return &result, nil
	}

	log.Printf("[services][ytmusic][SearchTrackWithTitle] Track not found in cache, fetching from YT Music: %v\n", cleanedArtiste)
	search := ytmusic.Search(fmt.Sprintf("%s %s", searchData.Artists[0], searchData.Title))
	r, err := search.Next()
	if err != nil {
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Error fetching track from YT Music: %v\n", err)
		return nil, err
	}

	tracks := r.Tracks

	if len(tracks) == 0 {
		return nil, nil
	}

	var track *ytmusic.TrackItem
	for _, t := range tracks {
		if strings.Contains(t.Title, searchData.Title) || strings.Contains(t.Artists[0].Name, searchData.Artists[0]) {
			track = t
			break
		}
		track = t
		break
	}

	// get artistes
	artistes := make([]string, 0)

	if track == nil {
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Track is nil, returning nil\n")
		return nil, nil
	}

	for _, artist := range track.Artists {
		artistes = append(artistes, artist.Name)
	}

	// get thumbnail
	thumbnail := ""
	if len(track.Thumbnails) > 0 {
		thumbnail = track.Thumbnails[0].URL
	}

	result := &blueprint.TrackSearchResult{
		URL:           fmt.Sprintf("https://music.youtube.com/watch?v=%s", track.VideoID),
		Artists:       artistes,
		Released:      "",
		Duration:      util.GetFormattedDuration(track.Duration),
		DurationMilli: track.Duration * 1000,
		Explicit:      track.IsExplicit,
		Title:         track.Title,
		Preview:       fmt.Sprintf("https://music.youtube.com/watch?v=%s", track.VideoID), // for now, preview is also original link
		Album:         track.Album.Name,
		ID:            track.VideoID,
		Cover:         thumbnail,
	}
	serviceResult, err := json.Marshal(result)
	if err != nil {
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Error marshalling track: %v\n", err)
		return nil, err
	}
	newHashIdentifier := util.HashIdentifier(fmt.Sprintf("ytmusic-%s-%s", artistes[0], track.Title))

	trackResultIdentifier := util.HashIdentifier(fmt.Sprintf("ytmusic:track:%s", track.VideoID))
	err = s.RedisClient.MSet(context.Background(), newHashIdentifier, serviceResult, trackResultIdentifier, serviceResult).Err()
	keys := map[string]interface{}{
		newHashIdentifier:     serviceResult,
		trackResultIdentifier: serviceResult,
	}

	// for each of the cache keys (track result identifier hash which is in format ytmusic:track:VIDEO_ID, and the new hash identifier which is in format ytmusic-ARTISTE-TRACK_TITLE)
	// set the value to the serviceResult (which is the marshalled track result) and set the expiration to 24 hours
	// the former is used to search for the track by its video id, and the latter is used to search for the track by its title and artiste
	for k, v := range keys {
		err = s.RedisClient.Set(context.Background(), k, v, time.Hour*24).Err()
		if err != nil {
			log.Printf("[services][ytmusic][SearchTrackWithTitle] Error caching track: %v\n", err)
			return nil, err
		}
	}

	// send webhook event here
	//svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), false)
	//payload := &blueprint.PlaylistConversionEventTrack{
	//	Platform: IDENTIFIER,
	//	Track:    result,
	//}
	//ok := svixInstance.SendTrackEvent(s.App.WebhookAppID, payload)
	//if !ok {
	//	log.Printf("[services][ytmusic][SearchTrackWithTitle] Error sending webhook event: %v\n", err)
	//}

	return result, nil
}

func (s *Service) CachePlaylistTracksWithID(tracks *[]blueprint.TrackSearchResult) {
	for _, t := range *tracks {
		key := util.FormatPlaylistTrackByCacheKeyID(IDENTIFIER, t.ID)
		value, err := json.Marshal(t)
		if err != nil {
			log.Printf("ERROR [services][spotify][CachePlaylistTracksWithID] json.Marshal error: %v\n", err)
		}
		// fixme(note): setting without expiry
		mErr := s.RedisClient.Set(context.Background(), key, value, 0)
		if mErr != nil {
			log.Printf("ERROR [services][spotify][CachePlaylistTracksWithID] Set error: %v\n", err)
		}
	}
}

// SearchTrackWithID fetches a track from the ID using the link.
func (s *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	cacheKey := "ytmusic:track:" + info.EntityID
	cachedTrack, err := s.RedisClient.Get(context.Background(), cacheKey).Result()

	if err != nil && errors.Is(err, redis.Nil) {
		log.Printf("[services][ytmusic][SearchTrackWithLink] Track not found in cache, fetching from YT Music: %v\n", info.EntityID)
		track, fErr := FetchSingleTrack(info.EntityID)
		if fErr != nil {
			log.Printf("[services][ytmusic][SearchTrackWithLink] Error fetching track from YT Music: %v\n", fErr)
			return nil, fErr
		}

		if track == nil {
			log.Printf("[services][ytmusic][SearchTrackWithLink] Track is nil: %v\n", info.EntityID)
			return nil, nil
		}

		// get artistes
		artistes := make([]string, 0)

		for _, artist := range track.Artists {
			artistes = append(artistes, artist.Name)
		}

		s.RedisClient.Set(context.Background(), cacheKey, track, time.Hour*24)
		// TODO: add more fields to the result in the ytmusic library
		thumbnail := ""
		if len(track.Thumbnails) > 0 {
			thumbnail = track.Thumbnails[0].URL
		}
		return &blueprint.TrackSearchResult{
			URL:           info.TargetLink,
			Artists:       artistes,
			Released:      "",
			Duration:      util.GetFormattedDuration(track.Duration),
			DurationMilli: track.Duration * 1000,
			Explicit:      false,
			Title:         track.Title,
			Preview:       info.TargetLink, // for now, preview is also original link
			Album:         track.Album.Name,
			ID:            track.VideoID,
			Cover:         thumbnail,
		}, nil
	}

	var result blueprint.TrackSearchResult
	err = json.Unmarshal([]byte(cachedTrack), &result)
	if err != nil {
		log.Printf("[services][ytmusic][SearchTrackWithLink] Error unmarshalling cached track: %v\n", err)
		return nil, err
	}
	return &result, nil
}

// FetchPlaylistTracklist fetches the tracks of a playlist on youtube music.
func FetchPlaylistTracklist(id string, red *redis.Client) (*[]blueprint.TrackSearchResult, error) {
	//n := ytmusic.Search(id)
	//r, err := n.Next()
	//if err != nil {
	//	log.Printf("[services][ytmusic][FetchPlaylistTracklist] Error fetching playlist tracklist: %v\n", err)
	//	return nil, err
	//}
	//playlists := r.Playlists
	//if len(playlists) == 0 {
	//	return nil, nil
	//}
	//playlist := playlists[0]
	//tracks := make([]blueprint.TrackSearchResult, len(playlist.))
	//playlistResult := &blueprint.PlaylistSearchResult{
	//	Title:   playlist.Title,
	//	Tracks:  nil,
	//	URL:     "",
	//	Length:  "",
	//	Preview: "",
	//	Owner:   "",
	//	Cover:   "",
	//}
	return nil, nil
}
