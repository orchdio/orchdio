package ytmusic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/raitonoberu/ytmusic"
	"log"
	"orchdio/blueprint"
	"orchdio/util"
	"strings"
	"sync"
	"time"
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

type Service struct {
	RedisClient          *redis.Client
	IntegrationAppSecret string
	IntegrationAppID     string
}

func NewService(redisClient *redis.Client) *Service {
	return &Service{
		RedisClient: redisClient,
		//IntegrationAppID:     integrationAppID,
		//IntegrationAppSecret: integrationAppSecret,
	}
}

// SearchTrackWithID fetches a track from the ID using the link.
func (s *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	cacheKey := "ytmusic:track:" + info.EntityID
	cachedTrack, err := s.RedisClient.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[services][ytmusic][SearchTrackWithLink] Error fetching track from cache: %v\n", err)
		return nil, err
	}

	if err != nil && err == redis.Nil {
		log.Printf("[services][ytmusic][SearchTrackWithLink] Track not found in cache, fetching from YT Music: %v\n", info.EntityID)
		track, err := FetchSingleTrack(info.EntityID)
		if err != nil {
			log.Printf("[services][ytmusic][SearchTrackWithLink] Error fetching track from YT Music: %v\n", err)
			return nil, err
		}

		log.Printf("[services][ytmusic][SearchTrackWithLink] Track fetched from YT Music: %v\n", track)

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

	return nil, nil
}

func (s *Service) SearchTrackWithTitle(title, artiste string) (*blueprint.TrackSearchResult, error) {
	cleanedArtiste := fmt.Sprintf("ytmusic-%s-%s", util.NormalizeString(artiste), title)

	log.Printf("Searching with stripped artiste: %s. Original artiste: %s", cleanedArtiste, artiste)

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
		return &result, nil
	}
	log.Printf("[services][ytmusic][SearchTrackWithTitle] Track not found in cache, fetching from YT Music: %v\n", cleanedArtiste)
	search := ytmusic.Search(fmt.Sprintf("%s %s", artiste, title))
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
		if strings.Contains(t.Title, title) || strings.Contains(t.Artists[0].Name, artiste) {
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

	return result, nil
}

func (s *Service) SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup) {
	track, err := s.SearchTrackWithTitle(title, artiste)
	if err != nil {
		log.Printf("[services][ytmusic][SearchTrackWithTitleChan] Error searching track: %v\n", err)
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

// FetchTracks searches for the tracks passed and return the results on youtube music.
func (s *Service) FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, error) {
	var fetchedTracks []blueprint.TrackSearchResult
	var omittedTracks []blueprint.OmittedTracks
	var wg sync.WaitGroup
	var c = make(chan *blueprint.TrackSearchResult, len(tracks))

	for _, track := range tracks {
		if track.Title == "" {
			log.Printf("[services][ytmusic][FetchTracks] Track title is empty, skipping: %v\n", track)
			continue
		}
		identifierHash := util.HashIdentifier(fmt.Sprintf("ytmusic-%s-%s", track.Artistes[0], track.Title))
		if red.Exists(context.Background(), identifierHash).Val() == 1 {
			var deserializedTrack blueprint.TrackSearchResult
			cachedTrack := red.Get(context.Background(), identifierHash).Val()
			err := json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
			if err != nil {
				log.Printf("[services][ytmusic][FetchTracks] Error fetching track from cache: %v\n", err)
				return nil, nil
			}
			fetchedTracks = append(fetchedTracks, deserializedTrack)
			continue
		}
		go s.SearchTrackWithTitleChan(track.Title, track.Artistes[0], c, &wg)
		outputTracks := <-c
		if outputTracks != nil {
			log.Printf("[services][ytmusic][FetchTracks] no track found for title : %v\n", track.Title)
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Title:    track.Title,
				URL:      track.URL,
				Artistes: track.Artistes,
			})
			continue
		}
		fetchedTracks = append(fetchedTracks, *outputTracks)
	}
	wg.Wait()
	return &fetchedTracks, nil
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
