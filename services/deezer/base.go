package deezer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/vicanso/go-axios"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"zoove/blueprint"
	"zoove/util"
)

type SearchInfo struct {
}

// ExtractTitle retrieves the title of a track if it contains opening and closing brackets
// This is to improve the searching accuracy when searching for these tracks on platforms
func ExtractTitle(title string) string {
	openingBracketIndex := strings.Index(title, "(")
	closingBracketIndex := strings.LastIndex(title, ")")
	if openingBracketIndex != -1 && closingBracketIndex != -1 {
		return title[:openingBracketIndex]
	}
	return title
}

// FetchSingleTrack fetches a single deezer track from the URL
func FetchSingleTrack(link string) (*Track, error) {
	response, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][playlist][FetchSingleTrack] error - Could not fetch single track from deezer %v\n", err)
		// TODO: return something here.
		return nil, err
	}

	singleTrack := &Track{}
	err = json.Unmarshal(response.Data, singleTrack)
	if err != nil {
		log.Printf("\n[services][deezer][playlist][FetchSingleTrack] error - Could not deserialize response %v\n", err)
		return nil, err
	}
	return singleTrack, nil
}

// FetchPlaylistInfo fetches the playlist info.
func FetchPlaylistInfo(link string) (*blueprint.PlaylistSearchResult, error) {
	response, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][playlist][FetchPlaylist] error - could not fetch playlist: %v\n", err)
		return nil, err
	}
	playlist := &Playlist{}
	err = json.Unmarshal(response.Data, playlist)

	if err != nil {
		log.Printf("\n[services][deezer][playlist][FetchPlaylist] error - could not deserialize response into output: %v\n", err)
		return nil, err
	}

	info := blueprint.PlaylistSearchResult{
		URL:     playlist.URL,
		Length:  util.GetFormattedDuration(playlist.Duration),
		Title:   playlist.Title,
		Preview: "",
	}
	return &info, nil
}

// SearchTrackWithLink fetches the deezer result for the track being searched using the URL
func SearchTrackWithLink(info *blueprint.LinkInfo, red *redis.Client) *blueprint.TrackSearchResult {
	// first, get the cached track
	cachedKey := fmt.Sprintf("%s-%s", info.Platform, info.EntityID)
	cachedTrack, err := red.Get(context.Background(), cachedKey).Result()

	if err != nil && err != redis.Nil {
		log.Printf("\n[universal][ConvertTrack] Error getting cached record\n")
		return nil
	}

	if err != nil && err == redis.Nil {
		log.Printf("\n[universal][ConvertTrack] Track has not been cached\n")
		dzSingleTrack, err := FetchSingleTrack(info.TargetLink)
		var dzTrackContributors []string
		for _, contributor := range dzSingleTrack.Contributors {
			if contributor.Type == "artist" {
				dzTrackContributors = append(dzTrackContributors, contributor.Name)
			}
		}

		if err != nil {
			// FIXME: do something.
			log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", err)
		}
		// FIXME: perhaps properly handle this error
		hour := dzSingleTrack.Duration / 60
		sec := dzSingleTrack.Duration % 60
		explicit := false
		if dzSingleTrack.ExplicitContentLyrics == 1 {
			explicit = true
		}

		fetchedDeezerTrack := blueprint.TrackSearchResult{
			Explicit: explicit,
			Duration: fmt.Sprintf("%d:%d", hour, sec),
			URL:      dzSingleTrack.Link,
			Artistes: dzTrackContributors,
			Released: dzSingleTrack.ReleaseDate,
			Title:    dzSingleTrack.Title,
			Preview:  dzSingleTrack.Preview,
			Album:    dzSingleTrack.Album.Title,
			ID:       strconv.Itoa(dzSingleTrack.ID),
		}

		// cache the track
		serializedTrack, err := json.Marshal(fetchedDeezerTrack)
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error serializing track - %v\n", err)
		}
		if err == nil {
			err = red.Set(context.Background(), cachedKey, string(serializedTrack), 0).Err()
			if err != nil {
				log.Printf("\n[platforms][base][SearchTrackWithLink][error] could not cache track %v\n", dzSingleTrack.Title)
			}
			if err == nil {
				log.Printf("\n[platforms][base][SearchTrackWithLink] Track %s has been cached\n", dzSingleTrack.Title)
			}
		}
		return &fetchedDeezerTrack
	}

	var deserializedTrack *blueprint.TrackSearchResult
	log.Printf("cached %v", cachedTrack)
	err = json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
	if err != nil {
		log.Printf("\n[platforms][base][SearchTrackWithLink] Could not deserialize cache result. err %v\n", err)
		return nil
	}

	return deserializedTrack
}

