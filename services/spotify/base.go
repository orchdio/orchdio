package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"log"
	"os"
	"sync"
	"zoove/blueprint"
	"zoove/util"
)

// createNewSpotifyUInstance creates a new spotify client to make API request that doesn't need user auth
func createNewSpotifyUInstance() *spotify.Client {
	token := fetchNewAuthToken()
	httpClient := spotifyauth.New().Client(context.Background(), token)
	client := spotify.New(httpClient)
	return client
}

// fetchNewAuthToken returns a fresh oauth2 token to be used for spotify api calls
func fetchNewAuthToken() *oauth2.Token {
	config := &clientcredentials.Config{
		ClientID:     os.Getenv("SPOTIFY_ID"),
		ClientSecret: os.Getenv("SPOTIFY_SECRET"),
		TokenURL:     spotifyauth.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchSingleTrack] error  - could not fetch spotify token: %v\n", err)
		return nil
	}
	return token
}

// FetchSingleTrack returns a single track by searching with the title
func FetchSingleTrack(title string) *spotify.SearchResult {
	config := &clientcredentials.Config{
		ClientID:     os.Getenv("SPOTIFY_ID"),
		ClientSecret: os.Getenv("SPOTIFY_SECRET"),
		TokenURL:     spotifyauth.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchSingleTrack] error  - could not fetch spotify token: %v\n", err)
		return nil
	}

	httpClient := spotifyauth.New().Client(context.Background(), token)
	client := spotify.New(httpClient)

	results, err := client.Search(context.Background(), title, spotify.SearchTypeTrack)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchingSingleTrack] error - could not search for track: %v\n", err)
		return nil
	}

	return results
}

// SearchTrackWithTitleChan searches a for a track using the title and channel
func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	result, err := SearchTrackWithTitle(title, artiste, red)
	if err != nil {
		log.Printf("\nError fetching track %s with channels\n. Error: %v", title, err)
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

// SearchTrackWithTitle searches spotify using the title of a track
func SearchTrackWithTitle(title, artiste string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	identifierHash := util.HashIdentifier(fmt.Sprintf("spotify-%s-%s", title, artiste))
	if red.Exists(context.Background(), identifierHash).Val() == 1 {
		// deserialize the result from redis
		var result *blueprint.TrackSearchResult
		cachedResult, err := red.Get(context.Background(), identifierHash).Result()
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal([]byte(cachedResult), &result)
		if err != nil {
			log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not unmarshal cached result: %v\n", err)
			return nil, err
		}
		return result, nil
	}
	spotifySearch := FetchSingleTrack(title)
	if spotifySearch == nil {
		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - error fetching single track on spotify\n")
		// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		return nil, blueprint.ENORESULT
	}

	var spSingleTrack spotify.FullTrack

	// we're extracting just the first track
	if len(spotifySearch.Tracks.Tracks) > 0 {
		spSingleTrack = spotifySearch.Tracks.Tracks[0]
	}

	var spTrackContributors []string
	// reminder: for now, i'm just returning the name of the artiste
	for _, contributor := range spSingleTrack.Artists {
		spTrackContributors = append(spTrackContributors, contributor.Name)
	}

	fetchedSpotifyTrack := blueprint.TrackSearchResult{
		Released: spSingleTrack.Album.ReleaseDate,
		URL:      spSingleTrack.SimpleTrack.ExternalURLs["spotify"],
		Artistes: spTrackContributors,
		Duration: util.GetFormattedDuration(spSingleTrack.Duration / 1000),
		Explicit: spSingleTrack.Explicit,
		Title:    spSingleTrack.Name,
		Preview:  spSingleTrack.PreviewURL,
		Album:    spSingleTrack.Album.Name,
		ID:       spSingleTrack.SimpleTrack.ID.String(),
	}

	// cache the track
	serializedTrack, err := json.Marshal(fetchedSpotifyTrack)
	if err != nil {
		log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not marshal track: %v\n", err)
		return nil, err
	}
	err = red.Set(context.Background(), identifierHash, serializedTrack, 0).Err()
	if err != nil {
		log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not cache track: %v\n", err)
	} else {
		log.Printf("\n[services][spotify][base][SearchTrackWithTitle] success - cached track: %v\n", fetchedSpotifyTrack)
	}

	return &fetchedSpotifyTrack, nil
}

// SearchTrackWithID fetches a track using a track (entityID) and return a spotify track.
func SearchTrackWithID(id string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	cacheKey := "spotify-" + id
	cachedTrack, err := red.Get(context.Background(), cacheKey).Result()

	if err != nil && err != redis.Nil {
		log.Printf("\n[services][SearchTrackWithID] error - Could not fetch record from cache\n")
		return nil, err
	}

	if err != nil && err == redis.Nil {
		log.Printf("\n[services][SearchTrackWithID] function track has not been cached")
		config := &clientcredentials.Config{
			ClientID:     os.Getenv("SPOTIFY_ID"),
			ClientSecret: os.Getenv("SPOTIFY_SECRET"),
			TokenURL:     spotifyauth.TokenURL,
		}

		token, err := config.Token(context.Background())
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchSingleTrack] error  - could not fetch spotify token: %v\n", err)
			return nil, err
		}

		httpClient := spotifyauth.New().Client(context.Background(), token)
		client := spotify.New(httpClient)
		results, err := client.GetTrack(context.Background(), spotify.ID(id))
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchingSingleTrack] error - could not search for track: %v\n", err)
			return nil, err
		}

		var artistes []string

		for _, artiste := range results.Album.Artists {
			artistes = append(artistes, artiste.Name)
		}

		out := blueprint.TrackSearchResult{
			URL:      results.ExternalURLs["spotify"],
			Artistes: artistes,
			Released: results.Album.ReleaseDate,
			Duration: util.GetFormattedDuration(results.Duration / 1000),
			Explicit: results.Explicit,
			Title:    results.Name,
			Preview:  results.PreviewURL,
			Album:    results.Album.Name,
			ID:       results.ID.String(),
		}

		serialized, err := json.Marshal(out)
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchSingleTrack] error - could not serialize track: %v\n", err)
		}

		err = red.Set(context.Background(), cacheKey, serialized, 0).Err()
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchSingleTrack] error - could not cache track: %v\n", err)
		} else {
			log.Printf("\n[services][spotify][base][FetchSingleTrack] success - track cached\n")
		}
		return &out, nil
	}

	var deserializedTrack blueprint.TrackSearchResult
	err = json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
	if err != nil {
		log.Printf("\n[services][SearchTrackWithID] error - Could not deserialize track from cache\n")
		return nil, err
	}
	return &deserializedTrack, nil
}

