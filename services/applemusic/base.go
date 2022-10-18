package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/minchao/go-apple-music"
	"log"
	"orchdio/blueprint"
	"orchdio/util"
	"os"
	"sync"
)

// SearchTrackWithLink fetches a track from the ID using the link.
func SearchTrackWithLink(info *blueprint.LinkInfo, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	cacheKey := "applemusic:" + info.EntityID
	_, err := red.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from cache: %v\n", err)
		return nil, err
	}

	log.Printf("[services][applemusic][SearchTrackWithLink] Track not found in cache, fetching from Apple Music: %v\n", info.EntityID)
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY")}
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

	track := &blueprint.TrackSearchResult{
		URL:      info.TargetLink,
		Artistes: []string{t.Attributes.ArtistName},
		Released: t.Attributes.ReleaseDate,
		Duration: util.GetFormattedDuration(int(t.Attributes.DurationInMillis / 1000)),
		Explicit: false,
		Title:    t.Attributes.Name,
		Preview:  previewURL,
		Album:    t.Attributes.AlbumName,
		ID:       t.Id,
		Cover:    t.Attributes.Artwork.URL,
	}

	serializeTrack, err := json.Marshal(track)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error serializing track: %v\n", err)
		return nil, err
	}
	err = red.Set(context.Background(), cacheKey, serializeTrack, 0).Err()
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error caching track: %v\n", err)
		return nil, err
	}
	return track, nil

}

// SearchTrack searches for a track using the query.
func SearchTrackWithTitle(title, artiste string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	identifierHash := util.HashIdentifier(fmt.Sprintf("applemusic-%s-%s", artiste, title))
	if red.Exists(context.Background(), identifierHash).Val() == 1 {
		track, err := red.Get(context.Background(), identifierHash).Result()
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

	log.Printf("[services][applemusic][SearchTrackWithTitle] Track not found in cache, fetching from Apple Music: %v\n", title)
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY")}
	client := applemusic.NewClient(tp.Client())
	results, response, err := client.Catalog.Search(context.Background(), "us", &applemusic.SearchOptions{
		Term: fmt.Sprintf("%s+%s", artiste, title),
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
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, blueprint.ENORESULT
	}

	if len(results.Results.Songs.Data) == 0 {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, blueprint.ENORESULT
	}

	t := results.Results.Songs.Data[0]
	previewURL := ""
	previews := *t.Attributes.Previews
	if len(previews) > 0 {
		previewURL = previews[0].Url
	}

	track := &blueprint.TrackSearchResult{
		Artistes: []string{t.Attributes.ArtistName},
		Released: t.Attributes.ReleaseDate,
		Duration: util.GetFormattedDuration(int(t.Attributes.DurationInMillis / 1000)),
		Explicit: false, // apple doesnt seem to return explict content value for songs
		Title:    t.Attributes.Name,
		Preview:  previewURL,
		Album:    t.Attributes.AlbumName,
		ID:       t.Id,
		Cover:    t.Attributes.Artwork.URL,
		URL:      t.Attributes.URL,
	}
	serializedTrack, err := json.Marshal(track)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error serializing track: %v\n", err)
		return nil, err
	}
	newHashIdentifier := util.HashIdentifier(fmt.Sprintf("applemusic-%s-%s", t.Attributes.ArtistName, t.Attributes.Name))
	trackIdentifierHash := util.HashIdentifier(fmt.Sprintf("applemusic:%s", t.Id))
	err = red.MSet(context.Background(), newHashIdentifier, string(serializedTrack), trackIdentifierHash, string(serializedTrack)).Err()
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error caching track: %v\n", err)
		return nil, err
	}
	return track, nil
}

// SearchTrackWithTitleChan searches for tracks using title and artistes but do so asynchronously.
func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	track, err := SearchTrackWithTitle(title, artiste, red)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitleChan] Error fetching track: %v\n", err)
		c <- nil
		wg.Add(1)
		defer wg.Done()
		return
	}
	c <- track
	wg.Add(1)
	defer wg.Done()
	return
}

// FetchTracks asynchronously fetches a list of tracks using the track id
func FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks, error) {
	var omittedTracks []blueprint.OmittedTracks
	var results []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		identifier := util.HashIdentifier(fmt.Sprintf("applemusic-%s-%s", track.Artistes[0], track.Title))
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
		go SearchTrackWithTitleChan(track.Title, track.Artistes[0], ch, &wg, red)
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

// FetchPlaylistTrackList fetches a list of tracks for a playlist and saves the last modified date to redis
func FetchPlaylistTrackList(id string, red *redis.Client) (*blueprint.PlaylistSearchResult, error) {
	log.Printf("[services][applemusic][FetchPlaylistTrackList] Fetching playlist tracks: %v\n", id)
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY")}
	client := applemusic.NewClient(tp.Client())
	results, response, err := client.Catalog.GetPlaylist(context.Background(), "us", id, nil)
	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error fetching playlist tracks: %v\n", err)
		return nil, err
	}

	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error fetching playlist tracks: %v\n", err)
		return nil, err
	}

	var tracks []blueprint.TrackSearchResult
	if len(results.Data) == 0 {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error fetching playlist tracks: %v\n", err)
		return nil, blueprint.ENORESULT
	}

	t := results.Data[0]
	duration := 0
	for _, d := range t.Relationships.Tracks.Data {
		tr, err := d.Parse()
		tt := tr.(*applemusic.Song)
		if err != nil {
			log.Printf("[services][applemusic][FetchPlaylistTrackList] Error parsing track: %v\n", err)
			return nil, err
		}

		previewURL := ""
		tribute := *tt.Attributes.Previews
		if len(tribute) > 0 {
			previewURL = tribute[0].Url
		}

		duration += int(tt.Attributes.DurationInMillis)

		track := &blueprint.TrackSearchResult{
			URL:      tt.Attributes.URL,
			Artistes: []string{tt.Attributes.ArtistName},
			Released: tt.Attributes.ReleaseDate,
			Duration: util.GetFormattedDuration(int(tt.Attributes.DurationInMillis / 1000)),
			Explicit: false,
			Title:    tt.Attributes.Name,
			Preview:  previewURL,
			Album:    tt.Attributes.AlbumName,
			ID:       tt.Id,
			Cover:    tt.Attributes.Artwork.URL,
		}

		tracks = append(tracks, *track)
	}

	playlist := &blueprint.PlaylistSearchResult{
		Title:   t.Attributes.Name,
		Tracks:  tracks,
		URL:     t.Attributes.URL,
		Length:  util.GetFormattedDuration(duration / 1000),
		Preview: "",
		Owner:   t.Attributes.CuratorName,
		Cover:   t.Attributes.Artwork.URL,
	}

	// save the last updated at to redis under the key "applemusic:playlist:<id>"
	err = red.Set(context.Background(), fmt.Sprintf("applemusic:playlist:%s", id), t.Attributes.LastModifiedDate, 0).Err()
	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error setting last updated at: %v\n", err)
		return nil, err
	}
	return playlist, nil
}

// FetchPlaylistSearchResult fetches the tracks for a playlist based on the search result
// from another platform
func FetchPlaylistSearchResult(p *blueprint.PlaylistSearchResult, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Title:    track.Title,
			Artistes: track.Artistes,
		})
	}
	tracks, omittedTracks, err := FetchTracks(trackSearch, red)
	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistSearchResult] Error fetching tracks: %v\n", err)
		return nil, nil
	}
	return tracks, omittedTracks
}
