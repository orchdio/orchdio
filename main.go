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
	_ "github.com/gofiber/fiber/v2"
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
	logger2 "orchdio/logger"
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
						log.Printf("[main] [QueueErrorHandler] Going to retry the handler needed to be run")
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
						log.Printf("[main] [QueueErrorHandler] Going to retry the handler needed to be run")
						playlistQueue := queue.NewOrchdioQueue(asyncClient, db, redisClient, asynqMux)
						// schedule the playlist conversion
						var playlistData blueprint.PlaylistTaskData
						err = json.Unmarshal(task.Payload(), &playlistData)
						if err != nil {
							log.Printf("[main][QueueErrorHandler] error - could not unmarshal playlist task data %v", err)
							return
						}
						err = playlistQueue.PlaylistHandler(playlistData.TaskID, playlistData.ShortURL, playlistData.LinkInfo, playlistData.App.UID.String())
						if err != nil {
							log.Printf("[main][QueueErrorHandler] error - could not retry playlist conversion.. %v", err)
							return
						}
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
	orchdioQueue := queue.NewOrchdioQueue(asyncClient, db, redisClient, asynqMux)
	asynqMux.HandleFunc("send:appauth:email", orchdioQueue.SendEmailHandler)
	asynqMux.HandleFunc("playlist:conversion", orchdioQueue.PlaylistTaskHandler)
	asynqMux.HandleFunc("send:reset_password_email", orchdioQueue.SendEmailHandler)
	asynqMux.HandleFunc("send:welcome_email", orchdioQueue.SendEmailHandler)

	err = asynqServer.Start(asynqMux)
	if err != nil {
		log.Printf("Error starting asynq server")
		panic(err)
	}

	/// Go fiber server configuration
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
			// todo: check the type of error it is. because for example in the method to add playlist to user's account
			// if we couldnt fetch the userdata, we return an error. seems to kill the serve
			//r
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

	// for each queue, check if it is paused, if it is, unpause it
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

	userController := account.NewUserController(db, redisClient, asyncClient, asynqMux)
	authMiddleware := middleware.NewAuthMiddleware(db)
	conversionController := conversion.NewConversionController(db, redisClient, playlistQueue, QueueFactory, asyncClient, asynqServer, asynqMux)
	// create logger
	orchdioLogger := logger2.NewZapSentryLogger()
	devAppController := developer.NewDeveloperController(db, orchdioLogger)

	platformsControllers := platforms.NewPlatform(redisClient, db, asyncClient, asynqMux)
	whController := webhook.NewWebhookController(db, redisClient)

	// Migrate
	//dbDriver, err := migrate.NewWithDatabaseInstance("file://./db/migrations", "postgres", db)
	/**
	 ==================================================================
	+
	+
	+	ROUTE DEFINITIONS GO HERE
	+
	+
	 ==================================================================
	*/

	app.Use(cors.New(cors.Config{
		AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH",
	}), authMiddleware.LogIncomingRequest, authMiddleware.HandleTrolls)
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
	orchRouter.Get("/account/:userId/:platform/albums", authMiddleware.AddRequestPlatformToCtx, authMiddleware.AddReadWriteDeveloperToContext, authMiddleware.VerifyUserActionApp, platformsControllers.FetchPlatformAlbums)
	// TODO: implement for tidal
	orchRouter.Get("/account/:userId/:platform/history/tracks", authMiddleware.AddRequestPlatformToCtx, authMiddleware.AddReadWriteDeveloperToContext, authMiddleware.VerifyUserActionApp, platformsControllers.FetchTrackListeningHistory)

	orchRouter.Post("/follow", authMiddleware.AddReadWriteDeveloperToContext, userController.FollowPlaylist)
	orchRouter.Post("/waitlist/add", authMiddleware.AddReadWriteDeveloperToContext, userController.AddToWaitlist)

	orgRouter := app.Group("/v1/org")
	orgRouter.Post("/new", userController.CreateOrg)
	orgRouter.Post("/login", userController.LoginUserToOrg)
	orgRouter.Post("/reset-password", userController.ResetPassword)
	orgRouter.Get("/reset-password", userController.ResetPassword)
	orgRouter.Post("/change-password", userController.ChangePassword)

	orgRouter.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(os.Getenv("JWT_SECRET")),
		Claims:     &blueprint.AppJWT{},
		ContextKey: "appToken",
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			log.Printf("Error validating auth token %v:\n", err)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "Authorization error", "Invalid or Expired token")
		},
	}), middleware.VerifyAppJWT)
	orgRouter.Post("/:orgId/app/new", devAppController.CreateApp)
	orgRouter.Get("/:orgId/apps", devAppController.FetchAllDeveloperApps)

	orgRouter.Delete("/:orgId", userController.DeleteOrg)
	orgRouter.Patch("/:orgId", userController.UpdateOrg)
	orgRouter.Get("/all", userController.FetchUserOrgs)

	// apps endpoints are mostly for the developers, accessible by an api endpoint
	// they are a little different from the org endpoints, even though orgs call app.
	// this is for the internal orchdio dev dashboard/apps. therefore some endpoints
	// are essentially available to orgs and also developers (using their api keys)
	// note: perhpas this could be resolved to avoid confusion. Sync with @marvin
	appRouter := app.Group("/v1/app")
	appRouter.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(os.Getenv("JWT_SECRET")),
		Claims:     &blueprint.AppJWT{},
		ContextKey: "appToken",
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			log.Printf("Error validating auth token %v:\n", err)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "Authorization error", "Invalid or Expired token")
		},
	}), middleware.VerifyAppJWT)
	orchRouter.Get("/app/:appId", devAppController.FetchApp)

	appRouter.Get("/me", userController.FetchProfile)
	// developer endpoints
	//app.Get("/v1/:orgId/apps/all", jwtware.New(jwtware.Config{
	//	SigningKey: []byte(os.Getenv("JWT_SECRET")),
	//	Claims:     &blueprint.AppJWT{},
	//	ContextKey: "appToken",
	//	ErrorHandler: func(ctx *fiber.Ctx, err error) error {
	//		log.Printf("Error validating auth token: %v\n", err)
	//		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Authorization error", "Invalid or Expired token")
	//	},
	//}), middleware.VerifyAppJWT, devAppController.FetchAllDeveloperApps)

	appRouter.Get("/:appId/keys", devAppController.FetchKeys)
	//appRouter.Get("/:appId", devAppController.FetchApp)

	baseRouter.Post("/v1/:orgId/app/disable", devAppController.DisableApp)
	baseRouter.Post("/v1/:orgId/app/enable", devAppController.EnableApp)
	appRouter.Delete("/:orgId/app/delete", devAppController.DeleteApp)
	appRouter.Patch("/:appId", devAppController.UpdateApp)
	appRouter.Delete("/:appId/credentials/:platform", devAppController.DeletePlatformIntegrationCredentials)

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