// FetchPlaylistTracksAndInfo fetches a playlist and returns a list of tracks and the playlist info with pagination info
func FetchPlaylistTracksAndInfo(id string, red *redis.Client) (*blueprint.PlaylistSearchResult, *blueprint.Pagination, error) {
	//client := createNewSpotifyUInstance()
	token := fetchNewAuthToken()
	ctx := context.Background()
	httpClient := spotifyauth.New().Client(ctx, token)
	client := spotify.New(httpClient)


	options := spotify.Fields("description,uri,external_urls,snapshot_id,name")

	// --id--: 55JFgMW6BkDzIIHA7D3Wwo
	// --snapshotID--: MixiMzRkNTFkNDJhZTIyOGQ1ZWViZTFjYWI4OTIxMDdiNWE2ZTA5OGVm

	// first get the snapshot id from cache. if it exists and its the same as the one in the playlist,
	// then we want to return the cached data. however, if the snapshot id is different, we want to
	// fetch the data from the spotify api and cache it.
	cachedSnapshot, cacheErr := red.Get(context.Background(), "spotify:playlist:"+id).Result()
	if cacheErr != nil && cacheErr != redis.Nil {
		log.Printf("\n[services][FetchPlaylistTracksAndInfo] error - Could not fetch snapshot id from cache\n")
		return nil, nil, cacheErr
	}

	cachedSnapshotID, snapshotErr := red.Get(context.Background(), "spotify:snapshot:"+id).Result()
	if snapshotErr != nil && snapshotErr != redis.Nil {
		log.Printf("\n[services][FetchPlaylistTracksAndInfo] error - Could not fetch snapshot id from cache\n")
		return nil, nil, snapshotErr
	}

	info, err := client.GetPlaylist(context.Background(), spotify.ID(id), options)

	if cacheErr != nil && cacheErr == redis.Nil || cachedSnapshotID != info.SnapshotID {

		playlist, err := client.GetPlaylistTracks(ctx, spotify.ID(id))
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Could not fetch playlist from spotify: %v\n", err)
			return nil, nil, err
		}
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - playlist fetched from spotify: %v\n", len(playlist.Tracks))

		// fetch ALL the pages
		for page := 1; ; page++ {
			out := &spotify.PlaylistTrackPage{}
			paginationErr := client.NextPage(ctx, out)
			if paginationErr == spotify.ErrNoMorePages {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - No more pages for playlist\n")
				break
			}
			if paginationErr != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistTracksAndInfo] error - could not fetch playlist: %v\n", err)
				return nil, nil, err
			}
			playlist.Tracks = append(playlist.Tracks, out.Tracks...)
		}

		var tracks []blueprint.TrackSearchResult

		playlistLength := 0
		for _, track := range playlist.Tracks {
			var artistes []string
			for _, artist := range track.Track.Artists {
				artistes = append(artistes, artist.Name)
			}

			playlistLength += track.Track.Duration / 1000

			trackCopy := blueprint.TrackSearchResult{
				URL:      track.Track.ExternalURLs["spotify"],
				Artistes: artistes,
				Released: track.Track.Album.ReleaseDate,
				Duration: util.GetFormattedDuration(track.Track.Duration / 1000),
				Explicit: track.Track.Explicit,
				Title:    track.Track.Name,
				Preview:  track.Track.PreviewURL,
				Album:    track.Track.Album.Name,
				ID:       track.Track.ID.String(),
			}
			tracks = append(tracks, trackCopy)
			// cache the track
			cacheKey := "spotify-" + track.Track.ID.String()
			serialized, err := json.Marshal(trackCopy)
			if err != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not serialize track: %v\n", err)
			}
			err = red.Set(context.Background(), cacheKey, string(serialized), 0).Err()
			if err != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not cache track: %v\n", err)
			} else {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] success - track %s by %s has been cached\n", trackCopy.Title, trackCopy.Artistes[0])
			}
		}

		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - playlist trcaks length: %v\n", len(tracks))

		playlistResult := blueprint.PlaylistSearchResult{
			URL:    info.ExternalURLs["spotify"],
			Tracks: tracks,
			Title:  info.Name,
			Length: util.GetFormattedDuration(playlistLength),
		}

		// update the snapshotID in the cache
		err = red.Set(context.Background(), "spotify:snapshot:"+id, info.SnapshotID, 0).Err()
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not cache snapshot id: %v\n", err)
		} else {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] success - snapshot id has been cached %s\n", info.SnapshotID)
		}

		// cache the whole playlist
		serializedPlaylist, err := json.Marshal(playlistResult)
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not serialize playlist: %v\n", err)
		}

		err = red.Set(context.Background(), "spotify:playlist:"+id, string(serializedPlaylist), 0).Err()
		return &playlistResult, nil, nil
	}

	// if we get here, then the snapshot id is the same as the one in the cache.
	// we can return the cached data
	playlistResult := blueprint.PlaylistSearchResult{}
	err = json.Unmarshal([]byte(cachedSnapshot), &playlistResult)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not unmarshal playlist: %v\n", err)
		return nil, nil, err
	}

	return &playlistResult, nil, nil
}

