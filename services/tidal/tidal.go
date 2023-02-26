package tidal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/nleeper/goment"
	"github.com/samber/lo"
	"github.com/vicanso/go-axios"
)

const ApiUrl = "https://listen.tidal.com/v1"
const AuthBase = "https://auth.tidal.com/v1/oauth2"

type Platform interface {
	FetchSingleTrack(id string) (*Track, error)
}

type tidal struct {
	red *redis.Client
}

// SearchWithID searches for a track on tidal using the tidal ID
func SearchWithID(id string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	cacheKey := "tidal:track:" + id
	log.Println("\n[services][tidal][SearchWithID] - cacheKey - ", cacheKey)
	cachedTrack, err := red.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("\n[services][tidal][SearchWithID] - error - Could not fetch record from the cache. This is an unexpected error %v\n", err)
		return nil, err
	}

	if err != nil && err == redis.Nil {
		log.Printf("\n[services][tidal][SearchWithID] - this track has not been cached before %v\n", err)

		tracks, err := FetchSingleTrack(id)

		if err != nil {
			log.Printf("\n[services][tidal][SearchWithID] - error - %v\n", err)
			return nil, err
		}

		var artistes []string
		for _, artist := range tracks.Artists {
			artistes = append(artistes, artist.Name)
		}

		searchResult := blueprint.TrackSearchResult{
			URL:           tracks.URL,
			Artists:       artistes,
			Released:      tracks.StreamStartDate,
			Duration:      util.GetFormattedDuration(tracks.Duration),
			DurationMilli: tracks.Duration * 1000,
			Explicit:      tracks.Explicit,
			Title:         tracks.Title,
			Preview:       "",
			Album:         tracks.Album.Title,
			ID:            strconv.Itoa(tracks.Album.ID),
			Cover:         util.BuildTidalAssetURL(tracks.Album.Cover),
		}

		serialized, err := json.Marshal(searchResult)
		if err != nil {
			log.Printf("\n[services][tidal][SearchWithID] - could not serialize track result - %v\n", err)
			return nil, err
		}
		err = red.Set(context.Background(), cacheKey, serialized, time.Hour*24).Err()
		if err != nil {
			log.Printf("\n[services][tidal][SearchWithID] - could not cache track - %v\n", err)
		} else {
			log.Printf("\n[services][tidal][SearchWithID] - track cached successfully\n")
		}
		return &searchResult, nil

	}

	var deserialized blueprint.TrackSearchResult
	err = json.Unmarshal([]byte(cachedTrack), &deserialized)
	if err != nil {
		log.Printf("\n[services][tidal][SearchWithID] - error - %v\n", err)
		return nil, err
	}
	return &deserialized, nil
}

// FetchSingleTrack fetches a track from tidal
func FetchSingleTrack(id string) (*Track, error) {
	var TidalAccessToken = os.Getenv("TIDAL_ACCESS_TOKEN")
	// TODO: implement refresh token fetching the access token (if expired)
	// TODO: find a way to add access token securely since i need to store somewhere (tidal auth api limitation)
	// TODO: update the access token (probably store in redis)
	accessToken, _ := FetchNewAuthToken()
	TidalAccessToken = accessToken

	// first, fetch the access token hard coded in the config
	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + TidalAccessToken},
		},
	})
	// make a request to the tidal API
	response, err := instance.Get(fmt.Sprintf("/tracks/%s?countryCode=US", id))
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrack] - error - %v\n", err)
		return nil, err
	}
	singleTrack := &Track{}
	err = json.Unmarshal(response.Data, singleTrack)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrack] - error - %v\n", err)
		return nil, err
	}
	return singleTrack, nil
}

