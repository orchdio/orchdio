package platforms

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"log"
	"net/http"
	"os"
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
	//Tracks  []map[string]*[]blueprint.TrackSearchResult `json:"tracks"`
	Tracks struct{
		Deezer  *[]blueprint.TrackSearchResult `json:"deezer"`
		Spotify *[]blueprint.TrackSearchResult `json:"spotify"`
	} `json:"tracks"`
	Length  string              `json:"length"`
	Title   string              `json:"title"`
	Preview string              `json:"preview,omitempty"` // if no preview, not important to be bothered for now, API doesn't have to show it
	Pagination blueprint.Pagination `json:"pagination"`
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
		playlistInfoURL := fmt.Sprintf("%s/playlist/%s?limit=1", os.Getenv("DEEZER_API_BASE"), linkInfo.EntityID)
		deezerPlaylist, err := deezer.FetchPlaylistInfo(playlistInfoURL)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist] error - could not fetch playlist info from deezer: %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}

		index := ctx.Query("index", "none")
		link := linkInfo.TargetLink
		if index != "none" {
			link = fmt.Sprintf("%s&index=%s", link, index)
		}
		var playlistTracks, dzPagination, tracklistErr = deezer.FetchPlaylistTracklist(link)
		if tracklistErr != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist][error] - Could not fetch tracklist from deezer %v\n", tracklistErr)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}


		// then for each of these playlists, search for the tracks on spotify
		var trackTitles []string
		for _, tracks := range *playlistTracks {
			trackTitles = append(trackTitles, tracks.Title)
		}



		spotifyTracks := spotify.FetchTracks(trackTitles)
		convertedPlaylist := PlaylistConversion{
			URL:     deezerPlaylist.URL,
			Length:  deezerPlaylist.Length,
			Title:   deezerPlaylist.Title,
			Preview: "",
			Pagination: *dzPagination,
		}

		convertedPlaylist.Tracks.Deezer = playlistTracks
		convertedPlaylist.Tracks.Spotify = spotifyTracks

		log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v playlist with URL: %v\n",linkInfo.Platform, linkInfo.TargetLink)
		return util.SuccessResponse(ctx, http.StatusOK, convertedPlaylist)

	case spotify.IDENTIFIER:
		pagination := ctx.Query("pagination")
		entityID := linkInfo.EntityID
		spotifyPlaylist := blueprint.PlaylistSearchResult{}
		spPagination := blueprint.Pagination{}

		// THIS FEELS FUCKING HACKY. IT'S ALL SHADES OF WRONG
		// but, I am not sure how else to proceed for now
		// What this part does is that it checks if there's pagination and then fetch
		// the paginated playlist tracks. for example if its the third page, it fetches this
		// and converts the result into the standard *[]blueprint.TrackSearchResult
		// However, the request might be just for fetching the first page (normal playlist conversion)
		// So because of this, we're creating the empty variables, making the respective
		// requests and assigning the variables to the response of the result.
		if pagination != "" {
			log.Printf("\n[debug] - Its a pagination.. Extracted spotify ID")
			s1, s2, err := spotify.FetchNextPage(linkInfo.TargetLink)
			spotifyPlaylist = *s1
			spPagination = *s2
			if err != nil {
				log.Printf("\n[controllers][platforms][base] Error fetching next page from spotify")
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
			}
		} else {
			s1, s2, err := spotify.FetchPlaylistTracksAndInfo(entityID)
			if err != nil {
				log.Printf("\n[controllers][platforms][base] Error fetching next page from spotify")
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
			}
			spotifyPlaylist = *s1
			spPagination = *s2
		}

		deezerTracks := deezer.FetchPlaylistSearchResult(&spotifyPlaylist)
		convertedPlaylist := PlaylistConversion{
			URL:        spotifyPlaylist.URL,
			Title:      spotifyPlaylist.Title,
			Preview:    "",
			Pagination: spPagination,
		}

		convertedPlaylist.Tracks.Deezer = deezerTracks
		convertedPlaylist.Tracks.Spotify = &spotifyPlaylist.Tracks

		log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v playlist with URL: %v\n",linkInfo.Platform, linkInfo.TargetLink)
		return util.SuccessResponse(ctx, http.StatusOK, convertedPlaylist)
	}

	return util.ErrorResponse(ctx, http.StatusNotImplemented, "Not yet implemented")
}

