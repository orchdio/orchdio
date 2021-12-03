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

// SearchTrackWithLink fetches the deezer result for the track being searched using the URL
func SearchTrackWithLink(info *blueprint.LinkInfo, red *redis.Client) *blueprint.TrackSearchResult {
	// first, get the cached track
	//cachedKey := fmt.Sprintf("%s-%s", info.Platform, info.EntityID)
	cachedKey := "deezer-" + info.EntityID
	cachedTrack, err := red.Get(context.Background(), cachedKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("\n[services][deezer][playlist][SearchTrackWithLink] error - Could not get cached track %v\n", err)
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
		err = red.Set(context.Background(), cachedKey, string(serializedTrack), 0).Err()
		if err != nil {
			log.Printf("\n[platforms][base][SearchTrackWithLink][error] could not cache track %v\n", dzSingleTrack.Title)
		} else {
			log.Printf("\n[platforms][base][SearchTrackWithLink] Track %s has been cached\n", dzSingleTrack.Title)
		}
		return &fetchedDeezerTrack
	}

	var result blueprint.TrackSearchResult
	err = json.Unmarshal([]byte(cachedTrack), &result)
	if err != nil {
		log.Printf("\n[services][deezer][playlist][SearchTrackWithLink] error - Could not deserialize cached result %v\n", err)
		return nil
	}
	return &result
	//if red.Exists(context.Background(), cachedKey).Val() == 1 {
	//
	//}
}

// SearchTrackWithTitle searches for a track using the title (and artiste) on deezer
func SearchTrackWithTitle(title, artiste, album string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	identifierHash := util.HashIdentifier(fmt.Sprintf("deezer-%s-%s", title, artiste))
	// get the cached track
	if red.Exists(context.Background(), identifierHash).Val() == 1 {
		// deserialize the result from redis
		cachedTrack, err := red.Get(context.Background(), identifierHash).Result()
		if err != nil {
			log.Printf("\n[platforms][base][SearchTrackWithTitle] Could not get cached track. err %v\n", err)
			return nil, err
		}
		var deserializedTrack *blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
		if err != nil {
			log.Printf("\n[platforms][base][SearchTrackWithTitle] Could not deserialize cached track. err %v\n", err)
			return nil, err
		}
		return deserializedTrack, nil
	}


	trackTitle := ExtractTitle(title)
	_link := fmt.Sprintf("track:\"%s\" artist:\"%s\" album:\"%s\"", strings.Trim(trackTitle, " "), strings.Trim(artiste, " "), strings.Trim(album, " "))
	payload := url.QueryEscape(_link)
	link := fmt.Sprintf("%s/search?q=%s", os.Getenv("DEEZER_API_BASE"), payload)

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

		// cache the track
		serializedTrack, err := json.Marshal(out)
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error serializing track - %v\n", err)
		}

		err = red.Set(context.Background(), identifierHash, string(serializedTrack), 0).Err()

		if err != nil {
			log.Printf("\n[platforms][base][SearchTrackWithTitle][error] could not cache track %v\n", title)
		} else {
			log.Printf("\n[platforms][base][SearchTrackWithTitle] Track %s has been cached\n", title)
		}

		return &out, nil
	}
	return nil, nil
}

