package platforms

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	logger2 "orchdio/logger"
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

	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", ctx.Get("x-orchdio-app-pub-key")).String,
		Platform:             zap.String("platform", ctx.Get("x-orchdio-platform")).String,
	}
	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)

	// we fetch the credentials for the platform we're converting to
	// and then we pass it to the queue to be used by the worker
	// to make the conversion
	targetPlatform := linkInfo.TargetPlatform
	if targetPlatform == "" {
		orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Target platform not specified")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "target platform not specified", "Target platform not specified")
	}

	// make sure we're actually handling for track alone, not playlist.
	if strings.Contains(linkInfo.Entity, "track") {
		conversion, conversionError := universal.ConvertTrack(linkInfo, p.Redis, p.DB)
		if conversionError != nil {
			if errors.Is(conversionError, blueprint.ENOTIMPLEMENTED) {
				orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Not implemented")
				return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
			}

			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Could not convert track", zap.Error(conversionError))

			if strings.Contains(conversionError.Error(), "credentials not provided") {
				orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Credentials missing", zap.Error(conversionError))
				return util.ErrorResponse(ctx, http.StatusUnauthorized, "credentials missing", fmt.Sprintf("%s. Please update your app with the missing platform's credentials.", conversionError.Error()))
			}

			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Could not convert track", zap.Error(conversionError))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, conversionError, "An internal error occurred")
		}

		orchdioLogger.Info("[controllers][platforms][ConvertEntity] [info] - converted %v with URL %v", zap.Any("entity_info", linkInfo))

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
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Could not convert track", zap.Error(err))
			return util.ErrorResponse(ctx, http.StatusNotFound, err, "An internal error occurred")
		}

		// add the task ID to the conversion response
		conversion.ShortURL = string(shortURL)

		if err != nil {
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Could not convert track", zap.Error(err))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred. Could not deserialize result")
		}

		_, err = database.CreateTrackTaskRecord(uniqueId.String(), string(shortURL), linkInfo.EntityID, app.UID.String(), serialized)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Could not create task record", zap.Error(err))
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
		orchdioLogger.Info("[controllers][platforms][ConvertEntity] [info] - It is a playlist URL")

		uniqueId := uuid.New().String()
		shortURL := util.GenerateShortID()
		taskData := &blueprint.PlaylistTaskData{
			LinkInfo: linkInfo,
			App:      app,
			TaskID:   uniqueId,
			ShortURL: string(shortURL),
		}

		if !strings.Contains(linkInfo.Entity, "playlist") {
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Not a playlist")
			return ctx.Status(http.StatusBadRequest).JSON("not a playlist")
		}

		orchdioQueue := queue.NewOrchdioQueue(p.AsynqClient, p.DB, p.Redis, p.AsynqMux)

		// create new task and set the handler. the handler will create or update a new task in the db
		// in the case where the conversion fails, it sets the status to failed and ditto for success
		// serialize linkInfo
		ser, err := json.Marshal(&taskData)
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - could not deserialize link info", zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON("error marshalling link info")
		}
		// create new task
		conversionTask, err := orchdioQueue.NewTask(fmt.Sprintf("playlist:conversion:%s", taskData.TaskID), queue.PlaylistConversionTask, 1, ser)
		enqErr := orchdioQueue.EnqueueTask(conversionTask, queue.PlaylistConversionQueue, taskData.TaskID, time.Second*1)
		if enqErr != nil {
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - could not enqueue task", zap.Error(err), zap.String("task_id", taskData.TaskID))
			return ctx.Status(http.StatusInternalServerError).JSON("error enqueuing task")
		}

		database := db.NewDB{DB: p.DB}
		_, err = redis.ParseURL(os.Getenv("REDISCLOUD_URL"))
		if err != nil {
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - could not parse redis url", zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON("error parsing redis url")
		}

		// we were saving the task developer as user before but now we save the app
		_taskId, dbErr := database.CreateOrUpdateTask(uniqueId, string(shortURL), app.UID.String(), linkInfo.EntityID)
		if dbErr != nil {
			orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - could not create task", zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON("error creating task")
		}

		// Conversion task response to be polled later
		res := &blueprint.TaskResponse{
			ID:      string(_taskId),
			Payload: nil,
			Status:  "pending",
		}

		orchdioLogger.Info("[controllers][platforms][ConvertEntity] [info] - Task handler attached", zap.String("task id", taskData.TaskID))
		return util.SuccessResponse(ctx, http.StatusCreated, res)
	}

	log.Printf("\n[controllers][platforms][ConvertEntity] error - %v\n", "It is not a playlist or track URL")
	orchdioLogger.Error("[controllers][platforms][ConvertEntity] error - Not a playlist")
	return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid URL")
}
