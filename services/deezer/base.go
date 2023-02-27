package deezer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"orchdio/blueprint"
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
	cachedKey := "deezer:track:" + info.EntityID
	log.Printf("\n[services][deezer][SearchTrackWithLink] cachedKey %v\n", cachedKey)
	cachedTrack, err := red.Get(context.Background(), cachedKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("\n[services][deezer][playlist][SearchTrackWithLink] error - Could not get cached track %v\n", err)
		return nil
	}

	// if we have not cached this track before
	if err != nil && err == redis.Nil {
		log.Printf("\n[universal][ConvertEntity] Track has not been cached\n")
		dzSingleTrack, err := FetchSingleTrack(info.TargetLink)
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
			log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error serializing track - %v\n", err)
		}

		// cache the result
		err = red.Set(context.Background(), cachedKey, string(serializedTrack), time.Hour*24).Err()
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
}

// SearchTrackWithTitle searches for a track using the title (and artiste) on deezer
// This is typically expected to be used when the track we want to fetch is the one we just
// want to search on. That is, the other platforms that the user is trying to convert to.
func SearchTrackWithTitle(title, artiste, album string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	//searchKey := fmt.Sprintf("deezer-%s-%s", artiste, title)
	cacheKey := fmt.Sprintf("deezer-%s-%s", util.NormalizeString(artiste), title)

	log.Printf("\n[services][deezer][playlist][SearchTrackWithTitle] first artiste and title %s %s\n", artiste, title)
	// get the cached track
	if red.Exists(context.Background(), cacheKey).Val() == 1 {
		log.Printf("\n[services][deezer][playlist][SearchTrackWithTitle] Track has been cached\n")
		// deserialize the result from redis
		cachedTrack, err := red.Get(context.Background(), cacheKey).Result()
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

	log.Printf("\n[services][deezer][playlist][SearchTrackWithTitle] Track has not been cached\n")

	trackTitle := ExtractTitle(title)

	// for deezer we'll not trim the artiste name. this is because it becomes way less accurate.
	// deezer has second to the lowest accuracy in terms of search results (youtube being the lowest)
	// however, just like others, we're caching the result under the normalized string, which contains trimmed artiste name
	// like so: "deezer-artistename-title". For example: "deezer-flatbushzombies-reelgirls
	_link := fmt.Sprintf("track:\"%s\" artist:\"%s\" album:\"%s\"", strings.Trim(trackTitle, " "),
		artiste, strings.Trim(album, " "))
	link := fmt.Sprintf("%s/search?q=%s", os.Getenv("DEEZER_API_BASE"), url.QueryEscape(_link))

	response, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] error - Could not search the track on deezer: %v\n", err)
		return nil, err
	}

	log.Printf("\n[services][deezer][playlist][SearchTrackWithTitle] Searched deezer for track...")

	fullTrack := FullTrack{}
	err = json.Unmarshal(response.Data, &fullTrack)
	if err != nil {
		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] error - Could not deserialize the body into the out response: %v\n", err)
		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] error - Could not deserialize the body into the out response, body is: %v\n", string(response.Data))
		return nil, err
	}

	// NB: when the time comes to properly handle the results and return the best match (sometimes its like the 2nd result)
	// then, this is where to probably start.
	if len(fullTrack.Data) > 0 {
		track := fullTrack.Data[0]

		out := blueprint.TrackSearchResult{
			URL:           track.Link,
			Artists:       []string{track.Artist.Name},
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
		serializedTrack, err := json.Marshal(out)
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error serializing track - %v\n", err)
		}
		//newHashIdentifier := util.HashIdentifier("deezer-" + out.Artistes[0] + "-" + out.Title)
		// if the artistes are the same, the track result is most likely the same (except remixes, an artiste doesnt have two tracks with the same name)
		if lo.Contains(out.Artists, artiste) {
			log.Printf("\n[services][deezer][playlist][SearchTrackWithTitle][debug] - the result seems to not be exact same track but same track.\n")
			err = red.MSet(context.Background(), map[string]interface{}{
				cacheKey: string(serializedTrack),
			}).Err()
			if err != nil {
				log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error caching track - %v\n", err)
			} else {
				log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] Track %s has been cached\n", out.Title)
			}
		}

		log.Printf("\n[services][deezer][base][SearchTrackWithTitle] Deezer search for track done %v\n", out)

		//log.Printf("Old identifier: %s and new identifier: %s", identifierHash, newHashIdentifier)
		//log.Printf("\n[services][deezer][playlist][SearchTrackWithTitle] second artiste and title %s %s\n", out.Artistes[0], out.Title)

		// cache tracks. Here we are caching both with hash identifier and with the ID of the track itself
		// this is because in some cases, we need to fetch by ID and not by title
		// cache track but with identifier. this is for when we're searching by title again and its the same
		// track as this
		//err = red.MSet(context.Background(), newHashIdentifier, string(serializedTrack), fmt.Sprintf("deezer:%s", out.ID), string(serializedTrack)).Err()
		//if err != nil {
		//	log.Printf("\n[platforms][base][SearchTrackWithTitle][error] could not cache track %v\n", title)
		//}
		return &out, nil
	}

	log.Printf("\n[services][deezer][base][SearchTrackWithTitle] Deezer search for track done but no results. Searched with %s \n", _link)
	return nil, nil
}