//func CheckPlaylistHasBeenUpdated(playlistId string) bool {
//	token, err := FetchNewAuthToken()
//	if err != nil {
//		log.Printf("\n[controllers][platforms][tidal][CheckPlaylistHasBeenUpdated] - Error fetching new token from TIDAL.  - %v\n", err)
//		log.Printf("\n[controllers][platforms][tidal][CheckPlaylistHasBeenUpdated] - FATAL ERROR. PLEASE AUDIT- %v\n", err)
//		return false
//	}
//
//	instance := axios.NewInstance(&axios.InstanceConfig{
//		BaseURL:     ApiUrl,
//		EnableTrace: true,
//		Headers: map[string][]string{
//			"Accept":        {"application/json"},
//			"Authorization": {"Bearer " + token},
//		},
//	})
//	// https://listen.tidal.com/v1/playlists/1b46cea3-e06a-49d9-b21e-b1a1603a44bf?countryCode=US&locale=en_US&deviceType=BROWSER
//	response, err := instance.Get(fmt.Sprintf("/playlists/%s?countryCode=US&locale=en_US&deviceType=BROWSER", playlistId))
//	if err != nil {
//		log.Printf("\n[controllers][platforms][tidal][CheckPlaylistHasBeenUpdated] - Error fetching playlist from TIDAL.  - %v\n", err)
//		return false
//	}
//	var playlist PlaylistInfo
//	err = json.Unmarshal(response.Data, &playlist)
//	if err != nil {
//		log.Printf("\n[controllers][platforms][tidal][CheckPlaylistHasBeenUpdated] - Error unmarshalling playlist from TIDAL.  - %v\n", err)
//		return false
//	}
//
//	if
//}

// SearchTrackWithTitle will perform a search on tidal for the track we want
func SearchTrackWithTitle(title, artiste string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	//identifierHash := util.HashIdentifier(fmt.Sprintf("tidal-%s-%s", title, artiste))

	cleanedArtiste := strings.ToLower(fmt.Sprintf("tidal-%s-%s", util.NormalizeString(artiste), title))
	log.Printf("Searching with stripped artiste: %s. Original artiste: %s", cleanedArtiste, artiste)

	if red.Exists(context.Background(), cleanedArtiste).Val() == 1 {
		log.Printf("\n[services][tidal][SearchTrackWithTitle] - track found in cache\n")
		var result *blueprint.TrackSearchResult
		cachedResult, err := red.Get(context.Background(), cleanedArtiste).Result()
		if err != nil {
			log.Printf("\n[services][tidal][SearchTrackWithTitle] - ⚠️ error fetching key from redis. - %v\n", err)
			return nil, err
		}
		err = json.Unmarshal([]byte(cachedResult), &result)
		if err != nil {
			log.Printf("\n[services][tidal][SearchTrackWithTitle] - ⚠️ error deserializimng cache result - %v\n", err)
			return nil, err
		}
		return result, nil
	}

	result, err := FetchSingleTrackByTitle(title, artiste)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][SearchTrackWithTitle] - could not search track with title '%s' on tidal - %v\n", title, err)
		return nil, err
	}

	// here is where we select the best match. Right now, we just select the first result on the list
	// but ideally if for example we want to filter more "generic" tracks, we can do that here
	// etc.
	if len(result.Tracks.Items) > 0 {
		var track = result.Tracks.Items[0]
		var artistes []string
		for _, artist := range track.Artists {
			artistes = append(artistes, artist.Name)
		}

		tidalTrack := &blueprint.TrackSearchResult{
			URL:      track.Url,
			Artists:  artistes,
			Released: track.StreamStartDate,
			Duration: util.GetFormattedDuration(track.Duration),
			// format the duration in miliseconds. seems to be in seconds from TIDAL
			DurationMilli: track.Duration * 1000,
			Explicit:      track.Explicit,
			Title:         track.Title,
			Preview:       "",
			Album:         track.Album.Title,
			ID:            strconv.Itoa(track.Id),
			Cover:         util.BuildTidalAssetURL(track.Album.Cover),
		}

		serialized, err := json.Marshal(tidalTrack)
		if err != nil {
			log.Printf("\n[services][tidal][SearchTrackWithTitle] - could not serialize track result - %v\n", err)
			return nil, err
		}

		if lo.Contains(tidalTrack.Artists, artiste) {
			keys := map[string]interface{}{
				cleanedArtiste: string(serialized),
			}

			for cacheArtiste, cacheResult := range keys {
				err = red.Set(context.Background(), cacheArtiste, cacheResult, time.Hour*24).Err()
				if err != nil {
					log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error caching track - %v\n", err)
				} else {
					log.Printf("\n[controllers][platforms][tidal][SearchTrackWithTitle] Track %s has been cached\n", tidalTrack.Title)
				}
			}
		}

		return tidalTrack, nil
	}
	return nil, nil
}

