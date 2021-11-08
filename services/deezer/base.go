package deezer

import (
	"encoding/json"
	"fmt"
	"github.com/vicanso/go-axios"
	"log"
	"net/url"
	"os"
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

//func extractArtistes(contributors Track) []string {
//	var _contributors []string
//
//	for _, contributor := range contributors.Contributors {
//		if contributor.Type == "artist" {
//			_contributors = append(_contributors, contributor.Name)
//		}
//	}
//	return _contributors
//}

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

// FetchPlaylistInfo fetches the playlist info.
func FetchPlaylistInfo(link string) (*blueprint.PlaylistSearchResult, error) {
	response, err := axios.Get(link)
	if err != nil {
		log.Printf("\n[services][deezer][playlist][FetchPlaylist] error - could not fetch playlist: %v\n", err)
		return nil, err
	}
	playlist := &Playlist{}
	err = json.Unmarshal(response.Data, playlist)

	if err != nil {
		log.Printf("\n[services][deezer][playlist][FetchPlaylist] error - could not deserialize response into output: %v\n", err)
		return nil, err
	}

	info := blueprint.PlaylistSearchResult{
		URL:     playlist.URL,
		Length:  util.GetFormattedDuration(playlist.Duration),
		Title:   playlist.Title,
		Preview: "",
	}
	return &info, nil
}


// SearchTrackWithLink fetches the deezer result for the track being searched using the URL
func SearchTrackWithLink(link string) *blueprint.TrackSearchResult {
	dzSingleTrack, err := FetchSingleTrack(link)
	var dzTrackContributors []string
	for _, contributor := range dzSingleTrack.Contributors {
		if contributor.Type == "artist" {
			dzTrackContributors = append(dzTrackContributors, contributor.Name)
		}
	}

	if err != nil {
		// FIXME: do something.
		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", err)
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
	}
	return &fetchedDeezerTrack

}

// SearchTrackWithTitle searches for a track using the title (and artiste) on deezer
func SearchTrackWithTitle(title, artiste string) (*blueprint.TrackSearchResult, error) {
	trackTitle := ExtractTitle(title)
	payload := url.QueryEscape(fmt.Sprintf("track:\"%s\" artist:\"%s\"", strings.Trim(trackTitle, " ") , strings.Trim(artiste, " ")))
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
		}

		return &out, nil
	}

	return nil, blueprint.ENORESULT
}

func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup) {
	result, err := SearchTrackWithTitle(title, artiste)
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
func FetchTracks(tracks []blueprint.DeezerSearchTrack) *[]blueprint.TrackSearchResult {
	var fetchedTracks []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		go SearchTrackWithTitleChan(track.Title, track.Artiste, ch, &wg)

		outputTracks := <- ch
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
func FetchPlaylistTracklist(link string) (*[]blueprint.TrackSearchResult, *blueprint.Pagination, error) {
	tracks, err := axios.Get(link)
	if err != nil {
		return nil,nil, err
	}
	var trackList PlaylistTracksSearch
	err = json.Unmarshal(tracks.Data, &trackList)
	if err != nil {
		log.Println("Error deserializing result of playlist tracks search")
		return nil,nil, err
	}

	var out []blueprint.TrackSearchResult
	for _, track := range trackList.Data {
		result := &blueprint.TrackSearchResult{
			URL:      track.Link,
			Artistes: []string{track.Artist.Name},
			//Released: track.r,
			Duration: util.GetFormattedDuration(track.Duration),
			Explicit: util.DeezerIsExplicit(track.ExplicitContentLyrics),
			Title:    track.Title,
			Preview:  track.Preview,
		}
		out = append(out, *result)
	}
	pagination := &blueprint.Pagination{
		Next:     trackList.Next,
		Previous: trackList.Previous,
		Total:    trackList.Total,
		Platform: "deezer",
	}
	return &out, pagination, nil
}

func FetchPlaylistSearchResult(p *blueprint.PlaylistSearchResult) *[]blueprint.TrackSearchResult {
	var deezerTrackSearch []blueprint.DeezerSearchTrack
	for _, spotifyTrack := range p.Tracks {
		deezerTrackSearch = append(deezerTrackSearch, blueprint.DeezerSearchTrack{
			Artiste: spotifyTrack.Artistes[0],
			Title:   spotifyTrack.Title,
		})
	}

	deezerTracks := FetchTracks(deezerTrackSearch)
	return deezerTracks
}