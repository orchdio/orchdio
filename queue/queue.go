package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/vicanso/go-axios"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/universal"
	"orchdio/util"
)

type OrchQueue struct {
	Redis *redis.Client
}
type OrchdioQueue struct {
	AsynqClient *asynq.Client
	AsynqServer *asynq.Server
	DB          *sqlx.DB
	Red         *redis.Client
}

func NewOrchdioQueue(asynqClient *asynq.Client, asynqServer *asynq.Server, db *sqlx.DB, red *redis.Client) *OrchdioQueue {
	return &OrchdioQueue{
		AsynqClient: asynqClient,
		AsynqServer: asynqServer,
		DB:          db,
		Red:         red,
	}
}

// NewPlaylistQueue creates a new playlist queue.
func (o *OrchdioQueue) NewPlaylistQueue(entityID string, payload *blueprint.LinkInfo) (*asynq.Task, error) {
	ser, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[queue][NewPlaylistQueue][NewPlaylistQueue] - error marshalling playlist conversion: %v", err)
		return nil, err
	}

	log.Printf("[queue][NewPlaylistQueue][NewPlaylistQueue] - queuing playlist: %v\n", entityID)
	var task = asynq.NewTask(entityID, ser)
	log.Printf("[queue][NewPlaylistQueue][NewPlaylistQueue] - queued playlist: %v\n", entityID)
	return task, nil
}

//func (o *OrchdioQueue) ProcessFollowTaskHandler(ctx context.Context, task *asynq.Task) error {
//	log.Printf("[queue][ProcessFollowTaskHandler] - processing follow task")
//	var data blueprint.FollowTaskData
//	err := json.Unmarshal(task.Payload(), &data)
//	if err != nil {
//		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error unmarshalling task payload: %v", err)
//		return err
//	}
//
//	// fetch the link info from the url passed in the task payload
//	linkInfo, err := services.ExtractLinkInfo(data.Url)
//	if err != nil {
//		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error extracting link info: %v", err)
//		return err
//	}
//
//	followController := follow.NewFollow(o.DB, o.Red)
//
//	ok, err := followController.HasPlaylistBeenUpdated(linkInfo.Platform, linkInfo.Entity, linkInfo.EntityID)
//	if err != nil {
//		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error checking if playlist has been updated: %v", err)
//		return err
//	}
//
//	log.Printf("[queue][ProcessFollowTaskHandler][conversion] - playlist has been updated: %v", ok)
//	return nil
//	//  check the cache to see if we've cached the follow url in redis.
//	// key format: "<platform>:snapshot:"+id
//	//key := fmt.Sprintf("%s:snapshot:%s", linkInfo.Platform, linkInfo.EntityID)
//	//cachedSnapshotID, snapErr := o.Red.Get(context.Background(), key).Result()
//	//if snapErr != nil {
//	//	if snapErr == redis.Nil {
//	//		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - no cached snapshot for %s", key)
//	//	} else {
//	//		log.Printf("[queue][ProcessFollowTaskHandler][conversion] - error getting snapshot id from redis: %v", snapErr)
//	//	}
//	//	return snapErr
//	//}
//
//	// then get the latest snapshot from the database
//}

func (o *OrchdioQueue) PlaylistTaskHandler(ctx context.Context, task *asynq.Task) error {
	log.Printf("[queue][PlaylistTaskHandler] - processing task")
	// deserialize the task payload and get the PlaylistTaskData struct
	var data blueprint.PlaylistTaskData
	err := json.Unmarshal(task.Payload(), &data)
	if err != nil {
		log.Printf("[queue][PlaylistConversionHandler][conversion] - error unmarshalling task payload: %v", err)
		return err
	}
	cErr := o.PlaylistHandler(task.Type(), data.LinkInfo, data.User)
	if cErr != nil {
		log.Printf("[queue][PlaylistConversionHandler][conversion] - error processing task: %v", err)
		return cErr
	}
	return nil
}

// PlaylistHandler converts a playlist immediately.
func (o *OrchdioQueue) PlaylistHandler(uid string, info *blueprint.LinkInfo, user *blueprint.User) error {
	log.Printf("[queue][PlaylistHandler] - processing task: %v", uid)
	database := db.NewDB{DB: o.DB}
	log.Printf("[queue][PlaylistHandler] - processing playlist: %v %v %v\n", database, info, user)
	_taskId, dbErr := database.CreateOrUpdateTask(uid, user.UUID.String(), info.EntityID)
	taskId := string(_taskId)

	if dbErr != nil {
		log.Printf("[queue][EnqueueTask] - error creating or updating task: %v", dbErr)
		return dbErr
	}
	log.Printf("[queue][PlaylistHandler] - created or updated task: %v", taskId)

	h, err := universal.ConvertPlaylist(info, o.Red)
	if err != nil {
		log.Printf("[queue][EnqueueTask] - error converting playlist: %v", err)

		// update the task status to failed
		taskErr := database.UpdateTaskStatus(taskId, "failed")
		if taskErr != nil {
			log.Printf("[queue][EnqueueTask] - error updating task status: %v", taskErr)
			return taskErr
		}
		return err
	}
	// serialize h
	ser, mErr := json.Marshal(h)
	if mErr != nil {
		log.Printf("[queue][EnqueueTask] - error marshalling playlist conversion: %v", mErr)
		return mErr
	}
	result, rErr := database.UpdateTask(taskId, string(ser))

	if rErr != nil {
		log.Printf("[queue][EnqueueTask] - error updating task status: %v", rErr)
		return rErr
	}

	// update the task status to completed
	taskErr := database.UpdateTaskStatus(taskId, "completed")
	if taskErr != nil {
		log.Printf("[queue][EnqueueTask] - error updating task status: %v", taskErr)
		return taskErr
	}

	// post to the developer webhook
	webhook, wErr := database.FetchWebhook(user.UUID.String())
	// TODO: implement making sure developer has a webhook set
	if wErr != nil && wErr != sql.ErrNoRows {
		log.Printf("[queue][PlaylistHandler] - error fetching developer webhook: %v", wErr)
		return wErr
	}

	r := blueprint.WebhookMessage{
		Message: "playlist conversion done",
		Event:   blueprint.EEPLAYLISTCONVERSION,
		Payload: &result,
	}

	// get user api key
	apiKey, aErr := database.FetchUserApikey(user.UUID)
	if aErr != nil {
		log.Printf("[queue][PlaylistHandler] - error fetching user api key: %v", aErr)
		return aErr
	}
	// generate the hmac for the webhook
	hmac := util.GenerateHMAC(r, apiKey.Key.String())
	ax := axios.NewInstance(&axios.InstanceConfig{
		Headers: map[string][]string{
			"x-orchdio-hmac": {string(hmac)},
		},
	})

	re, evErr := ax.Post(webhook.Url, r)
	if re.Status != http.StatusOK {
		log.Printf("[queue][PlaylistHandler] - error posting webhook: %v", re)
		return blueprint.EPHANTOMERR
	}

	if evErr != nil {
		log.Printf("[queue][PlaylistHandler] - error posting webhook to endpoint %s=%v", string(webhook.Url), evErr)
		return evErr
	}

	log.Printf("[queue][EnqueueTask] - successfully processed task: %v", taskId)

	// NOTE: In the case of a "follow", instead of just exiting here, we reschedule the task to  like 2 mins later.
	return nil
}

//func (o *OrchdioQueue)
