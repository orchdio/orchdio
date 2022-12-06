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

// SearchTrackWithLink fetches a track from the ID using the link.
func SearchTrackWithLink(info *blueprint.LinkInfo, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	cacheKey := "ytmusic:" + info.EntityID
	cachedTrack, err := red.Get(context.Background(), cacheKey).Result()
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

		red.Set(context.Background(), cacheKey, track, 0)
		// TODO: add more fields to the result in the ytmusic library
		thumbnail := ""
		if len(track.Thumbnails) > 0 {
			thumbnail = track.Thumbnails[0].URL
		}
		return &blueprint.TrackSearchResult{
			URL:      info.TargetLink,
			Artists:  artistes,
			Released: "",
			Duration: util.GetFormattedDuration(track.Duration),
			Explicit: false,
			Title:    track.Title,
			Preview:  info.TargetLink, // for now, preview is also original link
			Album:    track.Album.Name,
			ID:       track.VideoID,
			Cover:    thumbnail,
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

func SearchTrackWithTitle(title, artiste string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	identifierHash := util.HashIdentifier(fmt.Sprintf("ytmusic-%s-%s", artiste, title))

	if red.Exists(context.Background(), identifierHash).Val() == 1 {
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Track found in cache: %v\n", identifierHash)
		cachedTrack, err := red.Get(context.Background(), identifierHash).Result()
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
	log.Printf("[services][ytmusic][SearchTrackWithTitle] Track not found in cache, fetching from YT Music: %v\n", identifierHash)
	s := ytmusic.Search(fmt.Sprintf("%s %s", artiste, title))
	r, err := s.Next()
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
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Found track:\n")
		if strings.Contains(t.Title, title) {
			log.Printf("[services][ytmusic][SearchTrackWithTitle] Found track with title: %v\n", title)
			track = t
		}
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
		URL:      fmt.Sprintf("https://music.youtube.com/watch?v=%s", track.VideoID),
		Artists:  artistes,
		Released: "",
		Duration: util.GetFormattedDuration(track.Duration),
		Explicit: track.IsExplicit,
		Title:    track.Title,
		Preview:  fmt.Sprintf("https://music.youtube.com/watch?v=%s", track.VideoID), // for now, preview is also original link
		Album:    track.Album.Name,
		ID:       track.VideoID,
		Cover:    thumbnail,
	}
	serviceResult, err := json.Marshal(result)
	if err != nil {
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Error marshalling track: %v\n", err)
		return nil, err
	}
	newHashIdentifier := util.HashIdentifier(fmt.Sprintf("ytmusic-%s-%s", artistes[0], track.Title))

	trackResultIdentifier := util.HashIdentifier(fmt.Sprintf("ytmusic:%s", track.VideoID))
	err = red.MSet(context.Background(), newHashIdentifier, serviceResult, trackResultIdentifier, serviceResult).Err()
	if err != nil {
		log.Printf("[services][ytmusic][SearchTrackWithTitle] Error caching track: %v\n", err)
		return nil, err
	}
	return result, nil
}

func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	track, err := SearchTrackWithTitle(title, artiste, red)
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
func FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, error) {
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
		go SearchTrackWithTitleChan(track.Title, track.Artistes[0], c, &wg, red)
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
