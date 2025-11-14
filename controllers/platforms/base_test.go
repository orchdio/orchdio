package platforms_test

import (
	"encoding/json"
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
	App *fiber.App
	DB  *sqlx.DB
}

type TestUserController struct {
	*account.UserController
}

func (p *PlatformsTestSuite) SetupSuite() {
	var testDependencies = testutils.SetupTestSuite()

	p.App = testDependencies.App
	p.DB = testDependencies.DB
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

	// Define response wrapper to match util.SuccessResponse structure
	type SuccessResponseWrapper struct {
		Message string                             `json:"message"`
		Status  int                                `json:"status"`
		Data    blueprint.OrchdioOrgCreateResponse `json:"data"`
	}

	var response SuccessResponseWrapper
	err = json.Unmarshal(bod, &response)
	p.NoError(err, "Should be able to unmarshal response")

	// Assert HTTP status code
	p.Equal(201, res.StatusCode)

	// Assert wrapper structure
	p.Equal("Request Ok", response.Message)
	p.Equal(201, response.Status)

	// Assert OrchdioOrgCreateResponse data structure
	p.NotEmpty(response.Data.OrgID, "OrgID should not be empty")
	p.Equal("TestOrg", response.Data.Name)
	p.Equal("some test organization", response.Data.Description)
	p.NotEmpty(response.Data.Token, "JWT token should not be empty")

	log.Printf("✅ Successfully created org with ID: %s", response.Data.OrgID)

	// var respp = &blueprint.OrchdioOrgCreateResponse{}
	// p.Equal(201, res.StatusCode)
	// log.Println("created org info is...")

	// spew.Dump(string(bod))
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
// 	req.Header.Add("x-orchdio-public-key", p.Pubkey)

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

		p.T().Fatal(err) // Fail the test if cleanup fails
	}
}
