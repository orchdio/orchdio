package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/universal"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	sendinblue "github.com/sendinblue/APIv3-go-library/v2/lib"
)

type QueueService interface {
	EnqueueTask(task *asynq.Task, q, taskId string, processIn time.Duration)
}

type OrchdioQueue struct {
	AsynqClient *asynq.Client
	AsynqRouter *asynq.ServeMux
	DB          *sqlx.DB
	Red         *redis.Client
}

func NewOrchdioQueue(asynqClient *asynq.Client, db *sqlx.DB, red *redis.Client, router *asynq.ServeMux) *OrchdioQueue {
	return &OrchdioQueue{
		AsynqClient: asynqClient,
		DB:          db,
		Red:         red,
		AsynqRouter: router,
	}
}

// NewPlaylistQueue creates a new playlist queue.
func (o *OrchdioQueue) NewPlaylistQueue(entityID string, payload *blueprint.LinkInfo) (*asynq.Task, error) {
	ser, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[queue][NewPlaylistQueue][NewPlaylistQueue] - error marshalling playlist conversion: %v", err)
		return nil, err
	}

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
	data.LinkInfo.TaskID = task.ResultWriter().TaskID()
	cErr := o.PlaylistHandler(task.ResultWriter().TaskID(), data.ShortURL, data.LinkInfo, data.App.UID.String())
	if cErr != nil {
		log.Printf("[queue][PlaylistConversionHandler][conversion] - error processing task in queue handler: %v", cErr)
		if errors.Is(err, blueprint.ErrPhantomErr) {
			log.Printf("[queue][PlaylistConversionHandler][conversion] - phantom error, skipping but marking as done")
			return nil
		}
		return cErr
	}
	return nil
}

// EnqueueTask enqueues the task passed in.
func (o *OrchdioQueue) EnqueueTask(task *asynq.Task, queue, taskId string, processIn time.Duration) error {
	log.Printf("[queue][EnqueueTask] - enqueuing task: %v", taskId)
	_, err := o.AsynqClient.Enqueue(task, asynq.Queue(queue), asynq.TaskID(taskId), asynq.Unique(time.Second*60),
		asynq.ProcessIn(processIn))
	if err != nil {
		log.Printf("[queue][EnqueueTask] - error enqueuing task: %v", err)
		return err
	}
	log.Printf("[queue][EnqueueTask] - enqueued task: %v", taskId)
	return nil
}

// RunTask runs the task passed in by sending it to the router server handle func.
func (o *OrchdioQueue) RunTask(pattern string, handler func(ctx context.Context, task *asynq.Task) error) {
	log.Printf("[queue][RunTask] - attaching handler to task")
	// create a new server
	o.AsynqRouter.HandleFunc(pattern, handler)
	log.Printf("[queue][RunTask] - attached handler to task")
}

// NewTask creates a new task and returns it.
func (o *OrchdioQueue) NewTask(taskType, queue string, retry int, payload []byte) (*asynq.Task, error) {
	return asynq.NewTask(taskType, payload, asynq.Queue(queue), asynq.Retention(time.Hour*24), asynq.MaxRetry(retry)), nil
}

// SendEmail sends the email using sendinblue.
func (o *OrchdioQueue) SendEmail(emailData *blueprint.EmailTaskData) error {
	// create new sendinblue config
	config := sendinblue.NewConfiguration()
	config.AddDefaultHeader("api-key", os.Getenv("SENDINBLUE_API_KEY"))
	client := sendinblue.NewAPIClient(config)

	subject := "App Access"
	if emailData.Subject != "" {
		subject = emailData.Subject
	}

	_, resp, err := client.TransactionalEmailsApi.SendTransacEmail(context.Background(), sendinblue.SendSmtpEmail{
		TemplateId: int64(emailData.TemplateID),
		MessageVersions: []sendinblue.SendSmtpEmailMessageVersions{
			{
				To: []sendinblue.SendSmtpEmailTo1{
					{
						Email: emailData.To,
					},
				},
				Params:  emailData.Payload,
				Subject: subject,
			},
		},
	})

	if err != nil {
		log.Printf("[queue][SendEmailHandler][send-email] error sending email: %v", err)
		return err
	}

	if resp.StatusCode >= 400 {
		log.Printf("[queue][SendEmailHandler][send-email] error sending email: %v", resp.StatusCode)
		return err
	}
	return nil
}

// SendEmailHandler is the handler for sending emails in queues.
func (o *OrchdioQueue) SendEmailHandler(_ context.Context, task *asynq.Task) error {
	log.Printf("[queue][SendEmailHandler][send-email] sending email in queue")
	var emailData blueprint.EmailTaskData
	err := json.Unmarshal(task.Payload(), &emailData)
	if err != nil {
		log.Printf("[queue][SendEmailHandler][send-email] error unmarshalling task payload: %v", err)
		return err
	}

	err = o.SendEmail(&emailData)
	if err != nil {
		log.Printf("[queue][SendEmailHandler][send-email] error sending email: %v", err)
		return err
	}
	log.Printf("[queue][SendEmailHandler][send-email] email sent to %v", emailData.To)
	return nil
}

