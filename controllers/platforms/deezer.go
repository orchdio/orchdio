package platforms

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	spotify2 "github.com/zmb3/spotify/v2"
	"log"
	"net/http"
	"strings"
	"zoove/services/deezer"
	"zoove/services/spotify"
	"zoove/types"
	"zoove/util"
)

type Conversion struct {
	Entity string `json:"entity"`
	Platforms struct{
		Deezer []map[string]interface{} `json:"deezer"`
		Spotify []map[string]interface{} `json:"spotify"`
	} `json:"platforms"`
}

func ConvertTrack(ctx *fiber.Ctx) error {
	linkInfo := ctx.Locals("linkInfo").(*types.LinkInfo)
	// make sure we're actually handling for track alone, not playlist.
	if !strings.Contains(linkInfo.Entity, "track") {
		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", "Not a track URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Not a track entity")
	}
	conversion := &Conversion{}
	switch linkInfo.Platform {
	case strings.ToLower(deezer.IDENTIFIER):
		// if it's a deezer link, we want to fetch it and then use the info to search on other platforms too
		// but for now, lets just return it
		/**
		* {
		*   entity: track,
		*	platforms: {
		*		deezer: [{
		*			url: "",
		*			artistes: [],
		*			released: "",
		*			duration: ""
		*			explicit: true
		*
		*		}],
		*		spotify: [{
		*
				}]
		* 	 }
		* }
		 */
		dzSingleTrack, err := deezer.FetchSingleTrack(linkInfo.TargetLink)
		var dzTrackContributors []string
		for _, contributor := range dzSingleTrack.Contributors{
			if contributor.Type == "artist" {
				dzTrackContributors = append(dzTrackContributors, contributor.Name)
			}
		}


		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}
		// FIXME: perhaps properly handle this error
		hour := dzSingleTrack.Duration / 60
		sec := dzSingleTrack.Duration % 60
		explicit := false
		if dzSingleTrack.ExplicitContentLyrics == 1 {
			explicit = true
		}
		// conversion.Entity = "track"
		fetchedDeezerTrack := []map[string]interface{}{{
			"url": dzSingleTrack.Link,
			// for now, we're returning just the artiste name, not link
			"artistes": dzTrackContributors,
			"released": dzSingleTrack.ReleaseDate,
			"duration": fmt.Sprintf("%d:%d", hour, sec),
			"explicit": explicit,
		}}
		conversion.Entity = "track"
		conversion.Platforms.Deezer = fetchedDeezerTrack

		// TODO: now do same for spotify here

		trackTitle := deezer.ExtractTitle(dzSingleTrack.Title)
		spotifySearch := spotify.FetchSingleTrack(trackTitle)
		if spotifySearch == nil {
			log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - error fetching single track on spotify\n")
			// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		}

		var spSingleTrack spotify2.FullTrack

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

		fetchedSpotifyTrack := []map[string]interface{}{{
			"url": spSingleTrack.SimpleTrack.ExternalURLs["spotify"],
			"artistes": spTrackContributors,
			"released": spSingleTrack.Album.ReleaseDate,
			"duration": fmt.Sprintf("%d:%d", spHr, spSec),
			"explicit": spSingleTrack.Explicit,
		}}

		conversion.Platforms.Spotify = fetchedSpotifyTrack
		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] api-call — URL %v converted and returned for spotify and deezer\n", linkInfo.TargetLink)
		return util.SuccessResponse(ctx, http.StatusOK, conversion)
	}

	return util.ErrorResponse(ctx, http.StatusNotImplemented, nil)
}