// FetchSingleTrackByTitle fetches a track from tidal by title and artist
func FetchSingleTrackByTitle(title, artiste string) (*SearchResult, error) {
	log.Printf("[controllers][platforms][tidal][FetchSingleTrackByTitle] - searching single track by title: %s %s\n", title, artiste)
	accessToken, err := FetchNewAuthToken()
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrackByTitle] - error - %v\n", err)
		return nil, err
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	})

	query := url.QueryEscape(fmt.Sprintf("%s %s", artiste, title))

	log.Printf("[controllers][[platforms][tidal][FetchSingleTrackByTitle]  - Search URL %s\n", fmt.Sprintf("%s %s", artiste, title))

	response, err := instance.Get(fmt.Sprintf("/search/top-hits?query=%s&countryCode=US&limit=2&offset=0&types=TRACKS", query))
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrackByTitle] - error - %v\n", err)
		return nil, err
	}
	searchResult := &SearchResult{}
	err = json.Unmarshal(response.Data, searchResult)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchSingleTrackByTitle] - could not deserialize search response from tidal - %v\n", err)
		return nil, err
	}
	return searchResult, nil
}

// FetchPlaylistInfo returns a playlist info[main] Error processing task handler not found for task
func FetchPlaylistInfo(id string) (*PlaylistInfo, error) {
	accessToken, err := FetchNewAuthToken()
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistInfo] - could not fetch auth token - %v\n", err)
		return nil, err
	}
	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	},
	)
	response, err := instance.Get(fmt.Sprintf("/playlists/%s?countryCode=US", id))
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistInfo] - could not fetch the playlist info for %s - %v\n", err, id)
		return nil, err
	}
	playlistInfo := &PlaylistInfo{}
	err = json.Unmarshal(response.Data, playlistInfo)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistInfo] - could not deserialize playlist info - %v\n", err)
		return nil, err
	}
	return playlistInfo, nil
}

