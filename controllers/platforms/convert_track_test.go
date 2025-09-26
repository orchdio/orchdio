package platforms_test

import (
	"net/http/httptest"
	"orchdio/blueprint"
	"orchdio/testutils"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertTrack(t *testing.T) {
	suite := testutils.NewControllerTestSuite(t)
	defer suite.Cleanup()

	// Create test data
	user := suite.DataBuilder.CreateTestUser("test@example.com")
	org := suite.DataBuilder.CreateTestOrganization("Test Org", "Test Description", user.UUID)
	app := suite.DataBuilder.CreateTestApp("Test App", "Test App Description", user.UUID, org.UID)

	tests := []struct {
		name           string
		linkInfo       *blueprint.LinkInfo
		expectedStatus int
	}{
		{
			name: "missing target platform",
			linkInfo: &blueprint.LinkInfo{
				Platform:       "spotify",
				Entity:         "track",
				EntityID:       "spotify_track_123",
				TargetPlatform: "",
				App:            app.UID.String(),
			},
			expectedStatus: 400,
		},
		{
			name: "non-track entity",
			linkInfo: &blueprint.LinkInfo{
				Platform:       "spotify",
				Entity:         "playlist",
				EntityID:       "spotify_playlist_123",
				TargetPlatform: "deezer",
				App:            app.UID.String(),
			},
			expectedStatus: 501,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite.App.Post("/convert", suite.PlatformController.ConvertTrack)

			_, err := suite.SetupRequest("POST", "/convert", "{}", tt.linkInfo, app)
			require.NoError(t, err)

			testResp, err := suite.App.Test(httptest.NewRequest("POST", "/convert", nil))
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, testResp.StatusCode)
		})
	}
}
