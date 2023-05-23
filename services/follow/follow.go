package follow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/services"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/universal"
	"time"
)

type Follow struct {
	DB  *sqlx.DB
	Red *redis.Client
}

// NewFollow returns a new follow struct.
func NewFollow(db *sqlx.DB, red *redis.Client) *Follow {
	return &Follow{
		DB:  db,
		Red: red,
	}
}

// FollowPlaylist follows a playlist. It will check if the follow already exists. If it exists, then
// we want to add the subscriber to the follow. If the subscriber has already followed the playlist,
// then we do nothing. If it doesn't exist, then we create a new follow and add the subscriber.
func (f *Follow) FollowPlaylist(developer, app, originalURL string, info *blueprint.LinkInfo, subscribers []string) ([]byte, error) {
	log.Printf("[follow][FollowPlaylist] - Running follow playlist")
	if len(subscribers) > 20 {
		log.Printf("[follow][FollowPlaylist] - too many subscribers. Max is 20")
		return nil, blueprint.ERRTOOMANY
	}
	// this function takes the playlist id and the user id and checks if  the user has
	// already been subscribed to the playlist. if they have, we don't need to do anything.
	// if they haven't, we need to subscribe them to the playlist by simply upserting.
	database := db.NewDB{DB: f.DB}
	rows, err := database.FetchFollowTask(info.EntityID)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[follow][FollowPlaylist] - no follow created for this entity (playlist)")
		} else {
			log.Printf("[follow][FollowPlaylist] - error fetching follow task: %v", err)
			return nil, err
		}
	}

	uniqueId, _ := uuid.NewUUID()

	if rows == nil {
		subs := pq.Array(subscribers)
		// TODO: pass taskID
		followId, err := database.CreateFollowTask(developer, app, uniqueId.String(), info.EntityID, originalURL, subs)
		if err != nil {
			log.Printf("[follow][FollowPlaylist] - error creating follow task: %v", err)
			return nil, err
		}
		log.Printf("[follow][FollowPlaylist] - created follow task: %v", string(followId))
		return followId, nil
	}

	var updateFollowByte []byte

	// FIXME: implement inserting the subscribers in array
	for _, subscriber := range subscribers {
		log.Printf("[follow][FollowPlaylist] - adding subscriber: %v", subscriber)
		updateFollowByte, err = database.UpdateFollowSubscriber(subscriber, info.EntityID)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("[follow][FollowPlaylist] - error updating follow subscriber: %v", err)
		}

		// if the subscriber is already subscribed, the error will be sql.ErrNoRows
		if err == sql.ErrNoRows {
			log.Printf("[follow][FollowPlaylist] - subscriber already exists")
		}
	}

	// if the follower passed is a single value, and the subscriber already follows the playlist,
	// then the length of the updateFollowByte will be 0. Only in this case do we send a "already exists"
	// error.
	// TODO: emphasis in docs that if a user already follows a playlist and they follow it again, it will
	// simply not do anything and return an ok.
	if len(updateFollowByte) == 0 {
		return nil, blueprint.EALREADY_EXISTS
	}

	log.Printf("[follow][FollowPlaylist] - updated follow subscriber: %v", updateFollowByte)
	return updateFollowByte, nil
}

type TaskCronHandler struct {
	DB  *sqlx.DB
	Red *redis.Client
}

func NewTaskCronHandler(db *sqlx.DB, red *redis.Client) *TaskCronHandler {
	return &TaskCronHandler{
		DB:  db,
		Red: red,
	}
}

