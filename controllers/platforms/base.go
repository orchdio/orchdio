package platforms

import (
	"context"
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
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/queue"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/tidal"
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

	// make sure we're actually handling for track alone, not playlist.
	if strings.Contains(linkInfo.Entity, "track") {
		log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - %v\n", "It is a track URL")

		conversion, err := universal.ConvertTrack(linkInfo, p.Redis)
		if err != nil {
			if err == blueprint.ENOTIMPLEMENTED {
				log.Printf("\n[controllers][platforms][deezer][ConvertEntity] error - %v\n", "Not implemented")
				return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
			}

			log.Printf("\n[controllers][platforms][base][ConvertEntity] - Could not convert track")
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
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
		const format = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-" // 0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-
		sid, err := shortid.New(1, format, 2342)
		if err != nil {
			log.Printf("\n[controllers][platforms][ConvertEntity] - could not generate short id %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
		}
		uID, _ := sid.Generate()

		serialized, err := json.Marshal(conversion)
		if conversion == nil {
			log.Printf("\n[controllers][platforms][ConvertEntity] - conversion is nil %v\n", err)
			return util.ErrorResponse(ctx, http.StatusNotFound, err, "An internal error occurred")
		}

		// add the task ID to the conversion response
		conversion.ShortURL = uID

		if err != nil {
			log.Printf("[db][CreateTrackTaskRecord] error serializing result. %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred. Could not deserialize result")
		}

		_, err = database.CreateTrackTaskRecord(uniqueId.String(), uID, linkInfo.EntityID, serialized)
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
		log.Printf("\n[controllers][platforms][ConvertEntity] error - %v\n", "It is a playlist URL")

		user := ctx.Locals("developer").(*blueprint.User)
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
			User:     user,
			TaskID:   uniqueId,
			ShortURL: shorturl,
		}

		if !strings.Contains(linkInfo.Entity, "playlist") {
			log.Printf("[controller][conversion][EchoConversion] - not a playlist")
			return ctx.Status(http.StatusBadRequest).JSON("not a playlist")
		}
		// create new task and set the handler. the handler will create or update a new task in the db
		// in the case where the conversion fails, it sets the status to failed and ditto for success
		// serialize linkInfo
		ser, err := json.Marshal(&taskData)
		if err != nil {
			log.Printf("[controller][conversion][EchoConversion] - error marshalling link info: %v", err)
			return ctx.Status(http.StatusInternalServerError).JSON("error marshalling link info")
		}
		// create new task
		conversionTask := asynq.NewTask(fmt.Sprintf("playlist:conversion:%s", taskData.TaskID), ser, asynq.Retention(time.Hour*24*7*4), asynq.Queue(queue.PlaylistConversionTask))

		//conversionCtx := context.WithValue(context.Background(), "payload", taskData)
		//log.Printf("[controller][conversion][EchoConversion] - conversionCtx: %v", conversionCtx)
		// enqueue the task
		// enquedTask, enqErr := c.Asynq.EnqueueContext(conversionCtx, conversionTask, asynq.Queue(queue.PlaylistConversionQueue), asynq.TaskID(taskData.TaskID), asynq.Unique(time.Second*60))
		enquedTask, enqErr := p.AsynqClient.Enqueue(conversionTask, asynq.Queue(queue.PlaylistConversionQueue), asynq.TaskID(taskData.TaskID), asynq.Unique(time.Second*60))
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
		//
		_taskId, dbErr := database.CreateOrUpdateTask(enquedTask.ID, shorturl, user.UUID.String(), linkInfo.EntityID)
		if dbErr != nil {
			log.Printf("[controller][conversion][EchoConversion] - error creating task: %v", dbErr)
			return ctx.Status(http.StatusInternalServerError).JSON("error creating task")
		}
		orchdioQueue := queue.NewOrchdioQueue(p.AsynqClient, p.DB, p.Redis)

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
				_, err := inspector.GetTaskInfo(queue.PlaylistConversionQueue, enquedTask.ID)
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
				log.Printf("[controller][conversion][EchoConversion] - dumped task info")

				// get the task from the db
				//taskRecord, err := database.FetchTask(string(_taskId))
				if err != nil {
					log.Printf("[controller][conversion][EchoConversion][error] - could not fetch task record from DB. Fatal error%v", err)
					return err
				}

				// update the task to success. because this seems to be a race condition in production where
				// it duplicates task scheduling even though the task is already queued
				// update task to success
				err = database.UpdateTaskStatus(enquedTask.ID, "pending")
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
				//
				//log.Printf("Dumped task info")
				//spew.Dump(queueInfo)

				log.Printf("[controller][conversion][EchoConversion] - task updated to success")
				if r.(string) == "asynq: multiple registrations for playlist:conversion" {
					log.Printf("[controller][conversion][EchoConversion] - task already queued")
				}
			}
			log.Printf("[controller][conversion][EchoConversion] - recovered from panic.. task already queued")

			return util.SuccessResponse(ctx, http.StatusOK, res)
		}()
		p.AsynqMux.HandleFunc("playlist:conversion:", orchdioQueue.PlaylistTaskHandler)
		log.Printf("[controller][conversion][EchoConversion] - task handler attached")
		return util.SuccessResponse(ctx, http.StatusCreated, res)
	}

	log.Printf("\n[controllers][platforms][ConvertEntity] error - %v\n", "It is not a playlist or track URL")
	return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid URL")
}