// PlaylistHandler converts a playlist immediately.
func (o *OrchdioQueue) PlaylistHandler(uid, shorturl string, info *blueprint.LinkInfo, appId string) error {
	log.Printf("[queue][PlaylistHandler] - processing task: %v", uid)
	database := db.NewDB{DB: o.DB}
	// fetch app from db
	_, err := database.FetchAppByAppIdWithoutDevId(appId)
	if err != nil {
		log.Printf("[queue][PlaylistHandler] - could not find user: %v", err)
		return err
	}

	// get task from db
	task, dbErr := database.FetchTask(uid)
	if dbErr != nil {
		log.Printf("[queue][PlaylistHandler] - could not find task: %v", dbErr)
		return dbErr
	}
	taskId := task.UID.String()

	// then here, we add the shortID to the info payload. this is to enable us do things like send the short_id (unique_id)
	// to a client (via webhook) after a playlist has been converted.
	info.UniqueID = task.UniqueID

	playlist, cErr := universal.ConvertPlaylist(info, o.Red, o.DB)
	var status string
	// for now, we don't want to bother about retrying and all of that. we're simply going to mark a task as failed if it fails
	// the reason is that it's hard handling the retry for it to worth it. In the future, we might add a proper retry system
	// but for now, if a playlist conversion fails, it fails. In the frontend, the user will most likely retry anyway and that means
	// calling the endpoint again, which will create a new task.
	if cErr != nil {
		log.Printf("[queue][EnqueueTask] - error converting playlist: %v", cErr)
		status = blueprint.TaskStatusFailed
		// this is for when for example, apple music returns Not Found for a playlist thats visible but not public. (needs citation)
		if errors.Is(cErr, blueprint.EnoResult) {
			// create a new payload
			payload := blueprint.TaskErrorPayload{
				Platform: info.Platform,
				Status:   "failed",
				Error:    "Not Found",
				Message:  "It could be that the playlist is visible but has not been added to public and search by Author. See https://support.apple.com/en-gb/HT207948",
			}

			// serialize the payload
			serializedPayload, jErr := json.Marshal(&payload)
			if jErr != nil {
				log.Printf("[queue][EnqueueTask] - error marshalling task 'result not found' payload: %v", jErr)
			}
			taskErr := database.UpdateTaskStatus(taskId, status)
			if taskErr != nil {
				log.Printf("[queue][EnqueueTask] - could not update task status in DB when updating not found conversion: %v", taskErr)
				return taskErr
			}

			// update task result to payload
			_, updateErr := database.UpdateTaskResult(taskId, string(serializedPayload))
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
			Status:   blueprint.TaskStatusFailed,
			Error:    cErr.Error(),
			Message:  "An error occurred while converting the playlist",
		}

		// serialize the payload
		ser, jErr := json.Marshal(&payload)
		if jErr != nil {
			log.Printf("[queue][EnqueueTask] - error marshalling task error payload: %v", jErr)
			return jErr
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

	if playlist == nil {
		log.Printf("Playlist is nil, what to do???")
		return nil
	}

	status = blueprint.TaskStatusCompleted
	playlist.Meta.ShortURL = shorturl
	// fixme: magic string
	playlist.Meta.Entity = "playlist"

	// serialize playlist
	ser, mErr := json.Marshal(playlist)
	if mErr != nil {
		log.Printf("[queue][EnqueueTask] - error marshalling playlist conversion: %v", mErr)
		return mErr
	}
	log.Printf("[queue][PlaylistHandler] - serialized conversion data")
	_, rErr := database.UpdateTaskResult(taskId, string(ser))
	if rErr != nil {
		log.Printf("[queue][EnqueueTask] - error updating task status: %v", rErr)
		return rErr
	}

	// update the task status to completed
	taskErr := database.UpdateTaskStatus(taskId, blueprint.TaskStatusCompleted)
	if taskErr != nil {
		log.Printf("[queue][EnqueueTask] - error updating task status: %v", taskErr)
		return taskErr
	}

	log.Printf("[queue][EnqueueTask] - successfully processed task: %v", taskId)
	// NOTE: In the case of a "follow", instead of just exiting here, we reschedule the task to  like 2 mins later.
	return nil
}

// CheckForOrphanedTasksMiddleware is a middleware that checks for orphaned tasks. Orphaned tasks are tasks that perhaps failed at a point
// and a handler was not able to be attached to them. This middleware checks for these tasks and process them. if the task is not orphaned,
// it just passes it through to the next middleware
func CheckForOrphanedTasksMiddleware(h asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		start := time.Now()
		log.Printf("[Queue][LoggerMiddleware] Started processing task %q", t.ResultWriter().TaskID())
		err := h.ProcessTask(ctx, t)
		if err != nil {
			// this block checks for tasks that are orphaned —they died mid-processing. from here, next time these errors are encountered, they will throw
			// a EnoResult error which is an Orchdio error that specifies that no result could be found for the action. This error will then
			// be attached to this orphaned task and marked to be retried.
			// then next time that the task is processed, the error handler method on the queue would be called (main.go, asyncServer declaration)
			// and the task would then be directly processed and ran normally, updating in record etc. If there's a success, then the task is marked as
			// done in the db and the queue but if error, then it'll retry and retry cycle happens. This feels like a workaround but at the same time strongly feels
			// like the best way to handle this.
			handlerNotFoundErr := asynq.NotFound(ctx, t)
			if handlerNotFoundErr != nil {
				log.Printf("[queue][CheckForOrphanedTasksMiddleware][warning] - Error is a handler not found error")
				return blueprint.EnoResult
			}
			log.Printf("[queue][CheckForOrphanedTasksMiddleware] - error processing task: %v", err)
			return err
		}
		log.Printf("Finished processing %q: Elapsed Time = %v", t.Type(), time.Since(start))
		return nil
	})
}
