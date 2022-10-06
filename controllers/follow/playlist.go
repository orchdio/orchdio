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
		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
	}

	if len(subscriberBody.Users) > 20 {
		log.Printf("[controller][follow][FollowPlaylist] - too many subscribers. Max is 20")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "too many subscribers. Max is 20")
	}

	linkInfo, err := services.ExtractLinkInfo(subscriberBody.Url)
	if err != nil {
		log.Printf("[controller][follow][FollowPlaylist] - error extracting link info: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
	}

	_ = strings.ToLower(linkInfo.Platform)
	if !lo.Contains(platforms, linkInfo.Platform) {
		log.Printf("[controller][follow][FollowPlaylist] - platform not supported")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "platform not supported")
	}

	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("[controller][conversion][playlist] - not a playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "not a playlist")
	}

	follow := NewFollow(c.DB, c.Red)

	followId, err := follow.FollowPlaylist(user.UUID.String(), subscriberBody.Url, linkInfo, subscriberBody.Users)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[controller][follow][FollowPlaylist] - error following playlist: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "error following playlist")
	}

	// if the error returned is sql.ErrNoRows, it means that the playlist is already followed
	//and the length of subscribers passed in the request body is 1
	if err == blueprint.EALREADY_EXISTS {
		log.Printf("[controller][follow][FollowPlaylist] - playlist already followed")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "playlist already followed")
	}

	res := map[string]interface{}{"follow_id": string(followId)}
	return util.SuccessResponse(ctx, http.StatusOK, res)
}