// FetchPlaylist fetches a specific playlist based on the id. It returns the playlist search result,
// a bool to indicate if the playlist has been updated since the last time a call was made
// and an error if there is one
func FetchPlaylist(id string, red *redis.Client) (*PlaylistInfo, *blueprint.PlaylistSearchResult, bool, error) {
	identifierHash := fmt.Sprintf("tidal:playlist:%s", id)

	// infoHash represents the key for the snapshot of the playlist info, in this case
	// just a lasUpdated timestamp in string format.
	infoHash := fmt.Sprintf("tidal:snapshot:%s", id)

	info, err := FetchPlaylistInfo(id)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not fetch playlist info - %v\n", err)
		return nil, nil, false, err
	}

	// if we have already cached the playlist info.
	// The assumption here is that the playlist info and the playlist tracks are always both cached every time
	if red.Exists(context.Background(), identifierHash).Val() == 1 {
		// fetch the playlist info from redis
		cachedInfo, err := red.Get(context.Background(), infoHash).Result()
		if err != nil && err != redis.Nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylist] - could not fetch cached playlist info - %v\n", err)
			return nil, nil, false, err
		}

		// deserialize the playlist info
		var cachedLastPlayedAt string
		_ = json.Unmarshal([]byte(cachedInfo), &cachedLastPlayedAt)

		// format the timestamps on both of the playlist info
		lastUpdated, err := goment.New(cachedLastPlayedAt)
		infoLastUpdated, err := goment.New(info.LastUpdated)

		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylist] - could not parse last updated time - %v\n", err)
			return nil, nil, false, err
		}

		var result *blueprint.PlaylistSearchResult

		// fetch the cached tracks from redis.
		cachedResult, err := red.Get(context.Background(), identifierHash).Result()
		if err != nil {
			log.Printf("\n[services][tidal][FetchPlaylistTracksInfo] - ⚠️ error fetching key from redis. - %v\n", err)
			return nil, nil, false, err
		}
		// deserialize the tracks we fetched from redis
		err = json.Unmarshal([]byte(cachedResult), &result)
		if err != nil {
			log.Printf("\n[services][tidal][FetchPlaylistTracksInfo] - ⚠️ error deserializimng cache result - %v\n", err)
			return nil, nil, false, err
		}
		// if the timestamps are the same, that means that our playlist has not
		// changed, so we can return the cached result. in the other case, we
		// are doing nothing so we go on to fetch the tracks from the tidal api.
		if lastUpdated.IsSame(infoLastUpdated) {
			return info, result, false, nil
		}
	}

	accessToken, err := FetchNewAuthToken()
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - error - %v\n", err)
		return nil, nil, false, err
	}

	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not deserialize playlist result from tidal - %v\n", err)
		return nil, nil, false, err
	}

	playlistResult := &PlaylistTracks{}

	var pages = info.NumberOfTracks / 100
	if pages == 0 {
		pages = 1
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL:     ApiUrl,
		EnableTrace: true,
		Headers: map[string][]string{
			"Accept":        {"application/json"},
			"Authorization": {"Bearer " + accessToken},
		},
	})
	// implement pagination fetching
	for page := 0; page <= pages; page++ {
		response, err := instance.Get(fmt.Sprintf("/playlists/%s/items?offset=%d&limit=100&countryCode=US", id, page*100))
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - error - %v\n", err)
			return nil, nil, false, err
		}
		res := &PlaylistTracks{}
		err = json.Unmarshal(response.Data, res)
		if err != nil {
			log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not deserialize playlist result from tidal - %v\n", err)
			return nil, nil, false, err
		}
		if len(res.Items) == 0 {
			break
		}
		playlistResult.Items = append(playlistResult.Items, res.Items...)
	}

	var tracks []blueprint.TrackSearchResult
	for _, item := range playlistResult.Items {
		var artistes []string
		for _, artist := range item.Item.Artists {
			artistes = append(artistes, artist.Name)
		}
		t := blueprint.TrackSearchResult{
			URL:           item.Item.Url,
			Artists:       artistes,
			Released:      item.Item.StreamStartDate,
			Duration:      util.GetFormattedDuration(item.Item.Duration),
			DurationMilli: item.Item.Duration * 1000,
			Explicit:      item.Item.Explicit,
			Title:         item.Item.Title,
			Preview:       "",
			Album:         item.Item.Album.Title,
			ID:            strconv.Itoa(item.Item.Id),
			Cover:         util.BuildTidalAssetURL(item.Item.Album.Cover),
		}

		tracks = append(tracks, t)
	}
	// then convert to a blueprint.PlaylistSearchResult
	result := &blueprint.PlaylistSearchResult{
		Title:   info.Title,
		Tracks:  tracks, // TODO: playlistResult.Items,
		URL:     info.Url,
		Length:  util.GetFormattedDuration(info.Duration),
		Preview: "",
		Owner:   "", // info.Creator.Id // TODO: implement fetching the user with this ID and populating it here,
		Cover:   util.BuildTidalAssetURL(info.SquareImage),
	}
	ser, _ := json.Marshal(result)
	// cache the result
	err = red.Set(context.Background(), identifierHash, ser, 0).Err()
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not cache playlist for %s into redis - %v\n", err, info.Title)
	} else {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - cached playlist into redis - %v\n", info.Title)
	}

	infoSer, _ := json.Marshal(info.LastUpdated)
	err = red.Set(context.Background(), infoHash, infoSer, 0).Err()
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - could not cache playlist info for %s info into redis - %v\n", err, info.Title)
	} else {
		log.Printf("\n[controllers][platforms][tidal][FetchPlaylistTracksInfo] - cached playlist info into redis - %v\n", info.Title)
	}
	return info, result, true, nil
}

// FetchTrackWithTitleChan fetches a track with the title from tidal but using a channel
func FetchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	track, err := SearchTrackWithTitle(title, artiste, red)
	if err != nil {
		log.Printf("\n[controllers][platforms][tidal][FetchTrackWithTitleChan] - error fetching title - %v\n", err)
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

// FetchTrackWithResult fetches the tracks for a playlist from tidal, using the result from search
// from another platform. This function builds the `PlatformSearchTrack` used to fetch the track
func FetchTrackWithResult(p *blueprint.PlaylistSearchResult, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Title:    track.Title,
			Artistes: track.Artists,
			URL:      track.URL,
			ID:       track.ID,
		})
		continue
	}
	tracks, omittedTracks := FetchTracks(trackSearch, red)
	return tracks, omittedTracks
}

