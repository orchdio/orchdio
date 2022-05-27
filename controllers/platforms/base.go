package platforms

import (
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"log"
	"net/http"
	"strings"
	"zoove/blueprint"
	"zoove/universal"
	"zoove/util"
)

// Platforms represents the structure for the platforms
type Platforms struct {
	Redis *redis.Client
}

func NewPlatform(r *redis.Client) *Platforms {
	return &Platforms{Redis: r}
}

// ConvertTrack returns the link to a track on several platforms
func (p *Platforms) ConvertTrack(ctx *fiber.Ctx) error {
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)

	// make sure we're actually handling for track alone, not playlist.
	if !strings.Contains(linkInfo.Entity, "track") {
		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", "Not a track URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Not a track entity")
	}

	conversion, err := universal.ConvertTrack(linkInfo, p.Redis)
	if err != nil {
		if err == blueprint.ENOTIMPLEMENTED {
			log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", "Not implemented")
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "Not implemented")
		}

		log.Printf("\n[controllers][platforms][base][ConvertTrack] - Could not convert track")
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v track with URL %v\n", linkInfo.Entity, linkInfo.TargetLink)

	// we want to cache the track inside redis here.

	return util.SuccessResponse(ctx, http.StatusOK, conversion)
}

// ConvertPlaylist retrieves info about a playlist from various platforms.
func (p *Platforms) ConvertPlaylist(ctx *fiber.Ctx) error {
	// first, we want to fetch the information on the link

	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)

	// make sure we're actually handling for track alone, not playlist.
	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("\n[controllers][platforms][ConvertTrack] error - %v\n", "Not a playlist URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Not a playlist entity")
	}

	convertedPlaylist, err := universal.ConvertPlaylist(linkInfo, p.Redis)

	if err != nil {
		if err == blueprint.ENOTIMPLEMENTED {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, err)
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][error] could not convert playlist %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	return util.SuccessResponse(ctx, http.StatusOK, convertedPlaylist)
}
