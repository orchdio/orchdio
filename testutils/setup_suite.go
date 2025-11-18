package testutils

import (
	"context"
	"errors"
	"log"
	"orchdio/blueprint"
	"orchdio/controllers/account"
	"orchdio/controllers/developer"
	"orchdio/controllers/platforms"
	"orchdio/db"
	"orchdio/middleware"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
)

type TestSuite struct {
	App    *fiber.App
	DB     *sqlx.DB
	DevApp *blueprint.DeveloperApp
}

func SetupTestSuite() *TestSuite {

	// todo: use test env variables... from file, perhaps.
	// set the env variable
	os.Setenv("JWT_SECRET", "some-secret")
	os.Setenv("ENCRYPTION_SECRET", "super-secure-secret-something-ff")
	os.Setenv("SVIX_API_KEY", "some-keys-here")
	os.Setenv("SVIX_API_URL", "https://api.eu.svix.com")
	os.Setenv("DEEZER_API_BASE", "https://api.deezer.com")

	dbURL := "postgres://kauffman@localhost:5432/orchdio_test?sslmode=disable"
	dbase, _ := db.ConnectDB(dbURL)

	drver, dErr := postgres.WithInstance(dbase.DB, &postgres.Config{})
	if dErr != nil {
		log.Print("Error instantiating the driver")
		panic(dErr)
	}

	dbDriver, dbErr := migrate.NewWithDatabaseInstance("file://../db/migration", "postgres", drver)
	if dbErr != nil {
		log.Println("Error running migrations....")
		panic(dbErr)
	}

	dbErr = dbDriver.Up()
	if dbErr != nil && !errors.Is(dbErr, migrate.ErrNoChange) {
		log.Println("Error migrating database")
		panic(dbErr)
	}

	// configure redis. following configuration is for the queue system setup, using redis.
	redisOpts, err := redis.ParseURL("redis://localhost:6379")
	if err != nil {
		log.Printf("⛔ Error parsing redis url")
	}

	redisClient := redis.NewClient(redisOpts)
	if redisClient.Ping(context.Background()).Err() != nil {
		log.Printf("COULD NOT CONNECT TO REDIST INSTANCE FOR TESTING...")
		panic("Could not connect to redis. Please check your redis configuration.")
	}

	mockQueue := &MockQueue{}
	svixInstance := &MockSvix{}

	app := fiber.New()
	authMiddleware := middleware.NewAuthMiddleware(dbase)
	platformsHandler := platforms.NewPlatform(redisClient, dbase, mockQueue, svixInstance)
	userController := account.NewUserController(dbase, redisClient, mockQueue)

	devAppHandler := developer.NewDeveloperController(dbase, svixInstance)

	// get JWT secret from environment or use test default
	// todo: remove this when test .env is figured out
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "test-secret-key-for-testing-only"
		log.Printf("⚠️  Using default test JWT secret")
	}

	app.Post("/v1/org/new", userController.CreateOrg)
	app.Post("/v1/track/convert", authMiddleware.AddReadOnlyDeveloperToContext, middleware.ExtractLinkInfoFromBody, platformsHandler.ConvertTrack)

	orgRouter := app.Group("/v1/org")
	orgRouter.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(jwtSecret),
		Claims:     &blueprint.AppJWT{},
		ContextKey: "appToken",
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			log.Printf("Test JWT validation error: %v", err)
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		},
	}), middleware.VerifyAppJWT)

	orgRouter.Post("/:orgId/app/new", devAppHandler.CreateApp)
	appRouter := app.Group("/v1/app")

	appRouter.Use(jwtware.New(jwtware.Config{
		SigningKey: []byte(jwtSecret),
		Claims:     &blueprint.AppJWT{},
		ContextKey: "appToken",
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			log.Printf("Test JWT validation error: %v", err)
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		},
	}), middleware.VerifyAppJWT)

	appRouter.Patch("/:appId", devAppHandler.UpdateApp)

	go func() {
		app.Listen(":4200")
	}()

	p := &TestSuite{}
	p.App = app
	p.DB = dbase

	return p
}
