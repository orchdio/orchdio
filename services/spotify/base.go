package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/vicanso/go-axios"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"log"
	"net/url"
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

	fetchedSpotifyTrack := blueprint.TrackSearchResult{
		Released: spSingleTrack.Album.ReleaseDate,
		URL:      spSingleTrack.SimpleTrack.ExternalURLs["spotify"],
		Artistes: spTrackContributors,
		Duration: util.GetFormattedDuration(spSingleTrack.Duration / 1000),
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

func FetchPlaylistTracksAndInfo(id string) (*blueprint.PlaylistSearchResult, *blueprint.Pagination, error) {
	client := createNewSpotifyUInstance()
	playlist, err := client.GetPlaylist(context.Background(), spotify.ID(id))
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Could not fetch playlist from spotify: %v\n", err)
		return nil, nil, err
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
	pagination := blueprint.Pagination{
		Next:     playlist.Tracks.Next,
		Previous: playlist.Tracks.Previous,
		Total:    playlist.Tracks.Total,
		Platform: "spotify",
	} 
	return &playlistResult, &pagination, nil
}

// FetchTracks fetches tracks in a playlist concurrently using channels
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

func FetchNextPage(link string) (*blueprint.PlaylistSearchResult, *blueprint.Pagination, error){
	token := fetchNewAuthToken()
	client := createNewSpotifyUInstance()
	// first, get the playlistInfo
	type requestOption struct {
		urlParams url.Values
	}
	options := spotify.Fields("description,uri")
	//options := spotify.RequestOption(requestOption{urlParams: map[string][]string{
	//	"fields": {"RequestOption"},
	//}})
	//options := RequestOption{
	//	urlParams: url.Values{
	//		"fields": {"description,uri"},
	//	},
	//}


	info , err := client.GetPlaylist(context.Background(), spotify.ID(link), options)
	paginatedPlaylist := PaginatedPlaylist{}
	axiosInstance := axios.NewInstance(&axios.InstanceConfig{
		Headers: map[string][]string{
			"Authorization": {fmt.Sprintf("Bearer %s", token.AccessToken)},
		},
	})

	response, err := axiosInstance.Get(link)
	if err != nil {
		log.Printf("Error fetching the next page.")
		return nil, nil, err
	}


	err = json.Unmarshal(response.Data, &paginatedPlaylist)
	if err != nil {
		log.Printf("\n[services][spotify][base] - Could not deserialize the body: %v\n", err)
		return nil, nil, err
	}
	var tracks []blueprint.TrackSearchResult

	for _, track := range paginatedPlaylist.Items {
        var artistes []string
        for _, artist := range track.Track.Artists {
            artistes = append(artistes, artist.Name)
        }

        var previewLink string
        if track.Track.PreviewUrl != nil {
        	previewLink = *track.Track.PreviewUrl
		}
         trackCopy := blueprint.TrackSearchResult{
             URL:      track.Track.ExternalUrls.Spotify,
             Artistes: artistes,
             Released: track.Track.Album.ReleaseDate,
             Duration: util.GetFormattedDuration(track.Track.DurationMs / 1000),
             Explicit: track.Track.Explicit,
             Title:    track.Track.Name,
             Preview: previewLink,
         }
         tracks = append(tracks, trackCopy)
    }

    pagination := blueprint.Pagination{
        Next:     paginatedPlaylist.Next,
        Previous: paginatedPlaylist.Previous,
        Total:    paginatedPlaylist.Total,
        Platform: "spotify",
    }

    playlistResult := blueprint.PlaylistSearchResult{
		URL:     info.ExternalURLs["spotify"],
		Tracks:  tracks,
		Title:   info.Name,
		Preview: "",
	}

    return &playlistResult, &pagination, nil
}
