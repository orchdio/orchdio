package platforms

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/queue"
	"orchdio/universal"
	"orchdio/util"
	"os"
	"strings"
	"time"
)

// Platforms represents the structure for the platforms
type Platforms struct {
	Redis       *redis.Client
	DB          *sqlx.DB
	AsynqClient *asynq.Client
	AsynqMux    *asynq.ServeMux
}

func NewPlatform(r *redis.Client, db *sqlx.DB, asynqClient *asynq.Client, asynqMux *asynq.ServeMux) *Platforms {
	return &Platforms{Redis: r, DB: db, AsynqClient: asynqClient, AsynqMux: asynqMux}
}

// ConvertEntity returns the link to a track on several platforms
func (p *Platforms) ConvertEntity(ctx *fiber.Ctx) error {
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)
	app := ctx.Locals("app").(*blueprint.DeveloperApp)

	// we fetch the credentials for the platform we're converting to
	// and then we pass it to the queue to be used by the worker
	// to make the conversion
	targetPlatform := linkInfo.TargetPlatform
	if targetPlatform == "" {
		log.Printf("\n[controllers][platforms][ConvertEntity] error - %v\n", "Target platform not specified")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "target platform not specified", "Target platform not specified")
	}

	// make sure we're actually handling for track alone, not playlist.
	if strings.Contains(linkInfo.Entity, "track") {
		log.Printf("\n[controllers][platforms][deezer][ConvertEntity] [info] - It is a track URL")

		conversion, conversionError := universal.ConvertTrack(linkInfo, p.Redis, p.DB)
		if conversionError != nil {
			if conversionError == blueprint.ENOTIMPLEMENTED {
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - %v\n", "Not implemented")
				return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
			}

			if conversionError == blueprint.ECREDENTIALSMISSING {
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - %v\n", "Credentials missing")
				return util.ErrorResponse(ctx, http.StatusUnauthorized, "credentials missing", fmt.Sprintf("Credentials missing for %v. Please update your app with your %s credentials and try again.", linkInfo.TargetPlatform, strings.ToUpper(linkInfo.TargetPlatform)))
			}

			log.Printf("\n[controllers][platforms][base][ConvertEntity] - Could not convert track")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, conversionError, "An internal error occurred")
		}

		log.Printf("\n[controllers][platforms][ConvertEntity] - converted %v with URL %v\n", linkInfo.Entity, linkInfo.TargetLink)

		// HACK: insert a new task in the DB directly and return the ID as part of the
		// conversion response. We are saving directly because for playlists, we run them in asynq job queue
		// and for tracks we don't. we use this task info to implement being able to share a conversion page
		// (for example on zoove) with a link because it carries data unique to the operation.
		database := db.NewDB{DB: p.DB}
		uniqueId, _ := uuid.NewUUID()
		// generate a URL friendly short ID. this is what we're going to send to the API response
		// and user can use it in the conversion URL.
		shortURL := util.GenerateShortID()
		serialized, err := json.Marshal(conversion)
		if conversion == nil {
			log.Printf("\n[controllers][platforms][ConvertEntity] - conversion is nil %v\n", err)
			return util.ErrorResponse(ctx, http.StatusNotFound, err, "An internal error occurred")
		}

		// add the task ID to the conversion response
		conversion.ShortURL = string(shortURL)

		if err != nil {
			log.Printf("[db][CreateTrackTaskRecord] error serializing result. %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred. Could not deserialize result")
		}

		_, err = database.CreateTrackTaskRecord(uniqueId.String(), string(shortURL), linkInfo.EntityID, app.UID.String(), serialized)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertEntity] - Could not create task record")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred and could not create task record.")
		}

		conversion.Entity = "track"
		taskResponse := blueprint.TaskResponse{
			Payload: conversion,
		}

		return util.SuccessResponse(ctx, http.StatusOK, taskResponse)
	}

	// make sure we're actually handling for playlist alone, not track.
	if strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("\n[controllers][platforms][deezer][ConvertEntity] warning - It is a playlist URL")

		uniqueId := uuid.New().String()
		shortURL := util.GenerateShortID()
		taskData := &blueprint.PlaylistTaskData{
			LinkInfo: linkInfo,
			App:      app,
			TaskID:   uniqueId,
			ShortURL: string(shortURL),
		}

		if !strings.Contains(linkInfo.Entity, "playlist") {
			log.Printf("[controller][conversion][EchoConversion] - not a playlist")
			return ctx.Status(http.StatusBadRequest).JSON("not a playlist")
		}

		orchdioQueue := queue.NewOrchdioQueue(p.AsynqClient, p.DB, p.Redis, p.AsynqMux)

		// create new task and set the handler. the handler will create or update a new task in the db
		// in the case where the conversion fails, it sets the status to failed and ditto for success
		// serialize linkInfo
		ser, err := json.Marshal(&taskData)
		if err != nil {
			log.Printf("[controller][conversion][EchoConversion] - error marshalling link info: %v", err)
			return ctx.Status(http.StatusInternalServerError).JSON("error marshalling link info")
		}
		// create new task
		conversionTask, err := orchdioQueue.NewTask(fmt.Sprintf("playlist:conversion:%s", taskData.TaskID), queue.PlaylistConversionTask, 1, ser)
		enqErr := orchdioQueue.EnqueueTask(conversionTask, queue.PlaylistConversionQueue, taskData.TaskID, time.Second*1)
		if enqErr != nil {
			log.Printf("[controller][conversion][EchoConversion] - error enqueuing task: %v", enqErr)
			return ctx.Status(http.StatusInternalServerError).JSON("error enqueuing task")
		}

		database := db.NewDB{DB: p.DB}
		redisOpts, err := redis.ParseURL(os.Getenv("REDISCLOUD_URL"))
		if err != nil {
			log.Printf("[controller][conversion][EchoConversion] - error parsing redis url: %v", err)
			return ctx.Status(http.StatusInternalServerError).JSON("error parsing redis url")
		}

		// we were saving the task developer as user before but now we save the app
		_taskId, dbErr := database.CreateOrUpdateTask(uniqueId, string(shortURL), app.UID.String(), linkInfo.EntityID)
		if dbErr != nil {
			log.Printf("[controller][conversion][EchoConversion] - error creating task: %v", dbErr)
			return ctx.Status(http.StatusInternalServerError).JSON("error creating task")
		}

		// Conversion task response to be polled later
		res := &blueprint.TaskResponse{
			ID:      string(_taskId),
			Payload: nil,
			Status:  "pending",
		}

		// NB: THE SIDE EFFECT OF THIS IS THAT WHEN WE RESTART THE SERVER FOR EXAMPLE, WE LOSE
		// THE HANDLER ATTACHED. THIS IS BECAUSE WE'RE TRIGGERING THE HANDLER HERE IN THE
		// CONVERSION HANDLER. WE SHOULD BE ABLE TO FIX THIS BY HAVING A HANDLER THAT
		// ALWAYS RUNS AND FETCHES TASKS FROM A STORE AND ATTACH THEM TO A HANDLER.
		// FIXME: more investigations
		defer func() error {
			// handle panic
			if r := recover(); r != nil {
				log.Printf("[controller][conversion][EchoConversion] - gracefully ignoring this")
				log.Printf("[controller][conversion][EchoConversion] - task already queued%v", r)
				inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisOpts.Addr, Password: redisOpts.Password})
				// get the task
				_, err := inspector.GetTaskInfo(queue.PlaylistConversionQueue, uniqueId)
				if err != nil {
					log.Printf("[controller][conversion][EchoConversion] - error getting task info: %v", err)
					return err
				}
				log.Printf("[controller][conversion][EchoConversion] - task info:")
				queueInfo, err := inspector.GetQueueInfo(queue.PlaylistConversionQueue)
				if err != nil {
					log.Printf("[controller][conversion][EchoConversion] - error getting queue info: %v", err)
					return err
				}

				// update the task to success. because this seems to be a race condition in production where
				// it duplicates task scheduling even though the task is already queued
				// update task to success
				err = database.UpdateTaskStatus(uniqueId, "pending")
				if err != nil {
					log.Printf("[controller][conversion][EchoConversion] - error unmarshalling task data: %v", err)
				}

				if queueInfo.Paused {
					log.Printf("[controller][conversion][EchoConversion] - queue is paused. resuming it")
					err = inspector.UnpauseQueue(queue.PlaylistConversionQueue)
					if err != nil {
						log.Printf("[controller][conversion][EchoConversion] - error resuming queue: ")
						spew.Dump(err)
					}
					log.Printf("[controller][conversion][EchoConversion] - queue resumed")
				}

				log.Printf("[controller][conversion][EchoConversion] - task updated to success")

				if r.(string) == "asynq: multiple registrations for playlist:conversion" {
					log.Printf("[controller][conversion][EchoConversion] - task already queued")
				}
			}
			log.Printf("[controller][conversion][EchoConversion] - recovered from panic.. task already queued")

			return util.SuccessResponse(ctx, http.StatusOK, res)
		}()
		orchdioQueue.RunTask(fmt.Sprintf("playlist:conversion:%s", uniqueId), orchdioQueue.PlaylistTaskHandler)
		log.Printf("[controller][conversion][EchoConversion] - task handler attached")
		return util.SuccessResponse(ctx, http.StatusCreated, res)
	}

	log.Printf("\n[controllers][platforms][ConvertEntity] error - %v\n", "It is not a playlist or track URL")
	return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid URL")
}
