package conversion

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/teris-io/shortid"
	"github.com/vmihailenco/taskq/v3"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/queue"
	"orchdio/util"
	"os"
	"strings"
	"time"
)

// Controller is the controller for the conversion service.
type Controller struct {
	DB          *sqlx.DB
	Red         *redis.Client
	Queue       taskq.Queue
	Factory     taskq.Factory
	Asynq       *asynq.Client
	AsynqServer *asynq.Server
	AsynqMux    *asynq.ServeMux
}

// NewConversionController creates a new conversion controller.
func NewConversionController(db *sqlx.DB, red *redis.Client, queue taskq.Queue, factory taskq.Factory, asynqClient *asynq.Client, asynqserver *asynq.Server, mux *asynq.ServeMux) *Controller {

	res := &Controller{
		DB:          db,
		Red:         red,
		Queue:       queue,
		Factory:     factory,
		Asynq:       asynqClient,
		AsynqServer: asynqserver,
		AsynqMux:    mux,
	}

	// create a new instance of the queue factory
	return res
}

// ConvertPlaylist creates a new playlist conversion task and returns an id to the task.
func (c *Controller) ConvertPlaylist(ctx *fiber.Ctx) error {
	log.Printf("[controller][conversion][ConvertPlaylist] - echo conversion")

	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)
	uniqueId := uuid.New().String()
	const format = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"
	sid, err := shortid.New(1, format, 2342)
	if err != nil {
		log.Printf("\n[controllers][platforms][ConvertEntity] - could not generate short id %v\n", err)
		return err
	}

	shorturl, _ := sid.Generate()

	taskData := &blueprint.PlaylistTaskData{
		LinkInfo: linkInfo,
		App:      app,
		TaskID:   uniqueId,
		ShortURL: shorturl,
	}

	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("[controller][conversion][ConvertPlaylist] - not a playlist")
		return ctx.Status(http.StatusBadRequest).JSON("not a playlist")
	}
	// create new task and set the handler. the handler will create or update a new task in the db
	// in the case where the conversion fails, it sets the status to failed and ditto for success
	// serialize linkInfo
	ser, err := json.Marshal(&taskData)
	if err != nil {
		log.Printf("[controller][conversion][ConvertPlaylist] - error marshalling link info: %v", err)
		return ctx.Status(http.StatusInternalServerError).JSON("error marshalling link info")
	}
	// create new task
	conversionTask := asynq.NewTask(fmt.Sprintf("playlist:conversion:%s", taskData.TaskID), ser, asynq.Retention(time.Hour*24*7*4), asynq.Queue(queue.PlaylistConversionTask))

	// enqueue the task
	enquedTask, enqErr := c.Asynq.Enqueue(conversionTask, asynq.Queue(queue.PlaylistConversionQueue), asynq.TaskID(taskData.TaskID), asynq.Unique(time.Second*60))
	if enqErr != nil {
		log.Printf("[controller][conversion][ConvertPlaylist] - error enqueuing task: %v", enqErr)
		return ctx.Status(http.StatusInternalServerError).JSON("error enqueuing task")
	}

	database := db.NewDB{DB: c.DB}
	redisOpts, err := redis.ParseURL(os.Getenv("REDISCLOUD_URL"))
	if err != nil {
		log.Printf("[controller][conversion][ConvertPlaylist] - error parsing redis url: %v", err)
		return ctx.Status(http.StatusInternalServerError).JSON("error parsing redis url")
	}

	_taskId, dbErr := database.CreateOrUpdateTask(enquedTask.ID, shorturl, app.UID.String(), linkInfo.EntityID)
	if dbErr != nil {
		log.Printf("[controller][conversion][ConvertPlaylist] - error creating task: %v", dbErr)
		return ctx.Status(http.StatusInternalServerError).JSON("error creating task")
	}
	orchdioQueue := queue.NewOrchdioQueue(c.Asynq, c.DB, c.Red)

	// Conversion task response to be polled later
	res := &blueprint.NewTask{ID: string(_taskId)}

	// NB: THE SIDE EFFECT OF THIS IS THAT WHEN WE RESTART THE SERVER FOR EXAMPLE, WE LOSE
	// THE HANDLER ATTACHED. THIS IS BECAUSE WE'RE TRIGGERING THE HANDLER HERE IN THE
	// CONVERSION HANDLER. WE SHOULD BE ABLE TO FIX THIS BY HAVING A HANDLER THAT
	// ALWAYS RUNS AND FETCHES TASKS FROM A STORE AND ATTACH THEM TO A HANDLER.
	// ENG-NOTE: after investigating, we're handling errors and queue failures in the queue handler in main.go
	// this block shouldnt be source of sorrow again but its still needed to keep around as it does indeed handle panics, just in case
	defer func() error {
		// handle panic
		if r := recover(); r != nil {
			log.Printf("[controller][conversion][ConvertPlaylist] - gracefully ignoring this")
			log.Printf("[controller][conversion][ConvertPlaylist] - task already queued%v", r)
			inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisOpts.Addr, Password: redisOpts.Password})

			queueInfo, err := inspector.GetQueueInfo(queue.PlaylistConversionQueue)
			if err != nil {
				log.Printf("[controller][conversion][ConvertPlaylist] - error getting queue info: %v", err)
				return err
			}

			// update the task to success. because this seems to be a race condition in production where
			// it duplicates task scheduling even though the task is already queued
			// update task to success
			err = database.UpdateTaskStatus(enquedTask.ID, "pending")
			if err != nil {
				log.Printf("[controller][conversion][ConvertPlaylist] - error unmarshalling task data: %v", err)
			}

			if queueInfo.Paused {
				log.Printf("[controller][conversion][ConvertPlaylist] - queue is paused. resuming it")
				err = inspector.UnpauseQueue(queue.PlaylistConversionQueue)
				if err != nil {
					log.Printf("[controller][conversion][ConvertPlaylist] - error resuming queue. Dumping error: ")
					spew.Dump(err)
					log.Printf("\n")
				}
				log.Printf("[controller][conversion][ConvertPlaylist] - queue resumed")
			}

			log.Printf("[controller][conversion][ConvertPlaylist] - task updated to success")
			if r.(string) == "asynq: multiple registrations for playlist:conversion" {
				log.Printf("[controller][conversion][ConvertPlaylist] - task already queued")
			}
		}
		log.Printf("[controller][conversion][ConvertPlaylist] - recovered from panic.. task already queued")

		return util.SuccessResponse(ctx, http.StatusOK, res)
	}()
	c.AsynqMux.HandleFunc("playlist:conversion:", orchdioQueue.PlaylistTaskHandler)
	log.Printf("[controller][conversion][ConvertPlaylist] - task handler attached")
	return util.SuccessResponse(ctx, http.StatusCreated, res)
}