// FetchTracks fetches all the tracks for a playlist from tidal, using the built `PlatformSearchTrack` type
func FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var c = make(chan *blueprint.TrackSearchResult, len(tracks))
	var fetchedTracks []blueprint.TrackSearchResult
	var omittedTracks []blueprint.OmittedTracks
	var wg sync.WaitGroup
	for _, track := range tracks {
		// WARNING: unhandled slice index
		go FetchTrackWithTitleChan(track.Title, track.Artistes[0], c, &wg, red)
		outputTrack := <-c
		if outputTrack == nil || outputTrack.URL == "" {
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Title:    track.Title,
				Artistes: track.Artistes,
				URL:      track.URL,
			})
			continue
		}
		fetchedTracks = append(fetchedTracks, *outputTrack)
	}

	wg.Wait()
	return &fetchedTracks, &omittedTracks
}

func FetchNewAuthToken() (string, error) {
	// now refresh token and get a new access token
	refreshInstance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: AuthBase,
		Headers: map[string][]string{
			"Content-Type": {"application/x-www-form-urlencoded"},
		},
	})
	// default scope. read and write user
	scope := "r_usr w_usr"
	params := url.Values{}
	params.Add("grant_type", "refresh_token")
	params.Add("refresh_token", os.Getenv("TIDAL_REFRESH_TOKEN"))
	params.Add("client_id", os.Getenv("TIDAL_CLIENT_ID"))
	params.Add("scope", scope)
	params.Add("client_secret", os.Getenv("TIDAL_CLIENT_SECRET"))

	inst, err := refreshInstance.Post("/token", params)

	// WARNING: it seems that the axios package does not handle the error when the response is not 200
	// so we need to check the status code ourselves inside the body of the response
	if err != nil {
		log.Printf("\n[services][tidal][auth][CompleteUserAuth] Error refreshing token - %v\n", err)
		return "", err
	}

	refresh := &RefreshTokenResponse{}
	err = json.Unmarshal(inst.Data, refresh)
	if err != nil {
		log.Printf("\n[services][tidal][auth][CompleteUserAuth] Error parsing refresh token response - %v\n", err)
		return "", err
	}
	return refresh.AccessToken, nil
}

// https://listen.tidal.com/v2/my-collection/playlists/folders/create-playlist?description=&folderId=root&isPublic=false&name=xxxxx&countryCode=US&locale=en_US&deviceType=BROWSER - create playlist PUT
// https://listen.tidal.com/v2/my-collection/playlists/folders/remove?trns=trn:playlist:a4a41a8c-a14e-4e60-b671-5f23f07a8a7d&countryCode=US&locale=en_US&deviceType=BROWSER - delete playlist. params in the format, encoded: trns:playlist:playlist_id PUT

func CreateNewPlaylist(title, description, musicToken string, tracks []string) ([]byte, error) {
	log.Printf("\n[services][tidal][CreateNewPlaylist] - creating new playlist - %v\n", title)
	accessToken, err := FetchNewAuthToken()
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error fetching new auth token - %v\n", err)
		return nil, err
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://listen.tidal.com/v2/my-collection/playlists/folders/",
		Headers: map[string][]string{
			"Content-Type":  {"application/x-www-form-urlencoded"},
			"Authorization": {fmt.Sprintf("Bearer %s", accessToken)},
		},
	})
	p := url.Values{}
	p.Add("description", description)
	p.Add("folderId", "root")
	p.Add("isPublic", "true")
	p.Add("name", title)
	p.Add("countryCode", "US")
	p.Add("locale", "en_US")
	p.Add("deviceType", "BROWSER")

	inst, err := instance.Put("create-playlist", p)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error creating playlist - %v\n", err)
		return nil, err
	}

	if inst.Status != 200 {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error creating playlist - %v\n", err)
		return nil, err
	}

	playlist := &CreatePlaylistResponse{}
	err = json.Unmarshal(inst.Data, playlist)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error parsing playlist response - %v\n", err)
		return nil, err
	}

	// now add tracks to the playlist. the tracks are added in a url encoded format, with property of trackIds and can take multiple values. the api endpoint is like: https://listen.tidal.com/v1/playlists/287fae69-37f0-40cf-b95f-52d8a3173530/items?countryCode=US&locale=en_US&deviceType=BROWSER
	instance = axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://listen.tidal.com/v1/playlists/",
		Headers: map[string][]string{
			"Content-Type":  {"application/x-www-form-urlencoded"},
			"Authorization": {fmt.Sprintf("Bearer %s", accessToken)},
			"if-none-match": {"*"},
		},
	})
	p = url.Values{}
	p.Add("trackIds", strings.Join(tracks, ","))

	p.Add("onDupes", "FAIL")
	p.Add("onArtifactNotFound", "FAIL")

	inst, err = instance.Post(fmt.Sprintf("%s/items?countryCode=US&locale=en_US&deviceType=BROWSER", playlist.Data.Uuid), p)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error adding tracks to playlist - %v\n", err)
		return nil, err
	}

	if inst.Status != 200 {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error adding tracks to playlist - %v\n", err)
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error adding tracks to playlist - %v\n", string(inst.Data))
		return nil, err
	}

	itemRes := &PlaylistItemAdditionResponse{}
	err = json.Unmarshal(inst.Data, itemRes)
	if err != nil {
		log.Printf("\n[services][tidal][CreateNewPlaylist] - error parsing playlist item addition response - %v\n", err)
		return nil, err
	}

	createdPlaylistLink := fmt.Sprintf("https://tidal.com/browse/playlist/%s", playlist.Data.Uuid)
	return []byte(createdPlaylistLink), nil
}

