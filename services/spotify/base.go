package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/samber/lo"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"log"
	"orchdio/blueprint"
	"orchdio/util"
	"os"
	"strings"
	"sync"
	"time"
)

// createNewSpotifyUInstance creates a new spotify client to make API request that doesn't need user auth
// NOT SURE WHAT I REALLY MEANT BY THIS BUT WHATEVER, KEEP AN EYE ON IT.
func createNewSpotifyUInstance() *spotify.Client {
	token := FetchNewAuthToken()
	httpClient := spotifyauth.New().Client(context.Background(), token)
	client := spotify.New(httpClient)
	return client
}

// ExtractArtiste retrieves an artiste from a passed string containing something like
// feat.
func ExtractArtiste(artiste string) string {
	featIndex := strings.Index(artiste, "feat")
	if featIndex != -1 {
		return strings.Trim(artiste[:featIndex], " ")
	}
	return strings.ReplaceAll(artiste, " ", "")
}

// FetchNewAuthToken returns a fresh oauth2 token to be used for spotify api calls
func FetchNewAuthToken() *oauth2.Token {
	config := &clientcredentials.Config{
		ClientID:     os.Getenv("SPOTIFY_ID"),
		ClientSecret: os.Getenv("SPOTIFY_SECRET"),
		TokenURL:     spotifyauth.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		if err.Error() == "oauth2: cannot fetch token: 503 Service Unavailable" {
			log.Printf("\n[services][spotify][base][FetchSingleTrack] error - 503 Service Unavailable\n")
			return nil
		}
		log.Printf("\n[services][spotify][base][FetchSingleTrack] error  - could not fetch spotify token: %v\n", err)
		return nil
	}
	return token
}

// FetchSingleTrack returns a single track by searching with the title
func FetchSingleTrack(title, artiste string) *spotify.SearchResult {
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

	results, err := client.Search(context.Background(), fmt.Sprintf("%s %s", artiste, title), spotify.SearchTypeTrack)
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

// SearchTrackWithTitle searches spotify using the title of a track
// This is typically expected to be used when the track we want to fetch is the one we just
// want to search on. That is, the other platforms that the user is trying to convert to.
func SearchTrackWithTitle(title, artiste string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	strippedArtiste := ExtractArtiste(artiste)
	cleanedArtiste := fmt.Sprintf("spotify-%s-%s", util.NormalizeString(artiste), title)

	log.Printf("Spotify: Searching with stripped artiste: %s. Original artiste: %s", cleanedArtiste, strippedArtiste)
	// if we have searched for this specific track before, we return the cached result
	// And how do we know if we have cached it before?
	// We store the hash of the title and artiste of the track in redis. we check if the hash of the
	// track we want to search exist.
	if red.Exists(context.Background(), cleanedArtiste).Val() == 1 {
		log.Printf("Spotify: Found cached result for %s", cleanedArtiste)
		// deserialize the result from redis
		var result *blueprint.TrackSearchResult
		cachedResult, err := red.Get(context.Background(), cleanedArtiste).Result()
		if err != nil {
			log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not get cached result for track. This is an unexpected error: %v\n", err)
			return nil, err
		}
		err = json.Unmarshal([]byte(cachedResult), &result)
		if err != nil {
			log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not unmarshal cached result: %v\n", err)
			return nil, err
		}
		return result, nil
	}

	spotifySearch := FetchSingleTrack(title, strippedArtiste)
	if spotifySearch == nil {
		log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - error fetching single track on spotify\n")
		// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		return nil, blueprint.ENORESULT
	}

	// probably better to deserialize the ```spotifySearch.Tracks``` so we can check if its nil or not
	// but it seems if its nil, then the spotiufySearch.Artists is also nil so check for that for now. but if
	// a similar problem where a result is empty but not detected as omittedTrack comes up again for spotify,
	// then we should check here and do the former.
	if len(spotifySearch.Tracks.Tracks) == 0 {
		log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - error fetching single track on spotify\n")
		// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		return nil, blueprint.ENORESULT
	}

	//if spotifySearch.Artists == nil {
	//	log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - error fetching single track on spotify\n")
	//	// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
	//	return nil, blueprint.ENORESULT
	//}

	log.Printf("\n[controllers][platforms][spotify][ConvertEntity] info - found %v tracks on spotify\n", len(spotifySearch.Tracks.Tracks))

	var spSingleTrack spotify.FullTrack

	// we're extracting just the first track.
	// NB: when the time comes to properly handle the results and return the best match (sometimes its like the 2nd result)
	// then, this is where to probably start.
	if len(spotifySearch.Tracks.Tracks) > 0 {
		spSingleTrack = spotifySearch.Tracks.Tracks[0]
	}

	var cover string
	// fetch the spotify image preview.
	if len(spSingleTrack.Album.Images) > 0 {
		cover = spSingleTrack.Album.Images[0].URL
	}

	// fetch all the tracks from the contributors.
	var spTrackContributors []string
	// reminder: for now, i'm just returning the name of the artiste
	for _, contributor := range spSingleTrack.Artists {
		spTrackContributors = append(spTrackContributors, contributor.Name)
	}

	fetchedSpotifyTrack := blueprint.TrackSearchResult{
		Released:      spSingleTrack.Album.ReleaseDate,
		URL:           spSingleTrack.SimpleTrack.ExternalURLs["spotify"],
		Artists:       spTrackContributors,
		Duration:      util.GetFormattedDuration(spSingleTrack.Duration / 1000),
		DurationMilli: spSingleTrack.Duration,
		Explicit:      spSingleTrack.Explicit,
		Title:         spSingleTrack.Name,
		Preview:       spSingleTrack.PreviewURL,
		Album:         spSingleTrack.Album.Name,
		ID:            spSingleTrack.SimpleTrack.ID.String(),
		Cover:         cover,
	}

	// serialize the track
	serializedTrack, err := json.Marshal(fetchedSpotifyTrack)
	trackCacheKey := "spotify:track:" + fetchedSpotifyTrack.ID

	if lo.Contains(fetchedSpotifyTrack.Artists, artiste) {
		err = red.MSet(context.Background(), map[string]interface{}{
			cleanedArtiste: string(serializedTrack),
		}).Err()
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error caching track - %v\n", err)
		} else {
			log.Printf("\n[controllers][platforms][spotify][SearchTrackWithTitle] Track %s has been cached\n", fetchedSpotifyTrack.Title)
		}
	}

	//newIdentifier := util.HashIdentifier(fmt.Sprintf("spotify-%s-%s", _artiste, fetchedSpotifyTrack.Title))

	//if err != nil {
	//	log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not marshal track: %v\n", err)
	//	return nil, err
	//}
	err = red.Set(context.Background(), trackCacheKey, serializedTrack, time.Hour*24).Err()
	// cache in redis. since this is for a track that we're just searching by the title and artiste,
	// we're saving the hash with a scheme of: "spotify-artist-title". e.g "spotify-taylor-swift-blink-182"
	// so we save the fetched track under that hash and assumed that was what the user searched for and wanted.
	//err = red.Set(context.Background(), newIdentifier, serializedTrack, 0).Err()

	if err != nil {
		log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not cache spotify track: %v\n", err)
	} else {
		log.Printf("\n[services][spotify][base][SearchTrackWithTitle] success - cached track: %v\n", fetchedSpotifyTrack.Title)
	}

	return &fetchedSpotifyTrack, nil
}