// GetPlaylistTask returns a playlist
func (c *Controller) GetPlaylistTask(ctx *fiber.Ctx) error {
	log.Printf("[controller][conversion][GetPlaylistTaskStatus] - getting playlist task status")
	taskId := ctx.Params("taskId")
	log.Printf("[controller][conversion][GetPlaylistTaskStatus] - taskId: %s", taskId)
	database := db.NewDB{DB: c.DB}
	taskRecord, err := database.FetchTask(taskId)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[controller][conversion][GetPlaylistTaskStatus] - task not found")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "task not found")
		}
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - error fetching task: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "error fetching task")
	}

	taskUUID, err := uuid.Parse(taskId)
	if err != nil {
		log.Printf("[controller][conversion][GetPlaylistTaskStatus][warning] - not a playlist task, most likely a short url")

		// we are casting the result into an interface. Before, we wanted to type each response
		// but with the current implementation, we don't care about that here because each result we
		// deserialize here and return to client is already typed based on what the entity is. For example
		// in this case, it isn't a playlist task, it might be a track or follow task. Casting into interface
		// is ok because each of these results were typed from a struct before being serialized into the DB for storage.
		var res interface{}
		// HACK: to check if the task is a playlist task result as to be able to format the right type
		err = json.Unmarshal([]byte(taskRecord.Result), &res)
		if err != nil {
			log.Printf("[controller][conversion][GetPlaylistTaskStatus] - not a playlist task")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not deserialize task result")
		}

		result := &blueprint.TaskResponse{
			ID:      taskId,
			Status:  taskRecord.Status,
			Payload: res,
		}

		return util.SuccessResponse(ctx, http.StatusOK, result)
	}

	if taskRecord.Status == "failed" {
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - task ")
		// deserialize the taskrecord result into blueprint.TaskErrorPayload
		var res blueprint.TaskErrorPayload
		err = json.Unmarshal([]byte(taskRecord.Result), &res)
		if err != nil {
			log.Printf("[controller][conversion][GetPlaylistTaskStatus] - error deserializing task result: %v", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "task failed. could not process or unknown error")
		}

		// create a new task response
		result := &blueprint.TaskResponse{
			ID:      taskId,
			Status:  taskRecord.Status,
			Payload: res,
		}
		return util.ErrorResponse(ctx, http.StatusOK, result, "")
	}

	if taskRecord.Status == "completed" {
		// deserialize the task data
		var res blueprint.PlaylistConversion
		err = json.Unmarshal([]byte(taskRecord.Result), &res)
		if err != nil {
			log.Printf("[controller][conversion][GetPlaylistTaskStatus] error- could not deserialize task data: %v", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "could not deserialize playlist task result")
		}

		if res.Meta.URL == "" {
			taskResponse := &blueprint.TaskResponse{
				ID:      taskId,
				Payload: nil,
				Status:  "pending",
			}
			return util.SuccessResponse(ctx, http.StatusOK, taskResponse)
		}

		res.Meta.Entity = "playlist"
		taskResponse := &blueprint.TaskResponse{
			ID:      taskUUID.String(),
			Payload: res,
			Status:  taskRecord.Status,
		}

		return util.SuccessResponse(ctx, http.StatusOK, taskResponse)
	}

	if taskRecord.Status == "pending" {
		taskResponse := &blueprint.TaskResponse{
			ID:      taskId,
			Payload: nil,
			Status:  "pending",
		}
		return util.SuccessResponse(ctx, http.StatusOK, taskResponse)
	}
	log.Printf("[controller][conversion][GetPlaylistTaskStatus] - task status: %s", taskRecord.Status)
	return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unknown error occurred while updating task record status")
}

// DeletePlaylistTask deletes a playlist task
func (c *Controller) DeletePlaylistTask(ctx *fiber.Ctx) error {
	log.Printf("[controller][conversion][DeletePlaylistTask] - deleting playlist task")
	taskId := ctx.Params("taskId")
	log.Printf("[controller][conversion][DeletePlaylistTask] - taskId: %s", taskId)
	database := db.NewDB{DB: c.DB}
	err := database.DeleteTask(taskId)
	if err != nil {
		log.Printf("[controller][conversion][DeletePlaylistTask] - error deleting task: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "error deleting task")
	}
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}