// FetchUserPlaylists - fetches the user's playlists
func FetchUserPlaylists(token string) (*UserPlaylistResponse, error) {
	log.Printf("\n[services][tidal][FetchUserPlaylists] - fetching user playlists\n")

	accessToken, err := FetchNewAuthToken()
	if err != nil {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching new auth token - %v\n", err)
		return nil, err
	}

	instance := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://listen.tidal.com",
		Headers: map[string][]string{
			"Content-Type":  {"application/x-www-form-urlencoded"},
			"Authorization": {fmt.Sprintf("Bearer %s", accessToken)},
		},
	})

	p := url.Values{}
	p.Add("countryCode", "US")
	p.Add("locale", "en_US")
	p.Add("deviceType", "BROWSER")
	p.Add("limit", "50")
	p.Add("order", "DATE")
	p.Add("orderDirection", "DESC")
	p.Add("folderId", "root")

	endpoint := fmt.Sprintf("/v2/my-collection/playlists/folders?folderId=root&countryCode=US&locale=en_US&deviceType=BROWSER&limit=50&order=DATE&orderDirection=DESC")

	inst, err := instance.Get(endpoint, p)
	if err != nil {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching user playlists - %v\n", err)
		return nil, err
	}

	if inst.Status != 200 {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching user playlists - %v\n", string(inst.Data))
		return nil, err
	}

	playlists := &UserPlaylistResponse{}
	err = json.Unmarshal(inst.Data, playlists)

	if err != nil {
		log.Printf("\n[services][tidal][FetchUserPlaylists] - error parsing playlists response - %v\n", err)
		return nil, err
	}

	for {
		if playlists.Cursor == "" {
			continue
		}
		endpoint := fmt.Sprintf("/v2/my-collection/playlists/folders?folderId=root&countryCode=US&locale=en_US&deviceType=BROWSER&limit=50&order=DATE&orderDirection=DESC&cursor=%s", playlists.Cursor)
		res, err := instance.Get(endpoint, p)
		if err != nil {
			log.Printf("\n[services][tidal][FetchUserPlaylists] - error fetching user playlists - %v\n", err)
			return nil, err
		}
		// deserialize the response
		resp := &UserPlaylistResponse{}
		err = json.Unmarshal(res.Data, &resp)
		if err != nil {
			log.Printf("\n[services][tidal][FetchUserPlaylists] - error parsing playlists response - %v\n", err)
			return nil, err
		}
		if resp.Cursor == "" {
			log.Printf("\n[services][tidal][FetchUserPlaylists] - no more playlists to fetch. All playlist in library fetched\n")
			break
		}
		playlists.Cursor = resp.Cursor
		playlists.Items = append(playlists.Items, resp.Items...)
	}
	log.Printf("\n[services][tidal][FetchUserPlaylists] - fetched %d playlists\n", len(playlists.Items))
	return playlists, nil
}
