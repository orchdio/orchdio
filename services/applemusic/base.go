package applemusic

import (
	"context"
	"github.com/go-redis/redis/v8"
	"log"
	"orchdio/blueprint"
)

func SearchTrackWithLink(info *blueprint.LinkInfo, red *redis.Client) *blueprint.TrackSearchResult {

	cacheKey := "applemusic:" + info.EntityID
	_, err := red.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from cache: %v\n", err)
		return nil
	}

	if err != nil && err == redis.Nil {

	}
	return nil
}