// SearchTrackWithTitle searches for a track using the title (and artiste) on deezer
func SearchTrackWithTitle(title, artiste, album string) (*blueprint.TrackSearchResult, error) {
	trackTitle := ExtractTitle(title)
	_link := fmt.Sprintf("track:\"%s\" artist:\"%s\" album:\"%s\"", strings.Trim(trackTitle, " "), strings.Trim(artiste, " "), strings.Trim(album, " "))
	payload := url.QueryEscape(_link)
	link := fmt.Sprintf("%s/search?q=%s", os.Getenv("DEEZER_API_BASE"), payload)

	log.Printf("\nHere is the link to search: %v\n", link)
	response, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] error - Could not search the track on deezer: %v\n", err)
		return nil, err
	}

	fullTrack := FullTrack{}
	err = json.Unmarshal(response.Data, &fullTrack)
	if err != nil {
		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	if len(fullTrack.Data) > 0 {
		track := fullTrack.Data[0]

		out := blueprint.TrackSearchResult{
			URL:      track.Link,
			Artistes: []string{track.Artist.Name},
			Released: "",
			Duration: util.GetFormattedDuration(track.Duration),
			Explicit: util.DeezerIsExplicit(track.ExplicitContentLyrics),
			Title:    track.Title,
			Preview:  track.Preview,
			Album:    track.Album.Title,
			ID:       strconv.Itoa(track.ID),
		}

		return &out, nil
	}

	return nil, blueprint.ENORESULT
}

// SearchTrackWithTitleChan searches for a track similar to `SearchTrackWithTitle` but uses a channel
func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup) {
	result, err := SearchTrackWithTitle(title, artiste, "")
	if err != nil {
		c <- nil
		wg.Add(1)
		defer wg.Done()
		return
	}
	c <- result
	wg.Add(1)

	defer wg.Done()
	return
}

// FetchTracks searches for the tracks (titles) passed and returns the tracks on deezer.
// This function is used to search for tracks in the playlists the user is trying to convert, on deezer
func FetchTracks(tracks []blueprint.DeezerSearchTrack) *[]blueprint.TrackSearchResult {
	var fetchedTracks []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		go SearchTrackWithTitleChan(track.Title, track.Artiste, ch, &wg)

		outputTracks := <-ch
		if outputTracks == nil {
			log.Printf("\n[services][deezer][FetchTracks] error - no track found for title: %v\n", track.Title)
			continue
		}
		fetchedTracks = append(fetchedTracks, *outputTracks)
	}
	wg.Wait()
	return &fetchedTracks
}

// FetchPlaylistTracklist fetches tracks under a playlist on deezer with pagination
func FetchPlaylistTracklist(link string) (*blueprint.PlaylistSearchResult, *blueprint.Pagination, error) {
	tracks, err := axios.Get(link)
	if err != nil {
		return nil, nil, err
	}
	var trackList PlaylistTracksSearch
	err = json.Unmarshal(tracks.Data, &trackList)
	if err != nil {
		log.Println("Error deserializing result of playlist tracks search")
		return nil, nil, err
	}

	log.Printf("Response from deezer search is %v\n", link)

	var out []blueprint.TrackSearchResult
	for _, track := range trackList.Tracks.Data {
		result := &blueprint.TrackSearchResult{
			URL:      track.Link,
			Artistes: []string{track.Artist.Name},
			//Released: track.r,
			Duration: util.GetFormattedDuration(track.Duration),
			Explicit: util.DeezerIsExplicit(track.ExplicitContentLyrics),
			Title:    track.Title,
			Preview:  track.Preview,
			Album:    track.Album.Title,
			ID:       strconv.Itoa(track.Id),
		}
		out = append(out, *result)
	}

	reply := blueprint.PlaylistSearchResult{
		URL:    trackList.Link,
		Tracks: out,
		Title:  trackList.Title,
		Length: util.GetFormattedDuration(trackList.Duration),
	}

	pagination := &blueprint.Pagination{
		//Next:     trackList.Next,
		//Previous: trackList.Previous,
		//Total:    trackList.Total,
		Platform: "deezer",
	}
	return &reply, pagination, nil
}

// FetchPlaylistSearchResult fetches the tracks for a playlist based on the search result
// from another platform (spotify for now).
func FetchPlaylistSearchResult(p *blueprint.PlaylistSearchResult) *[]blueprint.TrackSearchResult {
	var deezerTrackSearch []blueprint.DeezerSearchTrack
	for _, spotifyTrack := range p.Tracks {
		deezerTrackSearch = append(deezerTrackSearch, blueprint.DeezerSearchTrack{
			Artiste: spotifyTrack.Artistes[0],
			Title:   spotifyTrack.Title,
			ID:      spotifyTrack.ID,
		})
	}

	deezerTracks := FetchTracks(deezerTrackSearch)
	return deezerTracks
}
