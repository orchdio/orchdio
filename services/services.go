package services

import (
	"fmt"
	"github.com/badoux/goscraper"
	"log"
	"net/url"
	"os"
	"strings"
	"zoove/blueprint"
	"zoove/services/deezer"
	"zoove/services/spotify"
	"zoove/util"
)



// ExtractLinkInfo extracts a URL from a URL.
func ExtractLinkInfo(t string) (*blueprint.LinkInfo, error) {
	// first, check if the link is a shortlink

	/**
	* first, get the host of the URL. Typically, its https://www.deezer.com/en/artist/4037971
	* with the format in the way of: https://www.deezer.com/:country_locale_shortcode/:entity/:id
	* where country_locale_shortcode is the two-letter shortcode for the country/region the account
	* or resource is located, entity being either of artist, user or track. The link can also come with
	* some tracking parameters/queries but not exactly a major focus with Deezer, for the most part at least
	* for now.
	* Also, the URL can also come in the form of a shortlink. This shortlink (typically) changes with each
	* time a person copies it from Deezer.
	*
	* The URL for spotify is like: https://open.spotify.com/track/2I3dW2dCBZAJGj5X21E53k?si=ae80d72a4cc149ad
	* with the format in the way of: https://open.spotify.com/:entity/:id?:tracking_params
	* where entity is either of artiste, user or track (or something like that), the id being the ID of the
	* entity and tracking_params for tracking.
	 */

	song, escapeErr := url.QueryUnescape(t)
	if escapeErr != nil {
		log.Printf("\n[services][s: Track][error] Error escaping URL: %v\n", escapeErr)
		return nil, escapeErr
	}
	parsedURL, parseErr := url.Parse(song)
	if parseErr != nil {
		log.Printf("\n[services][s: Track][error] Error parsing escaped URL: %v\n", parseErr)
		return nil, parseErr
	}
	// index of the beginning of useless params/query (aka tracking links) in our link
	// if there are, then we want to remove them.
	trailingCharIndex := strings.Index(song, "?")
	if trailingCharIndex != -1 {
		song = song[:trailingCharIndex]
	}

	host := parsedURL.Host
	var (
		entityID string
		entity   = "track"
	)
	playlistIndex := strings.Index(song, "playlist")
	trackIndex := strings.Index(song, "track")
	switch host {
	case util.Find(blueprint.DeezerHost, host):
		// first, check the type of URL it is. for now, only track.
		if strings.Contains(song, "deezer.page.link") {
			// it contains a shortlink.
			previewResult, err := goscraper.Scrape(song, 10)
			if err != nil {
				log.Printf("\n[services][s: Track][error] could not retrieve preview of link: %v", previewResult)
				return nil, err
			}

			playlistIndex = strings.Index(previewResult.Preview.Link, "playlist")
			if playlistIndex != -1 {
				entityID = previewResult.Preview.Link[playlistIndex+9:]
				entity = "playlist"
			} else {
				trackIndex = strings.Index(previewResult.Preview.Link, "track")
				entityID = previewResult.Preview.Link[trackIndex + 6:]
			}
		} else {
			// it doesnt contain a preview URL and its a deezer track
			if playlistIndex != -1 {
				entityID = song[playlistIndex+9:]
				entity = "playlist"
			} else {
				entityID = song[trackIndex+6:]
			}
		}

		// then we want to return the real URL.
		linkInfo := &blueprint.LinkInfo{
			Platform:   deezer.IDENTIFIER,
			TargetLink: fmt.Sprintf("%s/%s/%s", os.Getenv("DEEZER_API_BASE"), entity, entityID),
			Entity:     entity,
			EntityID:   entityID,
		}
		return linkInfo, nil
	case blueprint.SpotifyHost:
		if playlistIndex != -1 {
			entityID = song[34:]
			entity = "playlists"
		} else {
			// then we rename the default entity to tracks, for spotify. because that's what
			// the URL scheme for spotify uses.
			entity = "tracks"
			entityID = song[31:]
		}

		// then we want to return the real URL.
		linkInfo := &blueprint.LinkInfo{
			Platform:   spotify.IDENTIFIER,
			TargetLink: fmt.Sprintf("%s/%s/%s", os.Getenv("SPOTIFY_API_BASE"), entity, entityID),
			Entity:     entity,
			EntityID:   entityID,
		}

		return linkInfo, nil
	default:
		log.Printf("\n[servies][s: Track][error] Link info could not be processed. Might be an invalid link")
		return nil, blueprint.EHOSTUNSUPPORTED
	}
}