// ConvertPlaylist retrieves info about a playlist from various platforms.
func (p *Platforms) ConvertPlaylist(ctx *fiber.Ctx) error {
	// first, we want to fetch the information on the link

	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)

	// make sure we're actually handling for track alone, not playlist.
	if strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("\n[controllers][platforms][ConvertEntity] error - %v\n", "It is a playlist URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Not a playlist entity")
	}

	convertedPlaylist, err := universal.ConvertPlaylist(linkInfo, p.Redis)

	if err != nil {
		if err == blueprint.ENOTIMPLEMENTED {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "not supported", "Not implemented")
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][error] could not convert playlist %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred")
	}
	return util.SuccessResponse(ctx, http.StatusOK, convertedPlaylist)
}

// AddPlaylistToAccount adds a playlist to a user's account
func (p *Platforms) AddPlaylistToAccount(ctx *fiber.Ctx) error {
	// get the platform they want to add the playlist to
	platform := ctx.Params("platform")
	if platform == "" {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No platform in context")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Platform not found")
	}

	//// get the playlist ID
	//playlistID := ctx.Params("playlistId")
	//if playlistID == "" {
	//	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No playlist ID in context")
	//	return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request ,"Playlist ID not found")
	//}

	// get the playlist creation body
	var createBodyData = struct {
		User   string   `json:"user"`
		Title  string   `json:"title"`
		Tracks []string `json:"tracks"`
	}{}

	err := ctx.BodyParser(&createBodyData)
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body. Please make sure the body is valid.")
	}

	if len(createBodyData.Tracks) == 0 {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No tracks in playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No tracks to insert into playlist. please add tracks to the playlist")
	}

	if createBodyData.Title == "" {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No title in playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "No title to insert into playlist. please add title to the playlist")
	}

	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] incoming body - %v\n", createBodyData)

	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - got user %v\n", createBodyData.User)

	// find the user in the database
	database := db.NewDB{DB: p.DB}
	user, err := database.FindUserByEmail(createBodyData.User, platform)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "User not found")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User not found")
		}
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
	}

	// get the user's access token
	t, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error decrypting user refresh token - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred while decrypting refresh token")
	}

	//title := fmt.Sprintf("Zoove playlist: %s", createBodyData.Title)
	description := "powered by Orchdio. https://orchdio.com"
	playlistlink := ""
	switch platform {
	case "spotify":
		httpClient := spotifyauth.New().Client(context.Background(), &oauth2.Token{
			RefreshToken: string(t),
		})

		client := spotify.New(httpClient)
		createdPlaylist, err := client.CreatePlaylistForUser(context.Background(), user.PlatformID, createBodyData.Title, description, true, false)

		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error getting profile - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Internal server error")
		}

		var trackIds []spotify.ID
		for _, track := range createBodyData.Tracks {
			if track != "" {
				trackIds = append(trackIds, spotify.ID(track))
			}
		}

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - track ids %v\n", len(trackIds))
		// update playlist with the tracks
		updated, err := client.AddTracksToPlaylist(context.Background(), createdPlaylist.ID, trackIds...)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error getting profile - %v\n", err)
			if err.Error() == "No tracks specified." {
				return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", err.Error())
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err.Error(), "An internal error occurred")
		}

		playlistlink = createdPlaylist.ExternalURLs["spotify"]

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", updated)

	case "deezer":
		id, err := deezer.CreateNewPlaylist(createBodyData.Title, user.PlatformID, string(t), createBodyData.Tracks)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create a new playlist for user")
		}

		playlistlink = fmt.Sprintf("https://www.deezer.com/en/playlist/%s", id)

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", createBodyData.Title)
	// get the user, to see if our token is valid

	case "applemusic":
		pl, err := applemusic.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount][error] - an error occurred while adding playlist to user platform account - %v\n", err)
			if err == blueprint.EFORBIDDEN {
				log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An internal error occurred.")
		}

	case "tidal":
		pl, err := tidal.CreateNewPlaylist(createBodyData.Title, description, string(t), createBodyData.Tracks)
		playlistlink = string(pl)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount][error] - an error occurred while adding playlist to user platform account - %v\n", err)
			if err == blueprint.EFORBIDDEN {
				log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
				return util.ErrorResponse(ctx, http.StatusForbidden, err, "Could not create new playlist for user. Access has not been granted by user")
			}
		}
	}
	return util.SuccessResponse(ctx, http.StatusCreated, playlistlink)

}
