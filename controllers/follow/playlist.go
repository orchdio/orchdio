package follow

import (
	"database/sql"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/services"
	"orchdio/util"
	"strings"
)

type Controller struct {
	DB  *sqlx.DB
	Red *redis.Client
}

func NewController(db *sqlx.DB, red *redis.Client) *Controller {
	return &Controller{
		DB:  db,
		Red: red,
	}
}

func (c *Controller) FollowPlaylist(ctx *fiber.Ctx) error {
	log.Printf("[controller][follow][FollowPlaylist] - follow playlist")
	var platforms = []string{"tidal", "spotify", "deezer"}

	user := ctx.Locals("user").(*blueprint.User)
	var subscriberBody = struct {
		Users []string `json:"users"`
		Url   string   `json:"url"`
	}{}
	err := ctx.BodyParser(&subscriberBody)

	if err != nil {
		log.Printf("[controller][follow][FollowPlaylist] - error parsing body: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not follow playlist. Invalid body passed")
	}

	if len(subscriberBody.Users) > 20 {
		log.Printf("[controller][follow][FollowPlaylist] - too many subscribers. Max is 20")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "large subscriber body", "too many subscribers. Maximum is 20")
	}
	for _, subscriber := range subscriberBody.Users {
		if !util.IsValidUUID(subscriber) {
			log.Printf("[controller][follow][FollowPlaylist] - error parsing subscriber uuid: %v", err)
			return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid subscriber uuid", "Invalid subscriber id present. Please make sure all subscribers are uuid format")
		}
	}

	linkInfo, err := services.ExtractLinkInfo(subscriberBody.Url)
	if err != nil {
		log.Printf("[controller][follow][FollowPlaylist] - error extracting link info: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not extract link information.")
	}

	_ = strings.ToLower(linkInfo.Platform)
	if !lo.Contains(platforms, linkInfo.Platform) {
		log.Printf("[controller][follow][FollowPlaylist] - platform not supported")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid platform", "platform not supported. Please make sure the tracks are from the supported platforms.")
	}

	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("[controller][conversion][playlist] - not a playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "not a playlist", "It seems your didnt pass a playlist url. Please check your url again")
	}

	follow := NewFollow(c.DB, c.Red)

	followId, err := follow.FollowPlaylist(user.UUID.String(), subscriberBody.Url, linkInfo, subscriberBody.Users)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[controller][follow][FollowPlaylist] - error following playlist: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not follow playlist")
	}

	// if the error returned is sql.ErrNoRows, it means that the playlist is already followed
	//and the length of subscribers passed in the request body is 1
	if err == blueprint.EALREADY_EXISTS {
		log.Printf("[controller][follow][FollowPlaylist] - playlist already followed")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Already followed", "playlist already followed")
	}

	res := map[string]interface{}{"follow_id": string(followId)}
	return util.SuccessResponse(ctx, http.StatusOK, res)
}
