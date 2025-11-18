package platforms_test

// import (
// 	"encoding/json"
// 	"io"
// 	"log"
// 	"net/http/httptest"
// 	"orchdio/blueprint"
// 	"orchdio/testutils"
// 	"strings"
// 	"testing"

// 	"github.com/davecgh/go-spew/spew"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/jmoiron/sqlx"
// 	"github.com/stretchr/testify/suite"
// )

// type PlatformTestSuite struct {
// 	suite.Suite
// 	App    *fiber.App
// 	DB     *sqlx.DB
// 	DevApp *blueprint.DeveloperApp
// }

// func (p *PlatformTestSuite) SetupSuite() {
// 	testDependencies := testutils.SetupTestSuite()

// 	// log.Println("Setup suite for base test...")
// 	// spew.Dump(testDependencies)

// 	p.App = testDependencies.App
// 	p.DB = testDependencies.DB
// 	p.DevApp = testDependencies.DevApp
// }

// func TestPlatformsTestSuite(t *testing.T) {
// 	suite.Run(t, &PlatformTestSuite{})
// }

// func (p *PlatformTestSuite) TestC_ConvertTrack() {
// 	// endpoint is /v1/track/convert
// 	conversionBody := &blueprint.ConversionBody{
// 		URL:            "https://open.spotify.com/track/01z2fBGB8Hl3Jd3zXe4IXR?si=d3685829cc0e498f",
// 		TargetPlatform: "tidal",
// 	}

// 	serializedBody, err := json.Marshal(conversionBody)
// 	if err != nil {
// 		log.Println("Could not serialized track conversion body")
// 		panic(err)
// 	}

// 	req := httptest.NewRequest("POST", "/v1/track/convert", strings.NewReader(string(serializedBody)))
// 	req.Header.Add("x-orchdio-public-key", p.DevApp.PublicKey.String())
// 	req.Header.Add("content-type", "application/json")

// 	res, err := p.App.Test(req)
// 	if err != nil {
// 		log.Println("Could not test track conversion endpoint")
// 		panic(err)
// 	}

// 	defer req.Body.Close()
// 	bod, err := io.ReadAll(req.Body)

// 	if err != nil {
// 		log.Print("Could not ready response body")
// 		panic(err)
// 	}

// 	log.Println("Body is")
// 	spew.Dump(string(bod))

// 	p.Equal(res.StatusCode, 200)
// }

// // func (p *PlatformTestSuite) TearDownSuite() {
// // 	log.Print("CLEANING UP AFTER INDIVIDUAL TEST...")
// // 	// TRUNCATE removes all data but keeps the table structure
// // 	_, err := p.DB.Exec("TRUNCATE apps, follows, organizations, tasks, user_apps, users, waitlists CASCADE")
// // 	if err != nil {
// // 		log.Printf("ERROR TRUNCATING TABLES: %v", err)

// // 		p.T().Fatal(err) // fail the test if cleanup fails
// // 	}
// // }
