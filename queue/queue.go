package queue

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/universal"
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
		return err
	}
	return nil
}

// PlaylistHandler processes a task in the queue (immediately).
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
	_, rErr := database.UpdateTask(taskId, string(ser))

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
	log.Printf("[queue][EnqueueTask] - successfully processed task: %v", taskId)
	return nil
}

func (o *OrchdioQueue) ConversionHandler(ctx context.Context, t *asynq.Task) error {
	// get the data passed first by serializing the task payload
	var info blueprint.LinkInfo
	err := json.Unmarshal(t.Payload(), &info)
	if err != nil {
		log.Printf("[queue][ProcessConversionTask][conversion] - error unmarshalling task payload: %v", err)
		return err
	}

	return nil
}
