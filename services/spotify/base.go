package spotify

import (
	"context"
	"fmt"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
	"log"
	"os"
	"zoove/blueprint"
	"zoove/util"
)

func FetchSingleTrack(title string) *spotify.SearchResult {
	config := &clientcredentials.Config{
		ClientID: os.Getenv("SPOTIFY_ID"),
		ClientSecret: os.Getenv("SPOTIFY_SECRET"),
		TokenURL: spotifyauth.TokenURL,
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

// SearchTrackWithTitle searches spotify using the title of a track
func SearchTrackWithTitle(title string) (*blueprint.TrackSearchResult, error){
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
	spSec := (spSingleTrack.Duration/ 1000) % 60

	fetchedSpotifyTrack := blueprint.TrackSearchResult{
		Released: spSingleTrack.Album.ReleaseDate,
		URL: spSingleTrack.SimpleTrack.ExternalURLs["spotify"],
		Artistes: spTrackContributors,
		Duration: fmt.Sprintf("%d:%d", spHr, spSec),
		Explicit: spSingleTrack.Explicit,
		Title: spSingleTrack.Name,
		Preview: spSingleTrack.PreviewURL,
	}

	return &fetchedSpotifyTrack, nil
}

// SearchTrackWithID fetches a track using a track (entityID) and return a spotify track.
func SearchTrackWithID(id string) (*blueprint.TrackSearchResult, error) {
	config := &clientcredentials.Config{
		ClientID: os.Getenv("SPOTIFY_ID"),
		ClientSecret: os.Getenv("SPOTIFY_SECRET"),
		TokenURL: spotifyauth.TokenURL,
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
		Duration: util.GetFormattedDuration(results.Duration/1000),
		Explicit: results.Explicit,
		Title:    results.Name,
		Preview: results.PreviewURL,
	}

	return &out, nil
}