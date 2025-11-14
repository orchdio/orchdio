package platforms_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"orchdio/blueprint"
	"orchdio/controllers/account"
	"orchdio/testutils"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/suite"
)

type PlatformsTestSuite struct {
	suite.Suite
	App    *fiber.App
	DB     *sqlx.DB
	DevApp *blueprint.DeveloperApp
}

type TestUserController struct {
	*account.UserController
}

func (p *PlatformsTestSuite) SetupSuite() {
	var testDependencies = testutils.SetupTestSuite()

	p.App = testDependencies.App
	p.DB = testDependencies.DB
	p.DevApp = testDependencies.DevApp
}

func TestPlatformsTestSuite(t *testing.T) {
	suite.Run(t, &PlatformsTestSuite{})
}

var orgId = ""
var orgJwt = ""

// Define response wrapper to match util.SuccessResponse structure
type SuccessResponseWrapper[T any] struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
	Data    T      `json:"data"`
}

func (p *PlatformsTestSuite) TestB_CreateNewDevApp() {
	newAppData := &blueprint.CreateNewDeveloperAppData{
		Name:                 "test_app",
		Description:          "Some description here",
		WebhookURL:           "https://orchdio.com/test/webhook",
		Organization:         orgId,
		IntegrationAppSecret: "secret_abc",
		// the app id from when the user creates a new app id on the platform of choice
		IntegrationAppId:    "platform_id",
		RedirectURL:         "https://orchdio.com/zoove/redirect",
		IntegrationPlatform: "spotify",
	}

	serializedBody, err := json.Marshal(newAppData)
	if err != nil {
		log.Println("Could not serialize createNewDevApp ")
		panic(err)
	}

	// endpoint is: /:orgId/app/new
	routeUrl := fmt.Sprintf("/v1/org/%s/app/new", orgId)
	bearerToken := fmt.Sprintf("Bearer %s", orgJwt)

	req := httptest.NewRequest("POST", routeUrl, strings.NewReader(string(serializedBody)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", bearerToken)

	res, err := p.App.Test(req)
	if err != nil {
		log.Println("Could not create new developer App")
		panic(err)
	}

	defer res.Body.Close()
	bod, err := io.ReadAll(res.Body)
	if err != nil {
		log.Println("Could not read IO utils body")
	}

	var response SuccessResponseWrapper[blueprint.CreateNewDevAppResponse]
	err = json.Unmarshal(bod, &response)
	p.Equal("Request Ok", response.Message)
	p.NotEmpty(response.Data.AppId, "App ID should not be empty")
	p.Equal(201, res.StatusCode)

}
func (p *PlatformsTestSuite) TestA_CreateNewOrg() {
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

	var response SuccessResponseWrapper[blueprint.OrchdioOrgCreateResponse]
	err = json.Unmarshal(bod, &response)
	p.NoError(err, "Should be able to unmarshal response")

	p.Equal(201, res.StatusCode)

	p.Equal("Request Ok", response.Message)
	p.Equal(201, response.Status)

	p.NotEmpty(response.Data.OrgID, "OrgID should not be empty")
	p.Equal("TestOrg", response.Data.Name)
	p.Equal("some test organization", response.Data.Description)
	p.NotEmpty(response.Data.Token, "JWT token should not be empty")

	// set the orgId & token in the local variables, to be used by the other endpoints, namely the endpoint to create a new devApp
	orgId = response.Data.OrgID
	orgJwt = response.Data.Token
}

// func (p *PlatformsTestSuite) TestConvertTrack() {
// 	defer gock.Off()

// 	// gock.New("https://api")

// 	conversion := &blueprint.ConversionBody{
// 		URL:            "https://open.spotify.com/track/01z2fBGB8Hl3Jd3zXe4IXR?si=d3685829cc0e498f",
// 		TargetPlatform: "all",
// 	}

// 	serConversion, err := json.Marshal(conversion)
// 	if err != nil {
// 		log.Println("Could not serialize conversion body")
// 	}

// 	req := httptest.NewRequest("POST", "/v1/track/convert", strings.NewReader(string(serConversion)))
// 	req.Header.Add("Content-Type", "application/json")
// 	req.Header.Add("x-orchdio-public-key", p.DevApp.PublicKey.String())

// 	res, err := p.App.Test(req)
// 	log.Printf("TEST SUITE TEST ERROR IS: %v", err)

// 	defer res.Body.Close()
// 	p.Equal(200, res.StatusCode)
// }

func (p *PlatformsTestSuite) TearDownSuite() {
	log.Print("CLEANING UP AFTER INDIVIDUAL TEST...")
	// TRUNCATE removes all data but keeps the table structure
	_, err := p.DB.Exec("TRUNCATE apps, follows, organizations, tasks, user_apps, users, waitlists CASCADE")
	if err != nil {
		log.Printf("ERROR TRUNCATING TABLES: %v", err)

		p.T().Fatal(err) // fail the test if cleanup fails
	}
}
