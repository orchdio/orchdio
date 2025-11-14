package platforms_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http/httptest"
	"orchdio/blueprint"
	"orchdio/controllers/account"
	"orchdio/controllers/platforms"
	"orchdio/db"
	"orchdio/middleware"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/h2non/gock"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/suite"
)

type PlatformsTestSuite struct {
	suite.Suite
	App        *fiber.App
	DB         *sqlx.DB
	Pubkey     string
	PrivateKey string
}

func SetupMigration() {}

type TestUserController struct {
	*account.UserController
}

type QueueService interface {
	EnqueueTask(task *asynq.Task, q, taskId string, processIn time.Duration)
	NewTask(taskType, queue string, retry int, payload []byte) (*asynq.Task, error)
}

type MockQueue struct{}

func (m *MockQueue) EnqueueTask(task *asynq.Task, q, taskId string, processIn time.Duration) error {
	log.Println("Mocked enqueued task")
	return nil
}

// Add this method
func (m *MockQueue) NewTask(taskType, queue string, retry int, payload []byte) (*asynq.Task, error) {
	log.Println("Mocked queue instantiation")
	return nil, nil
}

func (t *TestUserController) SendAdminWelcomeEmail(ctx context.Context, email string) error {
	log.Println("Overridden SendAdminWelcomeEmail method for test case....")
	return nil
}

func (p *PlatformsTestSuite) SetupSuite() {
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

	// create the test user db here, i suppose...
	//
	//
	redisOpts, err := redis.ParseURL("redis://localhost:6379")
	if err != nil {
		log.Printf("⛔ Error parsing redis url")
	}

	// configure redis. following configuration is for the queue system setup, using redis.
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

	log.Println("App is up and running")
	spew.Dump(p.App)

	p.App = app
	p.DB = dbase
	// p.Pubkey = pubKey
	// p.PrivateKey = privateKey
	//
}

func TestPlatformsTestSuite(t *testing.T) {
	suite.Run(t, &PlatformsTestSuite{})
}

func (p *PlatformsTestSuite) TestCreateNewOrg() {
	// url is: /v1/org/new
	createNewAppBody := &blueprint.CreateOrganizationData{
		Name:          "TestOrg",
		Description:   "some test organization",
		OwnerEmail:    "test@orchdio.com",
		OwnerPassword: "password",
	}

	serializedBody, err := json.Marshal(createNewAppBody)
	if err != nil {
		log.Println("Could not serialize the body here...")
		panic(err)
	}

	req := httptest.NewRequest("POST", "/v1/org/new", strings.NewReader(string(serializedBody)))
	req.Header.Add("Content-Type", "application/json")

	res, err := p.App.Test(req)
	if err != nil {
		log.Println("COULD NOT CREATE A NEW APP")
		panic(err)
	}

	defer res.Body.Close()
	bod, err := io.ReadAll(res.Body)
	if err != nil {
		log.Println("Could not read IO utils body")
	}

	println("the response")
	p.Equal(201, res.StatusCode)

	log.Println("created org info is...")
	spew.Dump(string(bod))

}

func (p *PlatformsTestSuite) TestConvertTrack() {
	defer gock.Off()

	// gock.New("https://api")

	conversion := &blueprint.ConversionBody{
		URL:            "https://open.spotify.com/track/01z2fBGB8Hl3Jd3zXe4IXR?si=d3685829cc0e498f",
		TargetPlatform: "all",
	}

	serConversion, err := json.Marshal(conversion)
	if err != nil {
		log.Println("Could not serialize conversion body")
	}

	req := httptest.NewRequest("POST", "/v1/track/convert", strings.NewReader(string(serConversion)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("x-orchdio-public-key", p.Pubkey)

	res, err := p.App.Test(req)
	log.Printf("TEST SUITE TEST ERROR IS: %v", err)

	defer res.Body.Close()
	p.Equal(200, res.StatusCode)
}

func (p *PlatformsTestSuite) TearDownSuite() {
	log.Print("TEST TEAR DOWN HERE...")

	_, err := p.DB.Exec("TRUNCATE apps, follows, organizations, schema_migrations, tasks, user_apps, users, waitlists")
	if err != nil {
		log.Println("ERROR TEARING DOWN ALL DATA IN THE DATABASE")
		panic(err)
	}
}
