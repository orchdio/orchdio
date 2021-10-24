package main

import (
	"database/sql"
	"fmt"
	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	_ "github.com/golang-migrate/migrate/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	"log"
	"net/http"
	"os"
	"zoove/controllers"
	"zoove/controllers/account"
	"zoove/controllers/platforms"
	"zoove/middleware"
	"zoove/types"
)

func init() {
	env := os.Getenv("ENV")
	if env == "" {
		log.Println("==⚠️ WARNING: env variable not set. Using dev==")
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

	app := fiber.New()

	baseRouter := app.Group("/api/v1")

	baseRouter.Get("/heartbeat", getInfo)
	//driver, err := postgres.WithInstance(db, &postgres.Config{})

	//if err != nil {
	//	log.Printf("\n[main][migrate][error] Error with migrate driver 1: %v\n", err)
	//	os.Exit(1)
	//}
	//m, migrateErr := migrate.NewWithDatabaseInstance(
	//	"file://db/migration",
	//	"postgres", driver)
	//
	//if migrateErr != nil {
	//	log.Printf("\n[main][migrate][error] Error with migrate driver: %v\n", err)
	//	os.Exit(1)
	//}
	//
	//migrateErr = m.Up()
	//if migrateErr != nil && migrateErr != migrate.ErrNoChange  {
	//	log.Printf("\n[main][migrate] Error with migration - %v", err)
	//	os.Exit(1)
	//}

	// here is the DB things
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbPass := os.Getenv("DB_PASS")

	psqlInfo := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost,
		dbPort,
		dbUser,
		dbPass,
		dbName,
	)
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Printf("Error connecting to postgresql db")
		panic(err)
	}
	defer func(db *sql.DB) {
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
	/**
	 ==================================================================
	+
	+
	+	ROUTE DEFINITIONS GO HERE
	+
	+
	 ==================================================================
	*/
	userController := account.UserController{
		DB: db,
	}
	baseRouter.Get("/:platform/connect", userController.RedirectAuth)
	baseRouter.Get("/spotify/auth", userController.AuthSpotifyUser)
	baseRouter.Get("/deezer/auth", userController.AuthDeezerUser)

	// MIDDLEWARE DEFINITION
	app.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(os.Getenv("JWT_SECRET")),
		Claims:     &types.ZooveUserToken{},
		ContextKey: "authToken",
	}))
	app.Use(middleware.VerifyToken)
	baseRouter.Get("/me", userController.FetchProfile)
	baseRouter.Get("/info", middleware.ExtractLinkInfo, controllers.LinkInfo)
	baseRouter.Get("/convert", middleware.ExtractLinkInfo, platforms.ConvertTrack)
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

	log.Printf("Server is up and running on port: %s", port)
	err = app.Listen(port)

	if err != nil {
		log.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}

}
