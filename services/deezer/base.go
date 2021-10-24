package deezer

import (
	"encoding/json"
	"fmt"
	"github.com/vicanso/go-axios"
	"log"
	"net/url"
	"os"
	"strings"
	"zoove/blueprint"
	"zoove/util"
)

type SearchInfo struct {

}
func ExtractTitle(title string) string {
	index := strings.Index(title, "(feat")
	if index == -1 {
		return title
	}
	return title[:index]
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

// FetchPlaylistFromLink returns a playlist information for a deezer playlist, including tracks with type of ``blueprint.TrackSearchResult``
func FetchPlaylistFromLink(link string) (*blueprint.PlaylistSearchResult, error){
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

	var sample []blueprint.TrackSearchResult
	var duration int
	for _, track := range playlist.Tracks.Data {

		trackCopy := blueprint.TrackSearchResult{
			URL:      track.URL,
			Artistes:  []string{track.Artistes.Name},
			Released: track.ReleaseDate,
			Duration: util.GetFormattedDuration(track.Duration),
			Explicit: track.Explicit,
			Title:    track.Title,
			Preview:  track.Preview,
		}
		duration += track.Duration
		sample = append(sample, trackCopy)
	}

	out := blueprint.PlaylistSearchResult{
		URL:     playlist.URL,
		Tracks:  sample,
		Length:  util.GetFormattedDuration(duration),
		Title:   playlist.Title,
		Preview: " ",
	}
	return &out, nil
}

// SearchTrackWithLink fetches the deezer result for the track being searched using the URL
func SearchTrackWithLink(link string) *blueprint.TrackSearchResult{
	dzSingleTrack, err := FetchSingleTrack(link)
	var dzTrackContributors []string
	for _, contributor := range dzSingleTrack.Contributors{
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
		URL: dzSingleTrack.Link,
		Artistes: dzTrackContributors,
		Released: dzSingleTrack.ReleaseDate,
		Title: dzSingleTrack.Title,
		Preview: dzSingleTrack.Preview,
	}
	return &fetchedDeezerTrack

}

// SearchTrackWithTitle searches for a track using the title (and artiste) on deezer
func SearchTrackWithTitle(title, artiste string) (*blueprint.TrackSearchResult, error) {
	log.Println(fmt.Sprintf("track:\"%s\" artist: \"%s\"", title, artiste))
	payload := url.QueryEscape(fmt.Sprintf("track:\"%s\" artist:\"%s\"", title, artiste))
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
		Preview: track.Preview,
	}

	return &out, nil
	}

	return nil, blueprint.ENORESULT
}

// SearchPlaylistWithID retrieves a playlist's info using the playlist ID
//func SearchPlaylistWithLink(id string) {
//	dzSingle
//}