package spotify

import (
	"context"
	"github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2/clientcredentials"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"log"
	"os"
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