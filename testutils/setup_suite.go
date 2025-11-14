package testutils

import (
	"context"
	"errors"
	"log"
	"orchdio/controllers/account"
	"orchdio/controllers/platforms"
	"orchdio/db"
	"orchdio/middleware"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
)

type TestUserController struct {
	*account.UserController
}

type TestSuite struct {
	App *fiber.App
	DB  *sqlx.DB
	// Pubkey     string
	// PrivateKey string
}

func SetupTestSuite() *TestSuite {
	dbURL := "postgres://kauffman@localhost:5432/orchdio_test?sslmode=disable"
	dbase, _ := db.ConnectDB(dbURL)

	drver, dErr := postgres.WithInstance(dbase.DB, &postgres.Config{})
	if dErr != nil {
		log.Print("Error instantiating the driver")
		panic(dErr)
	}

	dbDriver, dbErr := migrate.NewWithDatabaseInstance("file://../../db/migration", "postgres", drver)
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

	app := fiber.New()
	authMiddleware := middleware.NewAuthMiddleware(dbase)
	mockQueue := &MockQueue{}

	platformsHandler := platforms.NewPlatform(redisClient, dbase, mockQueue)
	userController := &TestUserController{
		UserController: account.NewUserController(dbase, redisClient, mockQueue),
	}
	//
	// userController := account.NewUserController(dbase, redisClient, nil, nil)

	// create a new user

	// dbInst := db.NewDB{
	// 	DB: dbase,
	// }

	// hashedPass, bErr := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	// if bErr != nil {
	// 	log.Println("COULD NOT HASH USER PASSWORD.")
	// }

	// userId := uuid.NewString()

	// // this returns the ID
	// _, newUserErr := dbInst.DB.Exec(queries.CreateNewOrgUser, "test@orchdio.com", userId, hashedPass)
	// // now, create an org
	// //
	// orgId := uuid.NewString()
	// if newUserErr != nil {
	// 	log.Println("ERROR CREATING NEW TEST USER")
	// }
	// _, err = dbInst.CreateOrg(orgId, "TestOrg", "Sample test org", userId)
	// if err != nil {
	// 	log.Println("Could not create test org")
	// }

	// newAppData := &blueprint.CreateNewDeveloperAppData{
	// 	Name:                 "test_app",
	// 	Description:          "Some description here",
	// 	WebhookURL:           "https://orchdio.com/test/webhook",
	// 	Organization:         orgId,
	// 	IntegrationAppSecret: "secret_abc",
	// 	// the app id from when the user creates a new app id on the platform of choice
	// 	IntegrationAppId:    "platform_id",
	// 	RedirectURL:         "https://orchdio.com/zoove/redirect",
	// 	IntegrationPlatform: "spotify",
	// }

	// pubKey := uuid.NewString()
	// privateKey := uuid.NewString()
	// verifySecret := "verify_verify"

	// // now we want to create a new app so we can use the API key to make requests.
	// _, err = dbInst.CreateNewApp(
	// 	newAppData.Name,
	// 	newAppData.Description,
	// 	newAppData.RedirectURL,
	// 	newAppData.WebhookURL,
	// 	pubKey,
	// 	userId,
	// 	privateKey,
	// 	verifySecret,
	// 	orgId,
	// 	"",
	// )

	// // then update the integration credentials
	// if err != nil {
	// 	log.Println("COULD NOT CREATE A NEW APP")
	// }

	app.Post("/v1/org/new", userController.CreateOrg)
	app.Post("/v1/track/convert", authMiddleware.AddReadOnlyDeveloperToContext, middleware.ExtractLinkInfoFromBody, platformsHandler.ConvertTrack)

	go func() {
		app.Listen(":4200")
	}()

	p := &TestSuite{}
	p.App = app
	p.DB = dbase

	return p
}
