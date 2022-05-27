package tidal

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
	"zoove/blueprint"
	"zoove/util"
)

const ApiUrl = "https://listen.tidal.com/v1"
const AuthBase = "https://auth.tidal.com/v1/oauth2"

type Platform interface {
	FetchSingleTrack(id string) (*Track, error)
}

type tidal struct {
	red *redis.Client
}

func SearchWithID(id string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	cacheKey := "tidal:" + id
	log.Println("\n[services][tidal][SearchWithID] - cacheKey - ", cacheKey)
	cachedTrack, err := red.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("\n[services][tidal][SearchWithID] - error - Could not fetch record from the cache. This is an unexpected error %v\n", err)
		return nil, err
	}

	if err != nil && err == redis.Nil {
		log.Printf("\n[services][tidal][SearchWithID] - this track has not been cached before %v\n", err)

		// TODO: implement redis caching
		tracks, err := FetchSingleTrack(id)

		if err != nil {
			log.Printf("\n[services][tidal][SearchWithID] - error - %v\n", err)
			return nil, err
		}
		log.Printf("\n[services][tidal][SearchWithID] - tracks - %v\n", tracks)

		var artistes []string
		for _, artist := range tracks.Artists {
			artistes = append(artistes, artist.Name)
		}

		searchResult := blueprint.TrackSearchResult{
			URL:      tracks.URL,
			Artistes: artistes,
			Released: tracks.StreamStartDate,
			Duration: util.GetFormattedDuration(tracks.Duration),
			Explicit: tracks.Explicit,
			Title:    tracks.Title,
			Preview:  "",
			Album:    tracks.Album.Title,
			ID:       strconv.Itoa(tracks.Album.ID),
			Cover:    util.BuildTidalAssetURL(tracks.Album.Cover),
		}

		serialized, err := json.Marshal(searchResult)
		if err != nil {
			log.Printf("\n[services][tidal][SearchWithID] - could not serialize track result - %v\n", err)
			return nil, err
		}
		err = red.Set(context.Background(), cacheKey, serialized, 0).Err()
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

// SearchTrackWithTitle will perform a search on tidal for the track we want
func SearchTrackWithTitle(title, artiste string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	identifierHash := util.HashIdentifier(fmt.Sprintf("tidal-%s-%s", title, artiste))

	if red.Exists(context.Background(), identifierHash).Val() == 1 {
		var result *blueprint.TrackSearchResult
		cachedResult, err := red.Get(context.Background(), identifierHash).Result()
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

	var track = result.Tracks.Items[0]
	var artistes []string
	for _, artist := range track.Artists {
		artistes = append(artistes, artist.Name)
	}

	tidalTrack := &blueprint.TrackSearchResult{
		URL:      track.Url,
		Artistes: artistes,
		Released: track.StreamStartDate,
		Duration: util.GetFormattedDuration(track.Duration),
		Explicit: track.Explicit,
		Title:    track.Title,
		Preview:  "",
		Album:    track.Album.Title,
		ID:       strconv.Itoa(track.Id),
		Cover:    util.BuildTidalAssetURL(track.Album.Cover),
	}

	serialized, err := json.Marshal(tidalTrack)
	if err != nil {
		log.Printf("\n[services][tidal][SearchTrackWithTitle] - could not serialize track result - %v\n", err)
		return nil, err
	}
	newHashIdentifier := util.HashIdentifier(fmt.Sprintf("tidal-%s-%s", tidalTrack.Artistes[0], tidalTrack.Title))
	// FIXME: perhaps look into how to batch insert into redis
	err = red.Set(context.Background(), newHashIdentifier, serialized, 0).Err()
	err = red.Set(context.Background(), identifierHash, serialized, 0).Err()
	if err != nil {
		log.Printf("\n[services][tidal][SearchTrackWithTitle] - could not cache track - %v\n", err)
	} else {
		log.Printf("\n[services][tidal][SearchTrackWithTitle] - track cached successfully\n")
	}
	return tidalTrack, nil
}

func FetchSingleTrackByTitle(title, artiste string) (*SearchResult, error) {
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
