package integration_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"orchdio/blueprint"
	"orchdio/db"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/h2non/gock"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)

type PlatformTestSuite struct {
	suite.Suite
	App    *fiber.App
	DB     *sqlx.DB
	DevApp *blueprint.DeveloperApp
}

func (p *DevAppTestSuite) TestD_ConvertTrack() {
	// get the app from the db, raw
	//
	defer gock.Off()

	// configure gock to only intercept Spotify domains
	// block all external requests by default
	gock.DisableNetworking()

	// creates a filter that allows non-streaming platforms requests to pass through
	gock.NetworkingFilter(func(req *http.Request) bool {
		host := req.URL.Host

		// return true to ALLOW the request to pass through (bypass gock)
		// return false to INTERCEPT with gock

		// allow localhost
		if strings.Contains(host, "localhost") || strings.Contains(host, "127.0.0.1") {
			return true
		}

		// allow Svix and other services (bypass gock). but we already override the real methods in tests so this still works
		if strings.Contains(host, "svix.com") {
			return true
		}

		// Block/intercept Spotify (gock will handle it)
		if lo.Contains([]string{"spotify.com", "deezer.com"}, host) {
			return false
		}

		// Allow everything else (including Tidal, or add more conditions)
		return true
	})

	gock.New("https://api.deezer.com").
		Get("/search").
		MatchParam("q", `track:"Test Track Name" artist:"Test Artist"`).
		Reply(200).
		JSON(MockDeezerTrack)

	gock.New("https://accounts.spotify.com").
		Post("/api/token").
		Reply(200).
		JSON(map[string]interface{}{
			"access_token": "mock_access_token_12345",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})

	gock.New("https://api.spotify.com").
		Get("/v1/tracks/01z2fBGB8Hl3Jd3zXe4IXR").
		Reply(200).
		JSON(MockedSpotifyTrack)

	dbInst := db.NewDB{DB: p.DB}
	devApp, err := dbInst.FetchAppByAppId(AppId)
	if err != nil {
		log.Println("Could not fetch DevApp")
		panic(err)
	}

	p.DevApp = devApp
	// endpoint is /v1/track/convert
	conversionBody := &blueprint.ConversionBody{
		URL:            "https://open.spotify.com/track/01z2fBGB8Hl3Jd3zXe4IXR",
		TargetPlatform: "deezer",
	}

	serializedBody, err := json.Marshal(conversionBody)
	if err != nil {
		log.Println("Could not serialized track conversion body")
		panic(err)
	}

	req := httptest.NewRequest("POST", "/v1/track/convert", strings.NewReader(string(serializedBody)))
	req.Header.Add("x-orchdio-public-key", p.DevApp.PublicKey.String())
	req.Header.Add("content-type", "application/json")

	res, err := p.App.Test(req)
	if err != nil {
		log.Println("Could not test track conversion endpoint")
		panic(err)
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)

	if err != nil {
		log.Print("Could not ready response body")
		panic(err)
	}

	var response SuccessResponseWrapper[*blueprint.TrackConversion]

	err = json.Unmarshal(body, &response)

	if err != nil {
		log.Println("Could not deserialize response body")
		panic(err)
	}

	p.Equal(200, res.StatusCode)
	p.NotEmpty(response.Data.UniqueID, "UniqueID is not present")
	p.NotEmpty(response.Data.Platforms.Deezer, "Deezer result not present")
	p.NotEmpty(response.Data.Platforms.Spotify, "Spotify result not present")
	p.Equal("track", response.Data.Entity)
}
