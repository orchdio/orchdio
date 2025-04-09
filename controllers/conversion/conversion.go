package conversion

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/util"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
)

// Controller is the controller for the conversion service.
type Controller struct {
	DB          *sqlx.DB
	Red         *redis.Client
	Asynq       *asynq.Client
	AsynqServer *asynq.Server
	AsynqMux    *asynq.ServeMux
}

// NewConversionController creates a new conversion controller.
func NewConversionController(db *sqlx.DB, red *redis.Client, asynqClient *asynq.Client, asynqserver *asynq.Server, mux *asynq.ServeMux) *Controller {

	res := &Controller{
		DB:          db,
		Red:         red,
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
	database := db.NewDB{DB: c.DB}
	taskRecord, err := database.FetchTask(taskId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[controller][conversion][GetPlaylistTaskStatus] - task not found: Task ID: \n%s", taskId)
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "task not found")
		}
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - error fetching task: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "error fetching task")
	}

	// check if the task is a playlist task. this is for short url support. A task has a short url
	// so when we get a task, we are technically getting two types of tasks in two different contexts.
	// in this case, if the task is not a valid uuid, that means it's a short url task. we then return
	// the task result task (context) data.
	taskUUID, pErr := uuid.Parse(taskId)
	if pErr != nil {
		log.Printf("[controller][conversion][GetPlaylistTaskStatus][warning] - not a playlist task, most likely a short url")

		// we are casting the result into an interface. Before, we wanted to type each response
		// but with the current implementation, we don't care about that here because each result we
		// deserialize here and return to client is already typed based on what the entity is. For example
		// in this case, it isn't a playlist task, it might be a track or follow task. Casting into interface
		// is ok because each of these results were typed from a struct before being serialized into the DB for storage.
		var res interface{}
		// HACK: to check if the task is a playlist task result as to be able to format the right type
		mErr := json.Unmarshal([]byte(taskRecord.Result), &res)
		if mErr != nil {
			log.Printf("[controller][conversion][GetPlaylistTaskStatus] - not a playlist task: Error: \n%v", mErr)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not deserialize task result")
		}

		result := &blueprint.PlaylistTaskResponse{
			ID:      taskId,
			Status:  taskRecord.Status,
			Payload: res,
		}
		return util.SuccessResponse(ctx, http.StatusOK, result)
	}

	if taskRecord.Status == blueprint.TaskStatusFailed {
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - task ")
		// deserialize the taskrecord result into blueprint.TaskErrorPayload
		var res blueprint.TaskErrorPayload
		err = json.Unmarshal([]byte(taskRecord.Result), &res)
		if err != nil {
			log.Printf("[controller][conversion][GetPlaylistTaskStatus] - error deserializing task result: %v", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "task failed. could not process or unknown error")
		}

		// create a new task response
		result := &blueprint.PlaylistTaskResponse{
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
			taskResponse := &blueprint.PlaylistTaskResponse{
				ID:      taskId,
				Payload: nil,
				Status:  "pending",
			}
			return util.SuccessResponse(ctx, http.StatusOK, taskResponse)
		}

		res.Meta.Entity = "playlist"
		taskResponse := &blueprint.PlaylistTaskResponse{
			ID:      taskUUID.String(),
			Payload: res,
			Status:  taskRecord.Status,
		}

		return util.SuccessResponse(ctx, http.StatusOK, taskResponse)
	}

	if taskRecord.Status == "pending" {
		taskResponse := &blueprint.PlaylistTaskResponse{
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
