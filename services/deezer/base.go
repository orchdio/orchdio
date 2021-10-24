package deezer

import (
	"encoding/json"
	"github.com/vicanso/go-axios"
	"log"
	"strings"
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