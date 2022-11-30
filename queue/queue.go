package queue

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/teris-io/shortid"
	"github.com/vicanso/go-axios"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/universal"
	"orchdio/util"
	"strings"
	"time"
)

const (
	PlaylistConversionQueue = "playlist-conversion"
	PlaylistConversionTask  = "playlist:conversion"
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

// PlaylistTaskHandler is the handler method for processing playlist conversion tasks.
func (o *OrchdioQueue) PlaylistTaskHandler(ctx context.Context, task *asynq.Task) error {
	log.Printf("[queue][PlaylistTaskHandler] - processing task")
	// deserialize the task payload and get the PlaylistTaskData struct
	var data blueprint.PlaylistTaskData
	err := json.Unmarshal(task.Payload(), &data)
	if err != nil {
		log.Printf("[queue][PlaylistConversionHandler][conversion] - error unmarshalling task payload: %v", err)
		return err
	}
	cErr := o.PlaylistHandler(task.ResultWriter().TaskID(), data.LinkInfo, data.User.UUID.String())
	if cErr != nil {
		log.Printf("[queue][PlaylistConversionHandler][conversion] - error processing task: %v", err)
		return cErr
	}
	return nil
}

// PlaylistHandler converts a playlist immediately.
func (o *OrchdioQueue) PlaylistHandler(uid string, info *blueprint.LinkInfo, developer string) error {
	log.Printf("[queue][PlaylistHandler] - processing task: %v", uid)
	database := db.NewDB{DB: o.DB}
	log.Printf("[queue][PlaylistHandler] - processing playlist: %v %v %v\n", database, info, developer)
	const format = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"
	sid, err := shortid.New(1, format, 2342)

	if err != nil {
		log.Printf("\n[controllers][platforms][ConvertTrack] - could not generate short id %v\n", err)
		return err
	}

	shorturl, _ := sid.Generate()
	// fetch user from database
	user, err := database.FindUserByUUID(developer)
	if err != nil {
		log.Printf("[queue][PlaylistHandler] - could not find user: %v", err)
		return err
	}

	_taskId, dbErr := database.CreateOrUpdateTask(uid, shorturl, user.UUID.String(), info.EntityID)
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

	h.ShortURL = shorturl
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
	if wErr != nil {
		if wErr.Error() != "sql: no rows in result set" {
			log.Printf("[queue][EnqueueTask] - error fetching webhook: %v", wErr)
			return wErr
		}
	}

	r := blueprint.WebhookMessage{
		Message: "playlist conversion done",
		Event:   blueprint.EEPLAYLISTCONVERSION,
		Payload: &result,
	}

	// get user api key
	apiKey, aErr := database.FetchUserApikey(user.Email)
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

	if webhook == nil {
		log.Printf("[queue][PlaylistHandler] - no webhook found. Skipping. Task done.")
		return nil
	}

	re, evErr := ax.Post(webhook.Url, r)

	// TODO: implement proper retry logic for queue. After retrying for some time, it should stop (marked as failed)
	//   current assumption is that asynq handles this but to-do is to verify this.
	if evErr != nil {
		log.Printf("[queue][PlaylistHandler] - error posting to webhook: %v", evErr.Error())
		// TODO: implement retry logic. For now, if the webhook is unavailable, it will NOT be retried since we're returning nil
		if strings.Contains(evErr.Error(), "no such host") {
			log.Printf("[queue][PlaylistHandler] - webhook unavailable, skipping")
			return nil
		}
		log.Printf("[queue][PlaylistHandler] - error posting webhook to endpoint %s=%v", webhook.Url, evErr)
		return evErr
	}

	if re.Status != http.StatusOK {
		log.Printf("[queue][PlaylistHandler] - error posting webhook: %v", re)
		return blueprint.EPHANTOMERR
	}

	log.Printf("[queue][EnqueueTask] - successfully processed task: %v", taskId)

	// NOTE: In the case of a "follow", instead of just exiting here, we reschedule the task to  like 2 mins later.
	return nil
}

func LoggingMiddleware(h asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		start := time.Now()
		log.Printf("Start processing %q", t.ResultWriter().TaskID())
		err := h.ProcessTask(ctx, t)
		if err != nil {
			return err
		}
		log.Printf("Finished processing %q: Elapsed Time = %v", t.Type(), time.Since(start))
		return nil
	})
}

//func (o *OrchdioQueue) RetryOrphanedFailedTasks(inspector asynq.Inspector, queue, developer string) error {
//	// get all failed tasks from asynq
//	failedTasks, err := inspector.ListRetryTasks(queue, asynq.Page(1), asynq.PageSize(1000))
//	if err != nil {
//		return err
//	}
//	for _, t := range failedTasks {
//		task := t
//		if task.Queue == queue {
//			payload := &blueprint.LinkInfo{}
//			err := json.Unmarshal(task.Payload, payload)
//			if err != nil {
//				log.Printf("[queue][RetryOrphanedFailedTasks] - error unmarshalling task payload: %v", err)
//				continue
//			}
//			// call the handler directly
//			// create taskinfo
//			info := &blueprint.LinkInfo{
//				Platform:   developer,
//				TargetLink: "",
//				Entity:     "",
//				EntityID:   "",
//			}
//			err = o.PlaylistHandler(context.Background(), task, developer)
//		}
//	}
//}