// SearchTrackWithID fetches a track using a track (entityID) and return a spotify track.
// this is typically expected to be used when the track we want to convert is the one
// the user wants to convert. i.e the track is what the user wants to convert
// and from the link, we can get the trackID.
// Basically, the platform the user is trying to convert from.
func SearchTrackWithID(id string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	// the cacheKey. scheme is "spotify:track_id"
	cacheKey := "spotify:track:" + id
	cachedTrack, err := red.Get(context.Background(), cacheKey).Result()

	if err != nil && err != redis.Nil {
		log.Printf("\n[services][SearchTrackWithID] error - Could not fetch record from cache. This is an unexpected error\n")
		return nil, err
	}

	// we have not cached this track before
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
			URL:           results.ExternalURLs["spotify"],
			Artists:       artistes,
			Released:      results.Album.ReleaseDate,
			Duration:      util.GetFormattedDuration(results.Duration / 1000),
			DurationMilli: results.Duration,
			Explicit:      results.Explicit,
			Title:         results.Name,
			Preview:       results.PreviewURL,
			Album:         results.Album.Name,
			ID:            results.ID.String(),
			Cover:         results.Album.Images[0].URL,
		}

		serialized, err := json.Marshal(out)
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchSingleTrack] error - could not serialize track: %v\n", err)
		}

		err = red.Set(context.Background(), cacheKey, serialized, time.Hour*24).Err()
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
// This function caches each of the tracks in the playlist, the playlist snapshop id in the scheme: "spotify:snapshot:id"
// and the playlist id in the scheme: "spotify:playlist:id"
func FetchPlaylistTracksAndInfo(id string, red *redis.Client) (*blueprint.PlaylistSearchResult, *blueprint.Pagination, error) {
	//client := createNewSpotifyUInstance()
	token := FetchNewAuthToken()
	ctx := context.Background()
	if token == nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistTracksAndInfo] error - could not fetch token\n")
		return nil, nil, errors.New("could not fetch token")
	}
	httpClient := spotifyauth.New().Client(ctx, token)
	client := spotify.New(httpClient)

	options := spotify.Fields("description,uri,external_urls,snapshot_id,name,images")

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

	// if we have not cached this playlist before or the snapshot id has changed (i.e. the playlist has been updated)
	// then we want to fetch the tracks and cache them.
	if cacheErr != nil && cacheErr == redis.Nil || cachedSnapshotID != info.SnapshotID {

		playlist, err := client.GetPlaylistItems(ctx, spotify.ID(id))
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Could not fetch playlist from spotify: %v\n", err)
			return nil, nil, err
		}
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - playlist fetched from spotify: %v\n", len(playlist.Items))

		// fetch ALL the pages
		for page := 1; ; page++ {
			out := &spotify.PlaylistItemPage{}
			paginationErr := client.NextPage(ctx, out)
			if paginationErr == spotify.ErrNoMorePages {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - No more pages for playlist\n")
				break
			}
			if paginationErr != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistTracksAndInfo] error - could not fetch playlist: %v\n", err)
				return nil, nil, err
			}
			playlist.Items = append(playlist.Items, out.Items...)
		}

		var tracks []blueprint.TrackSearchResult

		// calculate the duration of the playlist
		playlistLength := 0
		for _, track := range playlist.Items {
			var artistes []string
			for _, artist := range track.Track.Track.Artists {
				artistes = append(artistes, artist.Name)
			}

			playlistLength += track.Track.Track.Duration / 1000

			var cover string
			if len(track.Track.Track.Album.Images) > 0 {
				cover = track.Track.Track.Album.Images[0].URL
			}

			trackCopy := blueprint.TrackSearchResult{
				URL:           track.Track.Track.ExternalURLs["spotify"],
				Artists:       artistes,
				Released:      track.Track.Track.Album.ReleaseDate,
				Duration:      util.GetFormattedDuration(track.Track.Track.Duration / 1000),
				DurationMilli: track.Track.Track.Duration,
				Explicit:      track.Track.Track.Explicit,
				Title:         track.Track.Track.Name,
				Preview:       track.Track.Track.PreviewURL,
				Album:         track.Track.Track.Album.Name,
				ID:            track.Track.Track.ID.String(),
				Cover:         cover,
			}
			tracks = append(tracks, trackCopy)
			// cache the track. the scheme is: "spotify:track_id"
			cacheKey := "spotify:track:" + track.Track.Track.ID.String()
			serialized, err := json.Marshal(trackCopy)
			if err != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not serialize track: %v\n", err)
			}
			err = red.Set(context.Background(), cacheKey, string(serialized), time.Hour*24).Err()
			if err != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not cache track: %v\n", err)
			} else {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] success - track %s by %s has been cached\n", trackCopy.Title, trackCopy.Artists[0])
			}
		}

		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - playlist trcaks length: %v\n", len(tracks))

		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Owner info is: %v\n", info.Owner)

		playlistResult := blueprint.PlaylistSearchResult{
			URL:    info.ExternalURLs["spotify"],
			Tracks: tracks,
			Title:  info.Name,
			Length: util.GetFormattedDuration(playlistLength),
			Owner:  info.Owner.DisplayName,
			Cover:  info.Images[0].URL,
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
		// FIXME: unhandled slice index
		go SearchTrackWithTitleChan(t.Title, t.Artistes[0], ch, &wg, red)
		outputTrack := <-ch
		// for some reason, there is no spotify url which means could not fetch track, we
		// want to add to the list of "not found" tracks.
		if outputTrack == nil || outputTrack.URL == "" {
			// log info about empty track
			log.Printf("\n[services][spotify][base][FetchPlaylistSearchResult][warn] - Could not find track for %s\n", t.Title)
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Title: t.Title,
				URL:   t.URL,
				// WARNING: unhandled slice index
				Artistes: t.Artistes,
			})
			continue
		}

		fetchedTracks = append(fetchedTracks, *outputTrack)
	}

	wg.Wait()
	return &fetchedTracks, &omittedTracks
}

