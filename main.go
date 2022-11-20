//go:generate swagger generate spec

// TODO: UPDATE DOCS to reflect that revoke (and similar endpoints that dont return a data) will not have the data field in the response
package main

import (
	"context"
	"fmt"
	"github.com/antoniodipinto/ikisocket"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/gofiber/template/html"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	"github.com/vmihailenco/taskq/v3"
	"github.com/vmihailenco/taskq/v3/redisq"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/controllers"
	"orchdio/controllers/account"
	"orchdio/controllers/conversion"
	"orchdio/controllers/follow"
	"orchdio/controllers/platforms"
	"orchdio/controllers/webhook"
	"orchdio/middleware"
	"orchdio/universal"
	"os"
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

type SysInfo struct {
	Hostname string `bson:"hostname"`
	Platform string `bson:"platform"`
	CPU      string `bson:"cpu"`
	RAM      string `bson:"ram"`
	Disk     string `bson:"disk"`
}

func getInfo(ctx *fiber.Ctx) error {

	hostStat, _ := host.Info()
	cpuStat, _ := cpu.Info()
	vmStatus, _ := mem.VirtualMemory()
	diskStat, _ := disk.Usage("//")

	cpuName := ""
	if len(cpuStat) > 0 {
		cpuName = cpuStat[0].Family
	}

	info := SysInfo{
		Hostname: hostStat.Hostname,
		CPU:      cpuName,
		Platform: hostStat.Platform,
		Disk:     fmt.Sprintf("%dGB", diskStat.Total/1024/1024/1024),
		RAM:      fmt.Sprintf("%dGB", vmStatus.Total/1024/1024/1024),
	}

	response := map[string]interface{}{
		"processor": cpuName,
		"hostname":  info.Hostname,
		"ram":       info.RAM,
		"disk":      info.Disk,
		"platform":  info.Platform,
	}

	return ctx.Status(http.StatusOK).JSON(fiber.Map{
		"message": "Request OK",
		"status":  http.StatusOK,
		"data":    response,
	})
}
func main() {

	engine := html.New("layouts", ".html")
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	// Database and cache setup things
	envr := os.Getenv("ZOOVE_ENV")
	dbURL := os.Getenv("DATABASE_URL")
	if envr != "production" {
		dbURL = dbURL + "?sslmode=disable"
	}

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
	userController := account.UserController{
		DB: db,
	}

	/**
	 ===========================================================
	+ Redis connections here
	*/

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

	if os.Getenv("ENV") == "production" {
		log.Printf("\n[main] [info] - Running in production mode. Connecting to authenticated redis")

	}

	asyncClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisOpts.Addr, Password: redisOpts.Password})
	asynqServer := asynq.NewServer(asynq.RedisClientOpt{Addr: redisOpts.Addr, Password: redisOpts.Password}, asynq.Config{Concurrency: 10})

	asynqMux := asynq.NewServeMux()
	err = asynqServer.Start(asynqMux)
	if err != nil {
		log.Printf("Error starting asynq server")
		panic(err)
	}

	//asynqMux.HandleFunc("orchdio-playlist-queue", orchdioQueue.PlaylistTaskHandler)

	// ===========================================================
	// this is the job queue config shenanigans
	//conversionContr := conversion.
	// ===========================================================

	userController = *account.NewUserController(db)
	webhookController := account.NewWebhookController(db)
	authMiddleware := middleware.NewAuthMiddleware(db)
	conversionController := conversion.NewConversionController(db, redisClient, playlistQueue, QueueFactory, asyncClient, asynqServer, asynqMux)
	followController := follow.NewController(db, redisClient)
	// ==========================================
	// Migrate

	//log.Printf("Here is the db url %s", dbURL)
	//m, err := migrate.New("file://db/migration", dbURL)

	//if err != nil {
	//	log.Printf("Error firing up migrate %v", err)
	//}

	//log.Printf("Here is the migrate stuff")userController
	//if err := m.Up(); err != nil {
	//	log.Printf("Error migrating :sadface:")
	//	panic(err)
	//}

	platformsControllers := platforms.NewPlatform(redisClient, db)
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

	app.Use(cors.New(), authMiddleware.LogIncomingRequest)
	baseRouter := app.Group("/api/v1")
	//baseRouter.Use(authMiddleware.LogIncomingRequest)

	baseRouter.Get("/heartbeat", getInfo)
	baseRouter.Get("/:platform/connect", userController.RedirectAuth)
	baseRouter.Get("/spotify/auth", userController.AuthSpotifyUser)
	baseRouter.Get("/deezer/auth", userController.AuthDeezerUser)
	//baseRouter.Post("/applemusic/auth", userController.AuthAppleMusicUser)
	baseRouter.Post("/applemusic/auth", userController.AuthAppleMusicUser)
	baseRouter.Get("/track/convert", authMiddleware.ValidateKey, authMiddleware.AddAPIDeveloperToContext, middleware.ExtractLinkInfo, platformsControllers.ConvertTrack)
	//baseRouter.Get("/track/convert", middleware.ExtractLinkInfo, platformsControllers.ConvertTrack)
	baseRouter.Get("/playlist/convert", authMiddleware.ValidateKey, middleware.ExtractLinkInfo, platformsControllers.ConvertPlaylist)

	baseRouter.Post("/white-tiger", authMiddleware.AddAPIDeveloperToContext, whController.Handle)
	app.Get("/white-tiger", whController.AuthenticateWebhook)
	baseRouter.Post("/webhook/add", authMiddleware.ValidateKey, webhookController.CreateWebhookUrl)
	baseRouter.Patch("/webhook/update", authMiddleware.ValidateKey, webhookController.UpdateUserWebhookUrl)
	baseRouter.Get("/webhook", authMiddleware.ValidateKey, webhookController.FetchWebhookUrl)
	baseRouter.Delete("/webhook", authMiddleware.ValidateKey, webhookController.DeleteUserWebhookUrl)

	userRouter := app.Group("/api/v1/user")

	userRouter.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(os.Getenv("JWT_SECRET")),
		Claims:     &blueprint.OrchdioUserToken{},
		ContextKey: "authToken",
	}), middleware.VerifyToken)

	userRouter.Post("/generate-key", userController.GenerateAPIKey)
	userRouter.Patch("/key/revoke", authMiddleware.ValidateKey, userController.RevokeKey)
	userRouter.Patch("/key/allow", userController.UnRevokeKey)
	userRouter.Delete("/key/delete", middleware.VerifyToken, authMiddleware.ValidateKey, userController.DeleteKey)
	userRouter.Get("/key", userController.RetrieveKey)

	// ==========================================
	// NEXT ROUTES
	nextRouter := baseRouter.Group("/next", authMiddleware.ValidateKey)

	nextRouter.Post("/playlist/convert", middleware.ExtractLinkInfoFromBody, conversionController.ConvertPlaylist)
	nextRouter.Get("/task/:taskId", conversionController.GetPlaylistTask)
	nextRouter.Delete("/task/:taskId", conversionController.DeletePlaylistTask)

	// user account action routes
	userActionAddRouter := nextRouter.Group("/add")
	userActionAddRouter.Post("/playlist/:platform/:playlistId", platformsControllers.AddPlaylistToAccount)

	// FIXME: remove later. this is just for compatibility with the ping api for dev.
	nextRouter.Post("/job/ping", conversionController.ConvertPlaylist)
	nextRouter.Post("/follow", followController.FollowPlaylist)
	nextRouter.Post("/playlist/:platform/add", platformsControllers.AddPlaylistToAccount)
	nextRouter.Post("/waitlist/add", userController.AddToWaitlist)

	// MIDDLEWARE DEFINITION
	//app.Use(jwtware.New(jwtware.Config{
	//	SigningKey: []byte(os.Getenv("JWT_SECRET")),
	//	Claims:     &blueprint.OrchdioUserToken{},
	//	ContextKey: "authToken",
	//}))
	//app.Use(middleware.VerifyToken)

	baseRouter.Get("/me", userController.FetchProfile)
	// FIXME: move this endpoint thats fetching link info from the `controllers` package
	baseRouter.Get("/info", middleware.ExtractLinkInfo, controllers.LinkInfo)

	baseRouter.Get("/key", userController.RetrieveKey)

	// now to the WS endpoint to connect to when they visit the website and want to "convert"
	app.Get("/portal", ikisocket.New(func(kws *ikisocket.Websocket) {
		log.Printf("\nClient with ID %v connected\n", kws.UUID)
	}))

	//app.Use(func(c *fiber.Ctx) error {
	//	if websocket.IsWebSocketUpgrade(c) {
	//		c.Locals("allowed", true)
	//		return c.Next()
	//	}
	//	return fiber.ErrUpgradeRequired
	//})

	/**
	this is a test to see hpw the keyboard light patterns    are.
	*/

	// WEBSOCKET EVENT HANDLERS
	ikisocket.On(ikisocket.EventConnect, func(payload *ikisocket.EventPayload) {
		log.Printf("\n[main][SocketEvent][EventConnect] - A new client connected\n")
	})

	ikisocket.On(ikisocket.EventDisconnect, func(payload *ikisocket.EventPayload) {
		// TODO: incrementally retry to reconnect with the client
		log.Printf("\nClient has disconnected")
	})

	ikisocket.On(ikisocket.EventMessage, universal.TrackConversion)
	ikisocket.On(ikisocket.EventMessage, func(payload *ikisocket.EventPayload) {
		universal.PlaylistConversion(payload, redisClient)
	})

	/**
	 ==================================================================
	+ some job queue shenanigans here
	*/
	// consume all the jobs in the queue
	//if err :=

	/**
	 ==================================================================
	+
	+
	+	SERVER PORT CONFIGURATIONS AND SERVER STARTING THINGS HERE
	+
	+
	 ==================================================================
	*/
	port := os.Getenv("PORT")
	log.Printf("Port: %v", port)
	if port == " " {
		port = "52800"
	}

	port = fmt.Sprintf(":%s", port)

	//if aErr := asynqServer.Run(asynqMux); aErr != nil {
	//	log.Printf("\n[main] [error] - Could not start asynq server. Are you sure redis is configured correctly? Also, something else might be wrong")
	//	panic(aErr)
	//}

	//defer func(s *asynq.Server) {
	//	s.Stop()
	//}(asynqServer)

	//go func(DB *sqlx.DB) {
	//	c := make(chan int)
	//	log.Printf("\n[main] [info] - Process background tasks")
	//	<-c
	//	ProcessFollows(c, DB)
	//}(db)

	// hERE WE WANT TO SETUP A CRONJOB THAT RUNS EVERY 2 MINS TO PROCESS THE FOLLOWS
	c := cron.New()

	entryId, cErr := c.AddFunc("@every 1m", func() {
		log.Printf("\n[main] [info] - Process background tasks")
		follow.SyncFollowsHandler(db, redisClient, asyncClient, asynqMux)
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

}

//
//func ProcessFollows(c chan int, redisClient *redis.Client, DB *sqlx.DB, aClient *asynq.Client, aMux *asynq.ServeMux) {
//	for {
//		select {
//		case <-c:
//			follow.SyncFollowsHandler(DB, redisClient, aClient, aMux)
//			log.Printf("\n[main] [info] - Follows processed")
//		}
//	}
//}