// ProcessFollowTaskHandler is a handler for the follow task. It'll check if a playlist has been updated. If the
// playlist has been updated, we convert it again (to fetch the updated one) and send the updated playlist to the subscribers as notification.
func (s *TaskCronHandler) ProcessFollowTaskHandler(ctx context.Context, task *asynq.Task) error {
	log.Printf("[queue][ProcessFollowTaskHandler] - processing follow task")
	var data blueprint.FollowTaskData
	err := json.Unmarshal(task.Payload(), &data)
	if err != nil {
		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error unmarshalling task payload: %v", err)
		return err
	}

	log.Printf("[queue][ProcessFollowTaskHandler] - task data: %v", data)

	// fetch the app that made the request
	database := db.NewDB{DB: s.DB}
	app, err := database.FetchAppByAppId(data.App)
	if err != nil {
		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error fetching app: could not fetch app with app and developer id. please make sure they are correct %v", err)
		return err
	}

	var outBytes []byte
	switch data.Platform {
	case spotify.IDENTIFIER:
		outBytes = app.SpotifyCredentials
	case tidal.IDENTIFIER:
		outBytes = app.TidalCredentials
	case deezer.IDENTIFIER:
		outBytes = app.DeezerCredentials
	}

	var credentials blueprint.IntegrationCredentials
	err = json.Unmarshal(outBytes, &credentials)
	if err != nil {
		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - could not deserialize app integration credentials. Maybe a field is empty: %v", err)
		return err
	}

	// fetch the link info from the url passed in the task payload
	linkInfo, err := services.ExtractLinkInfo(data.Url)
	if err != nil {
		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error extracting link info: %v", err)
		return err
	}

	followService := services.NewFollowTask(s.DB, s.Red)
	// NOTE: for tidal, the update hash is a timestamp (in string format) for tidal
	updatedHash, ok, _, err := followService.HasPlaylistBeenUpdated(linkInfo.Platform, linkInfo.Entity, linkInfo.EntityID, data.App)

	if err != nil {
		// if the user wants follow a playlist we havent cached before, we're not (necessarily) going to
		// be able to check if the playlist has been updated. in this case, we want to create a new follow
		if err == redis.Nil {
			log.Printf("[queue][ProcessFollowTaskHandler] - playlist hasnt been cached")
			// todo: watch out for this
			convertedPlaylist, err := universal.ConvertPlaylist(linkInfo, s.Red, s.DB)
			if err != nil {
				log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error converting playlist: %v", err)
				return err
			}

			// then save the snapshot to redis
			snapshotID := fmt.Sprintf("%s:snapshot:%s", linkInfo.Platform, linkInfo.EntityID)
			err = s.Red.Set(context.Background(), snapshotID, updatedHash, 0).Err()
			if err != nil {
				log.Printf("[queue][ProcessFollowTaskHandler][redis] - error saving snapshot to redis: %v", err)
				return err
			}
			log.Printf("[queue][ProcessFollowTaskHandler] - playlist has been cached and converted: %v", convertedPlaylist)
			return nil
		}

		_, err = s.DB.Exec(queries.UpdateFollowLatUpdated, linkInfo.EntityID)
		if err != nil {
			log.Printf("[queue][ProcessFollowTaskHandler] - error updating follow last updated: %v", err)
			return err
		}

		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error checking if playlist has been updated: %v", err)
		return err
	}

	// if the playlist has been updated, then update the redis snapshot with the new hash
	if ok {
		log.Println("[queue][ProcessFollowTaskHandler] - playlist has been updated. Converting again to fetch new tracks")
		// todo: watch out for this
		updatedPlaylist, err := universal.ConvertPlaylist(linkInfo, s.Red, s.DB)
		if err != nil {
			log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error converting playlist: %v", err)
			return err
		}

		database := db.NewDB{DB: s.DB}
		// notify subscribers about the new update.
		follow, err := database.FetchFollowByEntityID(linkInfo.EntityID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("[queue][ProcessFollowTaskHandler] - no follow found for this entity")
				return nil
			}
			log.Printf("[queue][ProcessFollowTaskHandler] - error fetching follow: %v", err)
			return err
		}

		log.Printf("[queue][ProcessFollowTaskHandler] - follow: %v\n", follow.Subscribers)

		var subs []map[string]interface{}
		updatedPlaylistByte, _ := json.Marshal(updatedPlaylist)

		for _, subscriber := range follow.Subscribers.([]blueprint.User) {
			uniqueID, _ := uuid.NewUUID()

			log.Printf("[queue][ProcessFollowTaskHandler] - sending notification to subscriber: %v", subscriber.ID)

			var subscriberData = map[string]interface{}{
				"subscriber":      subscriber.UUID.String(),
				"notification_id": uniqueID.String(),
				"data":            string(updatedPlaylistByte),
			}

			subs = append(subs, subscriberData)
		}
		// do a bulk insert for all subscriber notification
		_, err = s.DB.NamedExec(queries.CreateFollowNotification, subs)

		if err != nil {
			log.Printf("[queue][ProcessFollowTaskHandler] - error creating follow notification: %v", err)
			return err
		}

		_, err = s.DB.Exec(queries.UpdateFollowLatUpdated, linkInfo.EntityID)
		if err != nil {
			log.Printf("[queue][ProcessFollowTaskHandler] - error updating follow last updated: %v", err)
			return err
		}

		snapshotID := fmt.Sprintf("%s:snapshot:%s", linkInfo.Platform, updatedHash)
		err = s.Red.Set(context.Background(), snapshotID, updatedHash, 0).Err()

		if err != nil {
			log.Printf("[queue][ProcessFollowTaskHandler] - error updating snapshot: %v", err)
			return err
		}
		log.Printf("[queue][ProcessFollowTaskHandler] - playlist has been cached %s", updatedPlaylist.Meta.URL)

		log.Printf("[queue][ProcessFollowTaskHandler] - Playlist has been updated and subscribers notified")
		return nil
	}

	_, err = s.DB.Exec(queries.UpdateFollowLatUpdated, linkInfo.EntityID)
	if err != nil {
		log.Printf("[queue][ProcessFollowTaskHandler] - error updating follow last updated: %v", err)
		return err
	}

	log.Printf("[queue][ProcessFollowTaskHandler] - playlist has not been updated")
	return nil
}