// SearchTrackWithTitleChan searches for a track similar to `SearchTrackWithTitle` but uses a channel
func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	result, err := SearchTrackWithTitle(title, artiste, "", red)
	if err != nil {
		defer wg.Done()
		c <- nil
		wg.Add(1)
		return
	}
	defer wg.Done()
	c <- result
	wg.Add(1)
	return
}

// FetchTracks searches for the tracks (titles) passed and returns the tracks on deezer.
// This function is used to search for tracks in the playlists the user is trying to convert, on deezer
func FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var fetchedTracks []blueprint.TrackSearchResult
	var omittedTracks []blueprint.OmittedTracks
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		// in order to create the identifier that we use to recognize tracks in cache, we simply take the artiste
		// name. but the thing is that an artiste can have spaces in their name, etc. this is definitely going to not go as we expect
		// so we need to remove spaces and weird characters from the artiste name
		// this is the same for the title of the track

		//cleanedArtiste := util.NormalizeString("deezer-" + track.Artistes[0] + "-" + track.Title)
		cleanedArtiste := fmt.Sprintf("deezer-%s-%s", util.NormalizeString(track.Artistes[0]), track.Title)
		// WARNING: unhandled slice index
		// check if its been cached. if so, we grab and return it. if not, we let it search
		if red.Exists(context.Background(), cleanedArtiste).Val() == 1 {
			// deserialize the result from redis
			var deserializedTrack *blueprint.TrackSearchResult
			cachedResult := red.Get(context.Background(), cleanedArtiste).Val()
			err := json.Unmarshal([]byte(cachedResult), &deserializedTrack)
			if err != nil {
				log.Printf("\n[platforms][base][FetchTracks] Could not deserialize cache result. err %v\n", err)
				return nil, nil
			}
			fetchedTracks = append(fetchedTracks, *deserializedTrack)
			continue
		}
		// WARNING: unhandled slice index
		go SearchTrackWithTitleChan(track.Title, track.Artistes[0], ch, &wg, red)

		outputTracks := <-ch
		if outputTracks == nil {
			log.Printf("\n[services][deezer][FetchTracks] error - no track found for title: %v\n", track.Title)
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
	return &fetchedTracks, &omittedTracks
}

