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
	"orchdio/queue"
	"orchdio/util"
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
	log.Printf("[controller][conversion][EchoConversion] - echo conversion")

	user := ctx.Locals("user").(*blueprint.User)
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)
	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("[controller][conversion][EchoConversion] - not a playlist")
		return ctx.Status(http.StatusBadRequest).JSON("not a playlist")
	}
	// create new task and set the handler. the handler will create or update a new task in the db
	// in the case where the conversion fails, it sets the status to failed and ditto for success
	// serialize linkInfo
	tInfo := blueprint.PlaylistTaskData{
		LinkInfo: linkInfo,
		User:     user,
	}
	ser, err := json.Marshal(&tInfo)
	if err != nil {
		log.Printf("[controller][conversion][EchoConversion] - error marshalling link info: %v", err)
		return ctx.Status(http.StatusInternalServerError).JSON("error marshalling link info")
	}
	uniqueId, _ := uuid.NewUUID()
	// create new task
	conversionTask := asynq.NewTask(uniqueId.String(), ser, asynq.Retention(time.Hour*24*7*4))
	// enqueue the task
	_, enqErr := c.Asynq.Enqueue(conversionTask)
	if enqErr != nil {
		log.Printf("[controller][conversion][EchoConversion] - error enqueuing task: %v", enqErr)
		return ctx.Status(http.StatusInternalServerError).JSON("error enqueuing task")
	}

	orchdioQueue := queue.NewOrchdioQueue(c.Asynq, c.AsynqServer, c.DB, c.Red)
	// NB: THE SIDE EFFECT OF THIS IS THAT WHEN WE RESTART THE SERVER FOR EXAMPLE, WE LOSE
	// THE HANDLER ATTACHED. THIS IS BECAUSE WE'RE TRIGGERING THE HANDLER HERE IN THE
	// CONVERSION HANDLER. WE SHOULD BE ABLE TO FIX THIS BY HAVING A HANDLER THAT
	// ALWAYS RUNS AND FETCHES TASKS FROM A STORE AND ATTACH THEM TO A HANDLER.
	// FIXME: more investigations
	c.AsynqMux.HandleFunc(uniqueId.String(), orchdioQueue.PlaylistTaskHandler)
	//stErr := c.AsynqServer.Start(c.AsynqMux)
	//if stErr != nil {
	//	log.Printf("[controller][conversion][EchoConversion][error] - could not start Asynq server: %v", stErr)
	//	return util.ErrorResponse(ctx, http.StatusInternalServerError, "could not start Asynq server")
	//}

	res := map[string]string{
		"taskId": uniqueId.String(),
	}
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
			return util.ErrorResponse(ctx, http.StatusNotFound, "task not found")
		}
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - error fetching task: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "error fetching task")
	}

	// deserialize the task data
	var res blueprint.PlaylistConversion
	err = json.Unmarshal([]byte(taskRecord.Result), &res)
	if err != nil {
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - error unmarshalling task data: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "error unmarshalling task data")
	}

	var data interface{}

	if res.URL == "" {
		data = nil
	} else {
		data = res
	}

	result := map[string]interface{}{
		"taskId": taskId,
		"status": taskRecord.Status,
		"data":   data,
	}
	return util.SuccessResponse(ctx, http.StatusOK, result)
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
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "error deleting task")
	}
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}
