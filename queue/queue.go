package queue

import (
	"context"
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
	DB          *sqlx.DB
	Red         *redis.Client
}

func NewOrchdioQueue(asynqClient *asynq.Client, db *sqlx.DB, red *redis.Client) *OrchdioQueue {
	return &OrchdioQueue{
		AsynqClient: asynqClient,
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

	log.Printf("[queue][PlaylistTaskHandler] - task context info: %v", ctx)
	//handlerChan := make(chan int)
	// deserialize the task payload and get the PlaylistTaskData struct
	var data blueprint.PlaylistTaskData
	err := json.Unmarshal(task.Payload(), &data)
	if err != nil {
		log.Printf("[queue][PlaylistConversionHandler][conversion] - error unmarshalling task payload: %v", err)
		return err
	}
	cErr := o.PlaylistHandler(task.ResultWriter().TaskID(), data.ShortURL, data.LinkInfo, data.User.UUID.String())
	if cErr != nil {
		log.Printf("[queue][PlaylistConversionHandler][conversion] - error processing task in queue handler: %v", cErr)
		if err == blueprint.EPHANTOMERR {
			log.Printf("[queue][PlaylistConversionHandler][conversion] - phantom error, skipping but marking as done")
			return nil
		}
		return cErr
	}

	return nil
}

// PlaylistHandler converts a playlist immediately.
func (o *OrchdioQueue) PlaylistHandler(uid, shorturl string, info *blueprint.LinkInfo, developer string) error {
	log.Printf("[queue][PlaylistHandler] - processing task: %v", uid)
	database := db.NewDB{DB: o.DB}
	log.Printf("[queue][PlaylistHandler] - processing playlist: %v %v %v\n", database, info, developer)

	// fetch user from database
	user, err := database.FindUserByUUID(developer)
	if err != nil {
		log.Printf("[queue][PlaylistHandler] - could not find user: %v", err)
		return err
	}

	//_taskId, dbErr := database.CreateOrUpdateTask(uid, shorturl, user.UUID.String(), info.EntityID)
	//taskId := string(_taskId)
	// get task from db
	task, dbErr := database.FetchTask(uid)
	if dbErr != nil {
		log.Printf("[queue][PlaylistHandler] - could not find task: %v", dbErr)
		return dbErr
	}
	taskId := task.UID.String()

	if dbErr != nil {
		log.Printf("[queue][EnqueueTask] - error creating or updating task: %v", dbErr)
		return dbErr
	}

	log.Printf("[queue][PlaylistHandler] - created or updated task: %v", taskId)

	h, err := universal.ConvertPlaylist(info, o.Red)
	var status string
	// for now, we dont want to bother about retrying and all of that. we're simply going to mark a task as failed if it fails
	// the reason is that it's hard handling the retry for it to worth it. In the future, we might add a proper retry system
	// but for now, if a playlist conversion fails, it fails. In the frontend, the user will most likely retry anyway and that means
	// calling the endpoint again, which will create a new task.
	if err != nil {
		log.Printf("[queue][EnqueueTask] - error converting playlist: %v", err)
		status = "failed"
		// this is for when for example, apple music returns Not Found for a playlist thats visible but not public.
		if err == blueprint.ENORESULT {
			// create a new
			payload := blueprint.TaskErrorPayload{
				Platform: "applemusic",
				Status:   "failed",
				Error:    "Not Found",
				Message:  "It could be that the playlist is visible but has not been added to public and search by Author. See https://support.apple.com/en-gb/HT207948",
			}

			// serialize the payload
			ser, err := json.Marshal(&payload)
			if err != nil {
				log.Printf("[queue][EnqueueTask] - error marshalling task 'result not found' payload: %v", err)
			}
			taskErr := database.UpdateTaskStatus(taskId, status)
			if taskErr != nil {
				log.Printf("[queue][EnqueueTask] - could not update task status in DB when updating not found conversion: %v", taskErr)
				return taskErr
			}

			// update task result to payload
			_, updateErr := database.UpdateTaskResult(taskId, string(ser))
			if updateErr != nil {
				log.Printf("[queue][EnqueueTask] - could not update task result in DB when updating not found conversion: %v", updateErr)
				return updateErr
			}

			log.Printf("[queue][EnqueueTask] could not fetch the playlist. skipping but marking as done")
			return nil
		}

		// update the task status to failed
		taskErr := database.UpdateTaskStatus(taskId, status)
		if taskErr != nil {
			log.Printf("[queue][EnqueueTask] - error updating task status: %v", taskErr)
			return taskErr
		}

		// create new task error payload
		payload := blueprint.TaskErrorPayload{
			Platform: info.Platform,
			Status:   "failed",
			Error:    err.Error(),
			Message:  "An error occurred while converting the playlist",
		}

		// serialize the payload
		ser, err := json.Marshal(&payload)
		if err != nil {
			log.Printf("[queue][EnqueueTask] - error marshalling task error payload: %v", err)
			return err
		}

		// update task result to payload
		_, updateErr := database.UpdateTaskResult(taskId, string(ser))
		if updateErr != nil {
			log.Printf("[queue][EnqueueTask] - failed to process playlist task and could not update 'task error payload' in database %v", updateErr)
			return updateErr
		}
		log.Printf("[queue][EnqueueTask] - error converting playlist: For some reason, we couldnt convert this playlist but we will mark as done.%v", err)

		return nil
	}

	if h != nil {
		status = "completed"
	}

	if status == "" {
		log.Printf("[queue][PlaylistHandler] - updating task status to failed... skipping")
		return nil
	}

	h.Meta.ShortURL = shorturl
	h.Meta.Entity = "playlist"

	log.Printf("[queue][PlaylistHandler] -shortlink gen is %v", status)
	// serialize h
	ser, mErr := json.Marshal(h)
	if mErr != nil {
		log.Printf("[queue][EnqueueTask] - error marshalling playlist conversion: %v", mErr)
		return mErr
	}
	log.Printf("[queue][PlaylistHandler] - serialized data")
	result, rErr := database.UpdateTaskResult(taskId, string(ser))

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
		// TODO: change this to return evErr
		return nil
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
		log.Printf("[Queue][LoggerMiddleware] Started processing task %q", t.ResultWriter().TaskID())
		err := h.ProcessTask(ctx, t)
		if err != nil {
			// this block checks for tasks that are orphaned —they died mid-processing. from here, next time these errors are encountered, they will throw
			// a ENORESULT error which is an Orchdio error that specifies that no result could be found for the action. This error will then
			// be attached to this orphaned task and marked to be retried.
			// then next time that the task is processed, the error handler method on the queue would be called (main.go, asyncServer declaration)
			// and the task would then be directly processed and ran normally, updating in record etc. If there's a success, then the task is marked as
			// done in the db and the queue but if error, then it'll retry and retry cycle happens. This feels like a workaround but at the same time strongly feels
			// like the best way to handle this.
			handlerNotFoundErr := asynq.NotFound(ctx, t)
			if handlerNotFoundErr != nil {
				log.Printf("[queue][LoggingMiddleware] - Error is a handler not found error %v", handlerNotFoundErr)
				return blueprint.ENORESULT
			}
			log.Printf("[queue][LoggingMiddleware] - error processing task: %v", err)
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