// FetchPlaylistTracksAndInfo fetches tracks under a playlist on deezer with pagination
func FetchPlaylistTracksAndInfo(id string, red *redis.Client) (*blueprint.PlaylistSearchResult, error) {
	log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] Fetching playlist %v\n", id)
	infoLink := "https://api.deezer.com/playlist/" + id + "?limit=1"
	info, err := axios.Get(infoLink)
	if err != nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not fetch playlist info: %v\n", err)
		return nil, err
	}
	var playlistInfo PlaylistTracksSearch
	err = json.Unmarshal(info.Data, &playlistInfo)
	if err != nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}

	tracks, err := axios.Get("https://api.deezer.com/playlist/" + id)

	cachedSnapshot, cacheErr := red.Get(context.Background(), "deezer:playlist:"+id).Result()

	if cacheErr != nil && cacheErr != redis.Nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not get cached snapshot for playlist %v\n", id)
		return nil, cacheErr
	}

	cachedSnapshotID, idErr := red.Get(context.Background(), "deezer:snapshot:"+id).Result()
	if idErr != nil && idErr != redis.Nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not get cached snapshot id for playlist %v\n", id)
		return nil, idErr
	}

	// if we have not cached this track or the snapshot has changed (that is, the playlist has been updated), then
	// we need to fetch the tracks and cache them
	if cacheErr != nil && cacheErr == redis.Nil || cachedSnapshotID != playlistInfo.Checksum {
		var trackList PlaylistTracksSearch
		err = json.Unmarshal(tracks.Data, &trackList)
		if err != nil {
			log.Println("Error deserializing result of playlist tracks search")
			return nil, err
		}

		var out []blueprint.TrackSearchResult
		for _, track := range trackList.Tracks.Data {
			log.Printf("deezer track duration mill is %v", track.Duration)
			result := &blueprint.TrackSearchResult{
				URL:     track.Link,
				Artists: []string{track.Artist.Name},
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
			serialized, err := json.Marshal(result)
			if err != nil {
				log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not serialize track: %v\n", err)
				return nil, err
			}

			err = red.Set(context.Background(), cacheKey, string(serialized), 0).Err()
			if err != nil {
				log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not cache track: %v\n", err)
			} else {
				log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] cached track: %v\n", result)
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
		err = red.Set(context.Background(), "deezer:snapshot:"+id, trackList.Checksum, 0).Err()
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not cache snapshot id: %v\n", err)
		} else {
			log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] cached snapshot id: %v\n", trackList.Checksum)
		}

		// cache the whole playlist
		serializedPlaylist, err := json.Marshal(reply)
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not serialize playlist: %v\n", err)
		}
		err = red.Set(context.Background(), "deezer:playlist:"+id, string(serializedPlaylist), 0).Err()
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not cache playlist: %v\n", err)
		} else {
			log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] cached playlist: %v %v %v\n", reply.Title, reply.URL, reply.Length)
		}

		// cache the checksum (snapshot id)
		err = red.Set(context.Background(), "deezer:snapshot:"+id, trackList.Checksum, 0).Err()
		if err != nil {
			log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not cache snapshot id: %v\n", err)
		} else {
			log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] cached snapshot id: %v\n", trackList.Checksum)
		}
		return &reply, nil
	}

	playlistResult := &blueprint.PlaylistSearchResult{}
	err = json.Unmarshal([]byte(cachedSnapshot), playlistResult)
	if err != nil {
		log.Printf("\n[services][deezer][FetchPlaylistTracksAndInfo] error - Could not deserialize the body into the out response: %v\n", err)
		return nil, err
	}
	return playlistResult, nil
}

// FetchPlaylistSearchResult fetches the tracks for a playlist based on the search result
// from another platform
func FetchPlaylistSearchResult(p *blueprint.PlaylistSearchResult, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Artistes: track.Artists,
			Title:    track.Title,
			ID:       track.ID,
			URL:      track.URL,
		})
	}
	deezerTracks, omittedTracks := FetchTracks(trackSearch, red)
	return deezerTracks, omittedTracks
}

// CreateNewPlaylist creates a new playlist for a user on their deezer account
func CreateNewPlaylist(title, userDeezerId, token string, tracks []string) ([]byte, error) {
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
func FetchUserPlaylists(token string) (*UserPlaylistsResponse, error) {
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
func FetchUserArtists(token string) (*blueprint.UserLibraryArtists, error) {
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
			ID:      strconv.Itoa(artist.Id),
			Name:    artist.Name,
			Picture: artist.Picture,
			URL:     artist.Link,
		})
	}

	response := blueprint.UserLibraryArtists{
		Payload: artists,
		Total:   artistsResponse.Total,
	}
	log.Printf("\n[services][deezer][FetchUserArtists] Fetched user deezer artists: %v\n", response)
	return &response, nil
}
