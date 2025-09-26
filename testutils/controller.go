package testutils

import (
	"encoding/json"
	"log"
	"net/http/httptest"
	"orchdio/blueprint"
	"orchdio/controllers/platforms"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

// ControllerTestSuite provides a base for controller testing
type ControllerTestSuite struct {
	App                *fiber.App
	Infra              *TestInfrastructure
	DataBuilder        *TestDataBuilder
	PlatformController *platforms.Platforms
	t                  *testing.T
}

// NewControllerTestSuite creates a new controller test suite
func NewControllerTestSuite(t *testing.T) *ControllerTestSuite {
	t.Helper()

	log.Print("CREATING NEW CONTROLLER TEST SUITE....")

	config := TestConfig{
		PostgresImage:    "postgres",
		RedisImage:       "redis",
		DatabaseName:     "orchdio_test",
		MigrationsPath:   "../../db/migration",
		NetworkName:      "orchdio_test_network",
		ReuseContainers:  true,
		ContainerTimeout: 120 * time.Second,
	}

	infra := GetOrCreateInfrastructure(t, &config)

	// Create isolated database for this test suite
	// testDB := infra.NewDatabase(t, "test_"+GenerateTestShortID())

	suite := &ControllerTestSuite{
		App:         fiber.New(),
		Infra:       infra,
		DataBuilder: NewTestDataBuilder(t, infra.DB),
		t:           t,
	}

	// Initialize platform controller
	suite.PlatformController = platforms.NewPlatform(
		infra.RedisClient,
		infra.DB,
		nil, // asynqClient - can be mocked if needed
		nil, // asynqMux - can be mocked if needed
	)

	return suite
}

// SetupRequest sets up a fiber request with context locals
func (s *ControllerTestSuite) SetupRequest(method, path, body string, linkInfo *blueprint.LinkInfo, app *blueprint.DeveloperApp) (*httptest.ResponseRecorder, error) {
	s.t.Helper()

	// setup middleware to inject context locals
	s.App.Use(func(c *fiber.Ctx) error {
		if linkInfo != nil {
			c.Locals("linkInfo", linkInfo)
		}
		if app != nil {
			c.Locals("app", app)
		}
		return c.Next()
	})

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	return httptest.NewRecorder(), nil
}

// ParseResponse parses the response into the given struct
func (s *ControllerTestSuite) ParseResponse(resp *httptest.ResponseRecorder, v interface{}) {
	s.t.Helper()

	err := json.NewDecoder(resp.Body).Decode(v)
	require.NoError(s.t, err)
}

// cleanup cleans up test data
func (s *ControllerTestSuite) Cleanup() {
	s.DataBuilder.CleanupTestData()
}
