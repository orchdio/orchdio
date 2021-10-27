package spotify

import (
	"context"
	"fmt"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
	"log"
	"os"
	"sync"
	"zoove/blueprint"
	"zoove/util"
)

// createNewSpotifyUInstance creates a new spotify client to make API request that doesn't need user auth
func createNewSpotifyUInstance() *spotify.Client {
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
	return client
}

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


func SearchTrackWithTitleChan(title string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup) {
	result, err := SearchTrackWithTitle(title)
	if err != nil {
		log.Printf("\nError fetching track %s with channels\n. Error: %v", title, err)
		c <- nil
		wg.Add(1)

		defer wg.Done()
		return
	}
	log.Printf("\nChann running for: %s\n", title)
	c <- result
	wg.Add(1)

	defer wg.Done()
	return
}

// SearchTrackWithTitle searches spotify using the title of a track
func SearchTrackWithTitle(title string) (*blueprint.TrackSearchResult, error) {
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

	spHr := (spSingleTrack.Duration / 1000) / 60
	spSec := (spSingleTrack.Duration / 1000) % 60

	fetchedSpotifyTrack := blueprint.TrackSearchResult{
		Released: spSingleTrack.Album.ReleaseDate,
		URL:      spSingleTrack.SimpleTrack.ExternalURLs["spotify"],
		Artistes: spTrackContributors,
		Duration: fmt.Sprintf("%d:%d", spHr, spSec),
		Explicit: spSingleTrack.Explicit,
		Title:    spSingleTrack.Name,
		Preview:  spSingleTrack.PreviewURL,
	}

	return &fetchedSpotifyTrack, nil
}

// SearchTrackWithID fetches a track using a track (entityID) and return a spotify track.
func SearchTrackWithID(id string) (*blueprint.TrackSearchResult, error) {
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
	}

	return &out, nil
}

func FetchPlaylistTracksAndInfo(id string) (*blueprint.PlaylistSearchResult, error) {
	client := createNewSpotifyUInstance()
	playlist, err := client.GetPlaylist(context.Background(), spotify.ID(id))
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Could not fetch playlist from spotify: %v\n", err)
		return nil, err
	}
	var tracks []blueprint.TrackSearchResult

	for _, track := range playlist.Tracks.Tracks {
		var artistes []string
		for _, artist := range track.Track.Artists {
			artistes = append(artistes, artist.Name)
		}

		 trackCopy := blueprint.TrackSearchResult{
			 URL:      track.Track.ExternalURLs["spotify"],
			 Artistes: artistes,
			 Released: track.Track.Album.ReleaseDate,
			 Duration: util.GetFormattedDuration(track.Track.Duration / 1000),
			 Explicit: track.Track.Explicit,
			 Title:    track.Track.Name,
			 Preview:  track.Track.PreviewURL,
		 }
		 tracks = append(tracks, trackCopy)
	}

	playlistResult := blueprint.PlaylistSearchResult{
		URL:     playlist.ExternalURLs["spotify"],
		Tracks:  tracks,
		Title:   playlist.Name,
	}
	return &playlistResult, nil
}

// FetchTracks fetches tracks in a playlist
func FetchTracks(tracks []string) *[]blueprint.TrackSearchResult {
	var fetchedTracks []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, title := range tracks {
		go SearchTrackWithTitleChan(title, ch, &wg)
		outputTracks := <-ch

		if outputTracks == nil {
			log.Printf("\n[spotify][chan] No result found")
			continue
		}
		fetchedTracks = append(fetchedTracks, *outputTracks)
	}
	wg.Wait()
	return &fetchedTracks
}


















