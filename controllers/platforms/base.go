package platforms

import (
	"github.com/gofiber/fiber/v2"
	"log"
	"net/http"
	"strings"
	"zoove/blueprint"
	"zoove/services/deezer"
	"zoove/services/spotify"
	"zoove/util"
)

// Conversion represents the final response for a typical track conversion
type Conversion struct {
	Entity    string `json:"entity"`
	Platforms struct {
		Deezer  *blueprint.TrackSearchResult `json:"deezer"`
		Spotify *blueprint.TrackSearchResult `json:"spotify"`
	} `json:"platforms"`
}


type PlaylistConversion struct {
	URL     string              `json:"url"`
	Tracks  []map[string]*[]blueprint.TrackSearchResult `json:"tracks"`
	Length  string              `json:"length"`
	Title   string              `json:"title"`
	Preview string              `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
}

// ConvertTrack returns the link to a track on several platforms
func ConvertTrack(ctx *fiber.Ctx) error {
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)

	// make sure we're actually handling for track alone, not playlist.
	if !strings.Contains(linkInfo.Entity, "track") {
		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", "Not a track URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Not a track entity")
	}
	conversion := &Conversion{}
	conversion.Entity = "track"
	switch linkInfo.Platform {
	case deezer.IDENTIFIER:
		dzSingleTrack := deezer.SearchTrackWithLink(linkInfo.TargetLink)
		trackTitle := deezer.ExtractTitle(dzSingleTrack.Title)

		spSingleTrack, err := spotify.SearchTrackWithTitle(trackTitle)

		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Deezer = dzSingleTrack

		// if there is no result from spotify search for whatever reason, we return empty
		if err != nil {
			if err == blueprint.ENORESULT {
				conversion.Platforms.Spotify = nil
			}
		}

		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] api-call — URL %v converted and returned for spotify and deezer\n", linkInfo.TargetLink)
		return util.SuccessResponse(ctx, http.StatusOK, conversion)

	case spotify.IDENTIFIER:
		spSingleTrack, err := spotify.SearchTrackWithID(linkInfo.EntityID)
		if err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not search track with ID from spotify: %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}

		dzSingleTrack, err := deezer.SearchTrackWithTitle(spSingleTrack.Title, spSingleTrack.Artistes[0])
		if err != nil {
			log.Printf("\n[controllers][platforms][spotify][ConvertTrack] error - could not search track with title on deezer")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}

		conversion.Platforms.Spotify = spSingleTrack
		conversion.Platforms.Deezer = dzSingleTrack
		log.Printf("\n[controllers][platforms][spotify][ConvertTrack] api-call — URL %v converted and returned for spotify and deezer\n", linkInfo.TargetLink)
		return util.SuccessResponse(ctx, http.StatusOK, conversion)
	}

	log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v track with URL %v\n", linkInfo.Entity, linkInfo.TargetLink)
	return util.ErrorResponse(ctx, http.StatusNotImplemented, nil)
}

// ConvertPlaylist retrieves info about a playlist from various platforms.
func ConvertPlaylist(ctx *fiber.Ctx) error {
	// first, we want to fetch the information on the link

	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)

	// make sure we're actually handling for track alone, not playlist.
	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("\n[controllers][platforms][ConvertTrack] error - %v\n", "Not a playlist URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Not a playlist entity")
	}

	switch linkInfo.Platform {
	case deezer.IDENTIFIER:
		deezerPlaylist, err := deezer.FetchPlaylistTracksAndInfo(linkInfo.TargetLink)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist] error - could not fetch playlist info from deezer: %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}
		// TODO: do for other platform

		// then for each of these playlists, search for the tracks on spotify
		var trackTitles []string

		for _, deezerTrack := range deezerPlaylist.Tracks {
			trackTitles = append(trackTitles, deezerTrack.Title)
		}

		spotifyTracks := spotify.FetchTracks(trackTitles)
		var allTracks []map[string]*[]blueprint.TrackSearchResult

		allTracks = append(allTracks, map[string]*[]blueprint.TrackSearchResult{
			"spotify": spotifyTracks,
			"deezer": &deezerPlaylist.Tracks,
		})

		convertedPlaylist := PlaylistConversion{
			URL:     deezerPlaylist.URL,
			Tracks:  allTracks,
			Length:  deezerPlaylist.Length,
			Title:   deezerPlaylist.Title,
			Preview: "",
		}

		log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v playlist with URL: %v\n",linkInfo.Platform, linkInfo.TargetLink)
		return util.SuccessResponse(ctx, http.StatusOK, convertedPlaylist)

	case spotify.IDENTIFIER:
		spotifyPlaylist, err := spotify.FetchPlaylistTracksAndInfo(linkInfo.EntityID)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertTrack] - could not fetch playlist track and info")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}

		var deezerTrackSearch []blueprint.DeezerSearchTrack
		for _, spotifyTrack := range spotifyPlaylist.Tracks {
			//trackTitles = append(trackTitles, map[string]string{"artiste": spotifyTrack.Artistes[0], "track": spotifyTrack.Title})
			deezerTrackSearch = append(deezerTrackSearch, blueprint.DeezerSearchTrack{
				Artiste: spotifyTrack.Artistes[0],
				Title:   spotifyTrack.Title,
			})
		}

		deezerTracks := deezer.FetchTracks(deezerTrackSearch)
		var allTracks []map[string]*[]blueprint.TrackSearchResult
		
		allTracks = append(allTracks, map[string]*[]blueprint.TrackSearchResult{
			"spotify": &spotifyPlaylist.Tracks,
			"deezer": deezerTracks,
		})
		
		convertedPlaylist := PlaylistConversion{
			URL:     spotifyPlaylist.URL,
			Tracks:  allTracks,
			Title:   spotifyPlaylist.Title,
		}

		log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v playlist with URL: %v\n",linkInfo.Platform, linkInfo.TargetLink)
		return util.SuccessResponse(ctx, http.StatusOK, convertedPlaylist)
	}

	return util.ErrorResponse(ctx, http.StatusNotImplemented, "Not yet implemented")
}