// FetchTracks fetches tracks in a playlist concurrently using channels
func FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var fetchedTracks []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var omittedTracks []blueprint.OmittedTracks
	var wg sync.WaitGroup
	for _, t := range tracks {
		go SearchTrackWithTitleChan(t.Title, t.Artiste, ch, &wg, red)
		outputTrack := <-ch
		// for some reason, there is no spotify url which means could not fetch track, we
		// want to add to the list of "not found" tracks.
		if outputTrack.URL == "" || outputTrack == nil {
			// log info about empty track
			log.Printf("\n[services][spotify][base][FetchPlaylistSearchResult][warn] - Could not find track for %s\n", t.Title)
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Title: t.Title,
				URL:   t.URL,
				Artiste: t.Artiste,
			})
			continue
		}

		fetchedTracks = append(fetchedTracks, *outputTrack)
	}

	wg.Wait()
	return &fetchedTracks, &omittedTracks
}

// FetchPlaylistSearchResult fetches the track for a playlist based on the search result
// from another platform (deezer for now)
func FetchPlaylistSearchResult(p *blueprint.PlaylistSearchResult, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Artiste: track.Artistes[0],
			Title:   track.Title,
			ID:      track.ID,
			URL: track.URL,
		})
	}
	track, omittedTracks := FetchTracks(trackSearch, red)
	return track, omittedTracks
}