// SearchTrackWithTitleChan searches for a track similar to `SearchTrackWithTitle` but uses a channel
func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	result, err := SearchTrackWithTitle(title, artiste, "", red)
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
func FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) *[]blueprint.TrackSearchResult {
	var fetchedTracks []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		identifierHash := util.HashIdentifier("deezer-" + track.Title + "-" + track.Artiste)
		// check if its been cached. if so, we grab and return it. if not, we let it search
		if red.Exists(context.Background(), identifierHash).Val() == 1 {
			// deserialize the result from redis
			var deserializedTrack *blueprint.TrackSearchResult
			cachedResult := red.Get(context.Background(), identifierHash).Val()
			err := json.Unmarshal([]byte(cachedResult), &deserializedTrack)
			if err != nil {
				log.Printf("\n[platforms][base][FetchTracks] Could not deserialize cache result. err %v\n", err)
				return nil
			}
			fetchedTracks = append(fetchedTracks, *deserializedTrack)
			continue
		}

		go SearchTrackWithTitleChan(track.Title, track.Artiste, ch, &wg, red)

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
func FetchPlaylistTracklist(id string, red *redis.Client) (*blueprint.PlaylistSearchResult, error) {

	infoLink := "https://api.deezer.com/playlist/"+id+"?limit=1"
	info, err := axios.Get(infoLink)
	if err != nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not fetch playlist info: %v\n", err)
		return nil, err
	}
	var playlistInfo PlaylistTracksSearch
	err = json.Unmarshal(info.Data, &playlistInfo)
	if err != nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}


	tracks, err := axios.Get("https://api.deezer.com/playlist/"+id)

	cachedSnapshot, cacheErr := red.Get(context.Background(), "deezer:playlist:"+id).Result()

	if cacheErr != nil && cacheErr != redis.Nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not get cached snapshot for playlist %v\n", id)
		return nil, cacheErr
	}

	cachedSnapshotID, idErr := red.Get(context.Background(), "deezer:snapshot:"+id).Result()
	if idErr != nil && idErr != redis.Nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not get cached snapshot id for playlist %v\n", id)
		return nil, idErr
	}

	if cacheErr != nil && cacheErr == redis.Nil || cachedSnapshotID != playlistInfo.Checksum {
		var trackList PlaylistTracksSearch
		err = json.Unmarshal(tracks.Data, &trackList)
		if err != nil {
			log.Println("Error deserializing result of playlist tracks search")
			return nil, err
		}

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
			// cache the track
			cacheKey := "deezer-"+result.ID
			serialized, err := json.Marshal(result)
			if err != nil {
				log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not serialize track: %v\n", err)
				return nil, err
			}

			err = red.Set(context.Background(), cacheKey, string(serialized), 0).Err()
			if err != nil {
				log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not cache track: %v\n", err)
			} else {
				log.Printf("\n[services][deezer][FetchPlaylistTracklist] cached track: %v\n", result)
			}
			out = append(out, *result)
		}

		reply := blueprint.PlaylistSearchResult{
			URL:    trackList.Link,
			Tracks: out,
			Title:  trackList.Title,
			Length: util.GetFormattedDuration(trackList.Duration),
		}

		// update the snapshotID cache
		err = red.Set(context.Background(), "deezer:snapshot:"+id, trackList.Checksum, 0).Err()
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not cache snapshot id: %v\n", err)
		} else {
			log.Printf("\n[services][deezer][FetchPlaylistTracklist] cached snapshot id: %v\n", trackList.Checksum)
		}


		// cache the whole playlist
		serializedPlaylist, err := json.Marshal(reply)
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not serialize playlist: %v\n", err)
		}
		err = red.Set(context.Background(), "deezer:playlist:"+id, string(serializedPlaylist), 0).Err()
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not cache playlist: %v\n", err)
		} else {
			log.Printf("\n[services][deezer][FetchPlaylistTracklist] cached playlist: %v %v %v\n", reply.Title, reply.URL, reply.Length)
		}

		// cache the checksum (snapshot id)
		err = red.Set(context.Background(), "deezer:snapshot:"+id, trackList.Checksum, 0).Err()
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not cache snapshot id: %v\n", err)
		} else {
			log.Printf("\n[services][deezer][FetchPlaylistTracklist] cached snapshot id: %v\n", trackList.Checksum)
		}
		return &reply, nil
	}

	playlistResult := &blueprint.PlaylistSearchResult{}
	err = json.Unmarshal([]byte(cachedSnapshot), playlistResult)
	if err != nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracklist] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}
	return playlistResult, nil
}

// FetchPlaylistSearchResult fetches the tracks for a playlist based on the search result
// from another platform (spotify for now).
func FetchPlaylistSearchResult(p *blueprint.PlaylistSearchResult, red *redis.Client) *[]blueprint.TrackSearchResult {
	var trackSearch []blueprint.PlatformSearchTrack
	var omittedTracks []blueprint.OmittedTracks
	for _, track := range p.Tracks {
		// for some reason, there is no spotify url which means could not fetch track, we
		// want to add to the list of "not found" tracks.
		if track.URL == "" {
			// log info about empty track
			log.Printf("\n[services][spotify][base][FetchPlaylistSearchResult] - Could not find track for %s\n", track.Title)
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Title: track.Title,
				URL:   track.URL,
			})
		}
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Artiste: track.Artistes[0],
			Title:   track.Title,
			ID:      track.ID,
		})
	}

	deezerTracks := FetchTracks(trackSearch, red)
	return deezerTracks
}
