//go:generate swagger generate spec

// TODO: UPDATE DOCS to reflect that revoke (and similar endpoints that dont return a data) will not have the data field in the response
package main

import (
	context "context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/antoniodipinto/ikisocket"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/etag"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	jwtware "github.com/gofiber/jwt/v3"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/vmihailenco/taskq/v3"
	"github.com/vmihailenco/taskq/v3/redisq"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/controllers"
	"orchdio/controllers/account"
	"orchdio/controllers/auth"
	"orchdio/controllers/conversion"
	"orchdio/controllers/developer"
	"orchdio/controllers/platforms"
	"orchdio/controllers/webhook"
	"orchdio/middleware"
	"orchdio/queue"
	follow2 "orchdio/services/follow"
	"orchdio/util"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func init() {
	env := os.Getenv("ENV")
	if env == "" {
		log.Println("==⚠️ WARNING: env variable not set. Using dev ⚠️==")
		env = "dev"
	}
	err := godotenv.Load(".env." + env)
	if err != nil {
		log.Println("Error reading the env file")
		log.Println(err)
	}
}

/**
   ===========================================================
  + Redis connections here
*/

func main() {
	// Database and cache setup things
	envr := os.Getenv("ORCHDIO_ENV")
	dbURL := os.Getenv("DATABASE_URL")
	if envr != "production" {
		dbURL = dbURL + "?sslmode=disable"
	}

	port := os.Getenv("PORT")
	log.Printf("Port: %v", port)
	if port == " " {
		port = "52800"
	}

	port = fmt.Sprintf(":%s", port)

	db, err := sqlx.Open("postgres", dbURL)
	if err != nil {
		log.Printf("Error connecting to postgresql db")
		panic(err)
	}
	defer func(db *sqlx.DB) {
		err := db.Close()
		if err != nil {
			log.Printf("Error closing")
		}
	}(db)
	err = db.Ping()
	if err != nil {
		log.Println("Error connecting to postgresql db")
		panic(err)
	}

	log.Println("Connected to Postgresql database")

	redisOpts, err := redis.ParseURL(os.Getenv("REDISCLOUD_URL"))
	if err != nil {
		log.Printf("Error parsing redis url")
		panic(err)
	}

	redisClient := redis.NewClient(redisOpts)
	if redisClient.Ping(context.Background()).Err() != nil {
		log.Printf("\n[main] [error] - Could not connect to redis. Are you sure redis is configured correctly?")
		panic("Could not connect to redis. Please check your redis configuration.")
	}

	var QueueFactory = redisq.NewFactory()

	var playlistQueue = QueueFactory.RegisterQueue(&taskq.QueueOptions{
		Name:  "orchdio-playlist-queue",
		Redis: redisClient,
	})
	asynqMux := asynq.NewServeMux()

	if os.Getenv("ENV") == "production" {
		log.Printf("\n[main] [info] - Running in production mode. Connecting to authenticated redis")
	}

	// ===========================================================
	// this is the job queue config shenanigans
	// ===========================================================
	asyncClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisOpts.Addr, Password: redisOpts.Password})
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisOpts.Addr, Password: redisOpts.Password})

	asynqServer := asynq.NewServer(asynq.RedisClientOpt{Addr: redisOpts.Addr, Password: redisOpts.Password},
		asynq.Config{Concurrency: 10,
			ShutdownTimeout: 3 * time.Second,
			Queues: map[string]int{
				"playlist-conversion": 5,
				"email":               2,
				"default":             1,
			},
			// NB: from the queue CheckForOrphanedTasksMiddleware, when we handle orphaned task and we return a blueprint.ENORESULT error, the execution
			// jumps here, so when the middleware runs and we return a blueprint.ENORESULT error, it'll run this block and reprocess the task
			// if the handler has successfully been attached or do nothing (and let the queue retry later) if there was an error
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Printf("[main][QueueErrorHandler] Running queue server error handler...")
				// handle for each task here

				// check if the task is an email task
				isEmailQueue := util.IsTaskType(task.Type(), "send:appauth")
				if isEmailQueue {
					queueInfo, qErr := inspector.GetQueueInfo(queue.EmailQueue)
					if qErr != nil {
						log.Printf("[main] [QueueErrorHandler] Error getting queue info %v", qErr)
						return
					}
					if queueInfo.Paused {
						log.Printf("[main] [QueueErrorHandler] Email queue is paused.. Unpausing")
						err = inspector.UnpauseQueue(queue.EmailQueue)
						return
					}
					notFound := asynq.NotFound(context.Background(), task)

					if notFound != nil {
						log.Printf("[main] [QueueErrorHandler] Task not found in queue. Seems this task is orphaned.. Going to retry the handler needed to be run")
						emailQueue := queue.NewOrchdioQueue(asyncClient, db, redisClient, asynqMux)
						var emailData blueprint.EmailTaskData
						err = json.Unmarshal(task.Payload(), &emailData)
						if err != nil {
							log.Printf("[main][QueueErrorHandler] error - could not unmarshal email task data %v", err)
							return
						}
						// schedule the email
						err = emailQueue.SendEmail(&emailData)
						if err != nil {
							log.Printf("[main][QueueErrorHandler] error - could not schedule email %v", err)
							return
						}
						log.Printf("[main][QueueErrorHandler] info - successfully scheduled email")
						return
					}
					// the task is found...
					log.Printf("[main] [QueueErrorHandler] Task found in queue. Seems this task is not orphaned. Doing nothing")
					return
				}

				// conversion queue
				isConversionQueue := util.IsTaskType(task.Type(), "playlist:conversion")
				if isConversionQueue {
					log.Printf("[main][QueueErrorHandler][info] This middleware is called when the queue server encounters an error and rescheduling the queues")
					// check that the queue isnt paused
					queueInfo, qErr := inspector.GetQueueInfo(queue.PlaylistConversionQueue)
					if qErr != nil {
						log.Printf("[main] [QueueErrorHandler] Error getting queue info %v", qErr)
						return
					}

					var taskData blueprint.PlaylistTaskData
					err = json.Unmarshal(task.Payload(), &taskData)
					if err != nil {
						log.Printf("[main] [QueueErrorHandler] Error unmarshalling task payload %v", err)
						return
					}
					log.Printf("[main] [QueueErrorHandler] Queue info %v", queueInfo)
					if queueInfo.Paused {
						log.Printf("[main][QueueErrorHandler] Queue is paused")
						err = inspector.UnpauseQueue(queue.PlaylistConversionQueue)
						return
					}

					// check if task has already been scheduled (has an handler), by fetching task from queue
					notFound := asynq.NotFound(context.Background(), task)
					if notFound != nil {
						log.Printf("[main] [QueueErrorHandler][warning] Task not scheduled")
						asynqMux.Handle(task.Type(), asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
							log.Printf("[main] [QueueErrorHandler] Custom running handler here%v", err)
							// create new taskHandler that will then process this job. From the ```queue.LoggerMiddleware``` method,
							// we check for "orphaned" tasks, these tasks are tasks that were created but were never fully processed,
							// for example during server restart or shutdown during task processing, even though we detect queue pauses and pause them
							// when we do this, the next task the queue server picks up the task, we'll attach this handler to it and just process it.
							taskHandler := queue.NewOrchdioQueue(asyncClient, db, redisClient, asynqMux)
							err = taskHandler.PlaylistHandler(t.ResultWriter().TaskID(), taskData.ShortURL, taskData.LinkInfo, taskData.App.UID.String())
							if err != nil {
								log.Printf("[main] [QueueErrorHandler] Error processing task %v", err)
								return err
							}
							log.Printf("[main] [QueueErrorHandler] Task processed successfully")
							return nil
						}))
						log.Printf("[main] [QueueErrorHandler][warning] Queue is not paused but an error occured")
						return
					}

					taskHandler := queue.NewOrchdioQueue(asyncClient, db, redisClient, asynqMux)
					err = taskHandler.PlaylistHandler(task.ResultWriter().TaskID(), taskData.ShortURL, taskData.LinkInfo, taskData.App.UID.String())
					if err != nil {
						log.Printf("[main] [QueueErrorHandler] Error processing task %v", err)
						return
					}

					log.Printf("[main] [QueueErrorHandler] Task already has a handler")
				}
			}),
		})

	asynqMux.Use(queue.CheckForOrphanedTasksMiddleware)
	emailQueue := queue.NewOrchdioQueue(asyncClient, db, redisClient, asynqMux)
	asynqMux.HandleFunc("send:appauth:email", emailQueue.SendEmailHandler)

	err = asynqServer.Start(asynqMux)
	if err != nil {
		log.Printf("Error starting asynq server")
		panic(err)
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
		AppName:               os.Getenv("APP_NAME"),
		DisableDefaultDate:    true,
		ReadTimeout:           45 * time.Second,
		WriteTimeout:          45 * time.Second,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			var e *fiber.Error
			if errors.As(err, &e) {
				// e.Code will be the status code.
				// e.Message will be the error message.
				log.Printf(err.Error())
				return util.ErrorResponse(ctx, e.Code, "internal error", e.Message)
			}
			log.Printf("Error in next router %v", err)
			// get the PID of the asynq server and send it a kill signal to OS
			// this is a hacky way to kill the asynq server
			queueServer, err := inspector.Servers()
			if err != nil {
				log.Printf("Error getting queue server %v", err)
				return err
			}

			// make sure we have a queue server
			if len(queueServer) == 0 {
				log.Printf("No queue server found")
				return nil
			}

			// on railway, the server is always the first one
			v := queueServer[0].PID
			p, err := os.FindProcess(v)
			if err != nil {
				log.Printf("Error finding process %v", err)
				return err
			}

			// send task creation signal cancellation to the queue server
			err = p.Signal(syscall.SIGINT)
			if err != nil {
				log.Printf("Error stopping new tasks%v", err)
				return err
			}

			// shutdown the queue server itself.
			err = p.Signal(syscall.SIGKILL)
			if err != nil {
				log.Printf("Error stopping queue server %v", err)
				return err
			}
			return nil
		},
	})
	serverChan := make(chan os.Signal, 1)
	// we listen for SIGINT, SIGTERM and SIGKILL. this sends a signal to the serverChan channel, which we listen to in the goroutine below
	signal.Notify(serverChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	// unpause the queue server
	// get status of the playlist conversion queue
	// if it is paused, unpause it
	queues, err := inspector.Queues()
	if err != nil {
		log.Printf("[main][Queue] Error getting queues %v", err)
		_ = app.Shutdown()
		return
	}

	if len(queues) > 0 {
		for _, q := range queues {
			queueInfo, err := inspector.GetQueueInfo(q)
			if err != nil {
				log.Printf("[main][Queue] Error getting queue info %v", err)
				_ = app.Shutdown()
				return
			}
			if queueInfo.Paused {
				log.Printf("[main][Queue] Queue %s is paused. Unpausing..", q)
				err = inspector.UnpauseQueue(q)
				if err != nil {
					log.Printf("[main][Queue] Error unpausing queue %v", err)
					_ = app.Shutdown()
					return
				}
			}
		}
	}

	serverShutdown := make(chan struct{})

	// handles the shutdown of the server.
	go func() {
		_ = <-serverChan
		log.Printf("[main] [info] - Shutting down server")
		// inspector
		// get all active tasks
		err := inspector.PauseQueue(queue.PlaylistConversionQueue)
		if err != nil {
			log.Printf("Error pausing queue %v", err)
			_ = app.Shutdown()
			serverShutdown <- struct{}{}
			return
		}
		log.Printf("[main] [info] - Paused queue...")
		_ = app.Shutdown()
		serverShutdown <- struct{}{}
		log.Printf("[main] [info] - Queue pause successful. Server shutdown complete")
	}()

	userController := account.NewUserController(db, redisClient)
	//webhookController := account.NewAccountWebhookController(db)
	authMiddleware := middleware.NewAuthMiddleware(db)
	conversionController := conversion.NewConversionController(db, redisClient, playlistQueue, QueueFactory, asyncClient, asynqServer, asynqMux)
	devAppController := developer.NewDeveloperController(db)

	platformsControllers := platforms.NewPlatform(redisClient, db, asyncClient, asynqMux)
	whController := webhook.NewWebhookController(db, redisClient)

	/**
	 ==================================================================
	+
	+
	+	ROUTE DEFINITIONS GO HERE
	+
	+
	 ==================================================================
	*/

	app.Use(cors.New(), authMiddleware.LogIncomingRequest, authMiddleware.HandleTrolls)
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))
	app.Use(etag.New())
	app.Use(limiter.New(limiter.Config{
		Max:               100,
		Expiration:        30 * time.Second,
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached: func(ctx *fiber.Ctx) error {
			log.Printf("[main] [info] - Rate limit exceeded")
			return util.ErrorResponse(ctx, fiber.StatusTooManyRequests, "rate limit error", "Rate limit exceeded")
		},
	}))
	app.Use(requestid.New(requestid.Config{
		Header:     "x-orchdio-request-id",
		ContextKey: "orchdio-request-id",
	}))
	app.Get("/kanye/info", monitor.New(monitor.Config{Title: "Orchdio-Core health info"}))
	baseRouter := app.Group("/api/v1")
	orchRouter := app.Group("/v1")

	baseRouter.Get("/", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(http.StatusOK)
	})
	app.Get("/vermont/info", monitor.New(monitor.Config{Title: "Orchdio-Core health info"}))

	authController := auth.NewAuthController(db, asyncClient, asynqServer, asynqMux, redisClient)
	// connect endpoints
	orchRouter.Get("/auth/:platform/connect", authMiddleware.AddRequestPlatformToCtx, authController.AppAuthRedirect)
	// the callback that the auth platform will redirect to and this is where we handle the redirect and generate an auth token for the user, as response
	orchRouter.Get("/auth/:platform/callback", authMiddleware.AddRequestPlatformToCtx, authController.HandleAppAuthRedirect)
	// this is for the apple music auth. its a POST as it carries a body
	orchRouter.Post("/auth/:platform/callback", authMiddleware.AddRequestPlatformToCtx, authController.HandleAppAuthRedirect)
	orchRouter.Post("/entity/convert", authMiddleware.AddReadOnlyDeveloperToContext,
		middleware.ExtractLinkInfoFromBody, platformsControllers.ConvertEntity)

	orchRouter.Get("/task/:taskId", authMiddleware.AddReadOnlyDeveloperToContext, conversionController.GetPlaylistTask)
	orchRouter.Post("/playlist/:platform/add", authMiddleware.AddRequestPlatformToCtx, authMiddleware.AddReadWriteDeveloperToContext, platformsControllers.AddPlaylistToAccount)
	// this is the account of the *DEVELOPER* not the user,
	// todo: implement user account fetching using the user id or email, and verify that the developer has access to the user account, by checking
	// 		for user apps that have the developer id
	orchRouter.Get("/account", authMiddleware.AddReadWriteDeveloperToContext, userController.FetchUserInfoByIdentifier)
	orchRouter.Get("/me", authMiddleware.AddReadWriteDeveloperToContext, userController.FetchUserProfile)
	orchRouter.Get("/account/:userId/:platform/playlists", authMiddleware.AddRequestPlatformToCtx, authMiddleware.AddReadWriteDeveloperToContext, platformsControllers.FetchPlatformPlaylists)
	// todo: add nb_artists to data response
	orchRouter.Get("/account/:userId/:platform/artists", authMiddleware.AddRequestPlatformToCtx, authMiddleware.AddReadWriteDeveloperToContext, platformsControllers.FetchPlatformArtists)
	orchRouter.Get("/account/:userId/:platform/albums", authMiddleware.AddRequestPlatformToCtx, authMiddleware.AddReadWriteDeveloperToContext, authMiddleware.VerifyUserActionDeveloper, platformsControllers.FetchPlatformAlbums)
	// TODO: implement for tidal
	orchRouter.Get("/account/:userId/:platform/history/tracks", authMiddleware.AddRequestPlatformToCtx, authMiddleware.AddReadWriteDeveloperToContext, authMiddleware.VerifyUserActionDeveloper, platformsControllers.FetchTrackListeningHistory)

	orchRouter.Post("/follow", authMiddleware.AddReadWriteDeveloperToContext, userController.FollowPlaylist)
	orchRouter.Post("/waitlist/add", authMiddleware.AddReadWriteDeveloperToContext, userController.AddToWaitlist)

	orgRouter := app.Group("/v1/org")
	orgRouter.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(os.Getenv("JWT_SECRET")),
		Claims:     &blueprint.OrchdioUserToken{},
		ContextKey: "authToken",
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			log.Printf("Error validating auth token %v:\n", err)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "Authorization error", "Invalid or Expired token")
		},
	}), middleware.VerifyToken)

	appRouter := app.Group("/v1/app")
	appRouter.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(os.Getenv("JWT_SECRET")),
		Claims:     &blueprint.OrchdioUserToken{},
		ContextKey: "authToken",
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			log.Printf("Error validating auth token %v:\n", err)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "Authorization error", "Invalid or Expired token")
		},
	}), middleware.VerifyToken)

	orchRouter.Get("/app/:appId", devAppController.FetchApp)
	orgRouter.Post("/new", userController.CreateOrg)
	orgRouter.Delete("/:orgId", userController.DeleteOrg)
	orgRouter.Put("/:orgId", userController.UpdateOrg)
	orgRouter.Get("/all", userController.FetchUserOrgs)

	appRouter.Get("/me", userController.FetchProfile)
	// developer endpoints
	appRouter.Get("/:appId/keys", devAppController.FetchKeys)
	//appRouter.Get("/:appId", devAppController.FetchApp)
	app.Get("/v1/apps/all", devAppController.FetchAllDeveloperApps)
	appRouter.Post("/new", devAppController.CreateApp)
	baseRouter.Post("/v1/app/disable", devAppController.DisableApp)
	baseRouter.Post("/v1/app/enable", devAppController.EnableApp)
	appRouter.Delete("/app/delete", devAppController.DeleteApp)
	appRouter.Put("/:appId", devAppController.UpdateApp)
	appRouter.Post("/:appId/keys/revoke", devAppController.RevokeAppKeys)

	//baseRouter.Get("/heartbeat", getInfo)
	orchRouter.Post("/white-tiger", authMiddleware.AddReadWriteDeveloperToContext, whController.Handle)
	orchRouter.Get("/white-tiger", whController.AuthenticateWebhook)

	// ==========================================
	// NEXT ROUTES
	nextRouter := baseRouter.Group("/next", authMiddleware.ValidateKey)

	//nextRouter.Post("/playlist/convert", middleware.ExtractLinkInfoFromBody, conversionController.ConvertPlaylist)
	// TODO: implement checking for superuser access in middleware before deleting then remove kanye prefix
	nextRouter.Delete("/kanye/task/:taskId", conversionController.DeletePlaylistTask)

	// user account action routes
	//userActionAddRouter := nextRouter.Group("/add")
	//userActionAddRouter.Post("/playlist/:platform/:playlistId", platformsControllers.AddPlaylistToAccount)
	// FIXME: remove later. this is just for compatibility with the ping api for dev.
	//nextRouter.Post("/job/ping", conversionController.ConvertPlaylist)

	// FIXME: move this endpoint thats fetching link info from the `controllers` package
	baseRouter.Get("/info", middleware.ExtractLinkInfo, controllers.LinkInfo)

	//baseRouter.Get("/key", userController.RetrieveKey)

	// now to the WS endpoint to connect to when they visit the website and want to "convert"
	app.Get("/portal", ikisocket.New(func(kws *ikisocket.Websocket) {
		log.Printf("\nClient with ID %v connected\n", kws.UUID)
	}))

	/**
	 ==================================================================
	+
	+
	+	SERVER PORT CONFIGURATIONS AND SERVER STARTING THINGS HERE
	+
	+
	 ==================================================================
	*/

	// hERE WE WANT TO SETUP A CRONJOB THAT RUNS EVERY 2 MINS TO PROCESS THE FOLLOWS
	c := cron.New()

	entryId, cErr := c.AddFunc("@every 1m", func() {
		log.Printf("\n[main] [info] - Process background tasks")
		follow2.SyncFollowsHandler(db, redisClient, asyncClient, asynqMux)
	})

	if cErr != nil {
		log.Printf("\n[main] [error] - Could not start cron job.")
		panic(cErr)
	}

	//c.Start()

	log.Printf("\n[main] [info] - CRONJOB Entry ID is: %v", entryId)
	log.Printf("Server is up and running on port: %s", port)
	err = app.Listen(port)

	if err != nil {
		log.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
	<-serverShutdown
	log.Printf("[main] [info] - Cleaning up tasks: %s", port)
}
