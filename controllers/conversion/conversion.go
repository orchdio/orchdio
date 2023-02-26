package conversion

import (
	"database/sql"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/vmihailenco/taskq/v3"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/util"
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