// SyncFollowsHandler fetches follow tasks that can be processed and processes them. This is called from a cron job. (in main)
func SyncFollowsHandler(DB *sqlx.DB, red *redis.Client, asynqClient *asynq.Client, asynqMux *asynq.ServeMux) {
	database := db.NewDB{DB: DB}
	follows, err := database.FetchFollowsToProcess()
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[follow][SyncFollowsHandler] - no follow tasks to process")
			return
		}
		log.Printf("[follow][SyncFollowsHandler] - error fetching follows to process: %v", err)
		return
	}

	// enqueue each of these tasks. these would be unique using the entity_id. this is to make sure that we do not have multiple
	// type of same task
	for _, follow := range *follows {
		log.Printf("[follow][SyncFollowsHandler] - Entity URL with link to be extracted: %v", follow.EntityID)
		extractLinkInfo, err := services.ExtractLinkInfo(follow.EntityURL)
		if err != nil {
			log.Printf("[follow][SyncFollowsHandler] - error extracting link info: %v", err)
			err := database.UpdateFollowStatus(follow.UID.String(), "failed")
			if err != nil {
				log.Printf("[follow][SyncFollowsHandler] - error updating follow status: %v", err)
			}
			continue
		}
		var followTaskData = &blueprint.FollowTaskData{
			User:      follow.Developer,
			Url:       follow.EntityURL,
			EntityID:  follow.EntityID,
			Platform:  extractLinkInfo.Platform,
			App:       follow.App.String(),
			Developer: follow.Developer.String(),
		}

		// serialize followTaskData to bytes
		followTaskDataBytes, err := json.Marshal(followTaskData)
		if err != nil {
			log.Printf("[follow][SyncFollowsHandler] - error marshalling follow task data: %v", err)
			return
		}

		taskTypeID, _ := uuid.NewUUID()

		// make the task "unique" for the next 1hr. this is to make sure that whenever the cronjob runs
		// we dont have multiple tasks of the same type
		followTask := asynq.NewTask(taskTypeID.String(), followTaskDataBytes, asynq.Retention(time.Hour))
		// enqueue the task
		_, err = asynqClient.Enqueue(followTask)
		if err != nil {
			log.Printf("[follow][SyncFollowsHandler] - error enqueuing follow task: %v", err)
			return
		}
		sync := NewTaskCronHandler(DB, red)
		asynqMux.HandleFunc(taskTypeID.String(), sync.ProcessFollowTaskHandler)
	}
	log.Printf("[follow][SyncFollowsHandler] - fetched %d follow tasks to process", len(*follows))
	return
}

//var ConfigDefault = fiber.Config{
//	ErrorHandler: func(ctx *fiber.Ctx, err error) error {
//		return nil
//	},
//}
//
//func NewRecoveyHandler(c *fiber.Ctx) error {
//}
