package platforms

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/queue"
	"orchdio/universal"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Platforms represents the structure for the platforms
type Platforms struct {
	Redis         *redis.Client
	DB            *sqlx.DB
	Logger        *blueprint.OrchdioLoggerOptions
	Queue         queue.QueueService
	WebhookSender svixwebhook.SvixInterface
}

func NewPlatform(r *redis.Client, db *sqlx.DB, queue queue.QueueService, webhookSender svixwebhook.SvixInterface) *Platforms {
	return &Platforms{Redis: r, DB: db, Queue: queue, WebhookSender: webhookSender}
}

func (p *Platforms) ConvertTrack(ctx *fiber.Ctx) error {
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)
	app := ctx.Locals("app").(*blueprint.DeveloperApp)

	targetPlatform := linkInfo.TargetPlatform
	if targetPlatform == "" {
		log.Printf("\n[controllers][platforms] No target platform found in linkInfo\n")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "target platform not specified", "Target platform not specified.")
	}

	if strings.Contains(linkInfo.Entity, "track") {
		conversion, conversionError := universal.ConvertTrack(linkInfo, p.Redis, p.DB, p.WebhookSender)
		if conversionError != nil {
			if errors.Is(conversionError, blueprint.ErrNotImplemented) {
				log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", "Not implemented")
				return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
			}

			log.Printf("\n[controllers][platforms][%s —— %s][ConvertTrack] error - %v\n", linkInfo.Platform, linkInfo.TargetPlatform, conversionError.Error())

			if strings.Contains(conversionError.Error(), "credentials not provided") {
				log.Printf("\n[controllers][platforms][%s —— %s][ConvertTrack] - %v\n", linkInfo.Platform, linkInfo.TargetPlatform, "Credentials missing")
				return util.ErrorResponse(ctx, http.StatusUnauthorized, "credentials missing", fmt.Sprintf("%s. Please update your app with the missing platform's credentials.", conversionError.Error()))
			}

			log.Printf("\n[controllers][platforms][%s —— %s][ConvertTrack] - Could not convert track: error — %v", linkInfo.TargetPlatform, linkInfo.Platform, conversionError.Error())
			return util.ErrorResponse(ctx, http.StatusInternalServerError, conversionError, "An internal error occurred")
		}

		log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v with URL %v\n", linkInfo.Entity, linkInfo.TargetLink)

		// HACK: insert a new task in the DB directly and return the ID as part of the
		// conversion response. We are saving directly because for playlists, we run them in asynq job queue
		// and for tracks, we want to return the full result of the track conversion (since it takes less time).
		// we use this task info to implement being able to share a conversion page
		// (for example on Zoove) with a link because it carries data unique to the operation.
		database := db.NewDB{DB: p.DB}
		uniqueId, _ := uuid.NewUUID()
		// generate a URL friendly short ID. this is what we're going to send to the API response
		// and user can use it in the conversion URL.
		shortURL := util.GenerateShortID()
		serialized, err := json.Marshal(conversion)
		if conversion == nil {
			log.Printf("\n[controllers][platforms][controllers][platforms][%s —— %s][ConvertTrack] - conversion is nil %v\n", linkInfo.Platform, linkInfo.TargetPlatform, err)
			return util.ErrorResponse(ctx, http.StatusNotFound, err, "An internal error occurred")
		}

		// add the task ID to the conversion response
		conversion.UniqueID = string(shortURL)
		if err != nil {
			log.Printf("[db][CreateTrackTaskRecord] error serializing result. %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred. Could not deserialize result")
		}

		_, err = database.CreateTrackTaskRecord(uniqueId.String(), string(shortURL), linkInfo.EntityID, app.UID.String(), serialized)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertTrack] - Could not create task record")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred and could not create task record.")
		}

		response := &blueprint.TrackConversion{
			Entity:         "track",
			Platforms:      conversion.Platforms,
			UniqueID:       conversion.UniqueID,
			SourcePlatform: linkInfo.Platform,
			TargetPlatform: linkInfo.TargetPlatform,
		}
		return util.SuccessResponse(ctx, http.StatusOK, response)
	}

	return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
}

// ConvertPlaylist returns the link to a track on several platforms
func (p *Platforms) ConvertPlaylist(ctx *fiber.Ctx) error {
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)
	app := ctx.Locals("app").(*blueprint.DeveloperApp)

	// we fetch the credentials for the platform we're converting to
	// and then we pass it to the queue to be used by the worker
	// to make the conversion
	targetPlatform := linkInfo.TargetPlatform
	if targetPlatform == "" {
		log.Printf("\n[controllers][platforms][ConvertPlaylist] error - %v\n", "Target platform not specified")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "target platform not specified", "Target platform not specified")
	}
	// make sure we're actually handling for playlist alone, not track.
	if strings.Contains(linkInfo.Entity, "playlist") {
		uniqueId := uuid.New().String()
		shortURL := util.GenerateShortID()
		taskData := &blueprint.PlaylistTaskData{
			LinkInfo: linkInfo,
			App:      app,
			TaskID:   uniqueId,
			ShortURL: string(shortURL),
		}

		// orchdioQueue := queue.NewOrchdioQueue(p.AsynqClient, p.DB, p.Redis, p.AsynqMux)

		// create new task and set the handler. the handler will create or update a new task in the db
		// in the case where the conversion fails, it sets the status to failed and ditto for success
		// serialize linkInfo
		ser, err := json.Marshal(&taskData)
		if err != nil {
			log.Printf("[controller][conversion][EchoConversion] - error marshalling link info: %v", err)
			return ctx.Status(http.StatusInternalServerError).JSON("error marshalling link info")
		}
		// create new task
		conversionTask, err := p.Queue.NewTask(fmt.Sprintf("%s_%s", blueprint.PlaylistConversionTaskTypePattern, taskData.TaskID), blueprint.PlaylistConversionTaskTypePattern, 1, ser)
		enqErr := p.Queue.EnqueueTask(conversionTask, blueprint.PlaylistConversionQueueName, taskData.TaskID, time.Second*1)
		if enqErr != nil {
			log.Printf("[controller][conversion][EchoConversion] - error enqueuing task: %v", enqErr)
			return ctx.Status(http.StatusInternalServerError).JSON("error enqueuing task")
		}

		_, err = redis.ParseURL(os.Getenv("REDISCLOUD_URL"))
		if err != nil {
			log.Printf("[controller][conversion][EchoConversion] - error parsing redis url: %v", err)
			return ctx.Status(http.StatusInternalServerError).JSON("error parsing redis url")
		}
		database := db.NewDB{DB: p.DB}

		// we were saving the task developer as user before but now we save the app
		_taskId, dbErr := database.CreateOrUpdateTask(uniqueId, string(shortURL), app.UID.String(), linkInfo.EntityID)
		if dbErr != nil {
			log.Printf("[controller][conversion][EchoConversion] - error creating task: %v", dbErr)
			return ctx.Status(http.StatusInternalServerError).JSON("error creating task")
		}

		// TrackConversion task response to be polled later
		res := &blueprint.PlaylistTaskResponse{
			TaskID:   string(_taskId),
			UniqueID: string(shortURL),
			Payload:  nil,
			Status:   "pending",
		}

		log.Printf("[controller][conversion][EchoConversion] - task handler attached")
		return util.SuccessResponse(ctx, http.StatusCreated, res)
	}

	log.Printf("\n[controllers][platforms][ConvertPlaylist] error - %v\n", "It is not a playlist or track URL")
	return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid URL")
}