// FetchPlaylistSearchResult fetches the track for a playlist based on the search result
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
	track, omittedTracks := FetchTracks(trackSearch, red)
	return track, omittedTracks
}

func FetchPlaylistHash(playlistId string) []byte {
	token := FetchNewAuthToken()
	if token == nil {
		return nil
	}
	httpClient := spotifyauth.New().Client(context.Background(), token)
	client := spotify.New(httpClient)
	opts := spotify.Fields("snapshot_id")

	info, err := client.GetPlaylist(context.Background(), spotify.ID(playlistId), opts)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistHash] error - could not fetch playlist: %v\n", err)
		return nil
	}

	return []byte(info.SnapshotID)
}

// FetchUserPlaylist fetches the user's playlist
func FetchUserPlaylist(token string) ([]spotify.SimplePlaylist, error) {
	log.Printf("\n[services][spotify][base][FetchUserPlaylist] - token is: %v\n", token)
	httpClient := spotifyauth.New().Client(context.Background(), &oauth2.Token{RefreshToken: token})
	client := spotify.New(httpClient)
	playlists, err := client.CurrentUsersPlaylists(context.Background())
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchUserPlaylist] error - could not fetch playlist: %v\n", err)
		return nil, err
	}
	for {
		out := spotify.SimplePlaylistPage{}
		paginationErr := client.NextPage(context.Background(), &out)
		if paginationErr == spotify.ErrNoMorePages {
			log.Printf("\n[services][spotify][base][FetchUserPlaylist] - no more pages. User's full playlist retrieved\n")
			break
		}
		if paginationErr != nil {
			log.Printf("\n[services][spotify][base][FetchUserPlaylist] error - could not fetch playlist: %v\n", err)
			return nil, paginationErr
		}
		playlists.Playlists = append(playlists.Playlists, out.Playlists...)
	}

	return playlists.Playlists, nil
}
