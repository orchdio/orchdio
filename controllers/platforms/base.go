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
	Entity string `json:"entity"`
	Platforms struct{
		Deezer *blueprint.TrackSearchResult  `json:"deezer"`
		Spotify *blueprint.TrackSearchResult `json:"spotify"`
	} `json:"platforms"`
}
type PlaylistConversion struct {
	Entity string `json:"entity"`
	Platforms struct{
		Deezer *blueprint.PlaylistSearchResult `json:"deezer"`
		Spotify *blueprint.PlaylistSearchResult `json:"spotify"`
	} `json:"platforms"`
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
		spSingleTrack , err := spotify.SearchTrackWithID(linkInfo.EntityID)
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

	playlistConversion := &PlaylistConversion{
		Entity: "playlist",
	}
	switch linkInfo.Platform {
	case deezer.IDENTIFIER:
		singlePlaylist, err := deezer.FetchPlaylistFromLink(linkInfo.TargetLink)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertPlaylist] error - could not fetch playlist info from deezer: %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}
		// TODO: do for other platform
		playlistConversion.Platforms.Deezer = singlePlaylist
		return util.SuccessResponse(ctx, http.StatusOK, playlistConversion)
	}


	return util.ErrorResponse(ctx, http.StatusNotImplemented, "Not yet implemented")
}