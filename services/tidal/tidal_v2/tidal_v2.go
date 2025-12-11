package tidal_v2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/oauth2"
)

type TidalClient struct {
	httpClient  *http.Client
	CountryCode string
	baseURL     string
}

const defaultRetryDuration = time.Second * 5

func NewTidalClient(Client *http.Client) *TidalClient {
	client := &TidalClient{
		httpClient:  Client,
		CountryCode: "en-US",
		baseURL:     "https://openapi.tidal.com/v2/",
	}
	return client
}

type Error struct {
	Errors []map[string]string `json:"errors"`
}

func (tc *TidalClient) Token() (*oauth2.Token, error) {
	transp, ok := tc.httpClient.Transport.(*oauth2.Transport)
	if !ok {
		return nil, errors.New("tidal client: client is not backed by oauth2 transport")
	}

	t, err := transp.Source.Token()
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (tc *TidalClient) execute(req *http.Request, result interface{}, acceptedStatuses ...int) error {

	req.Header.Set("accept", "application/vnd.api+json")
	// req.Header.Set("content-type", "application/vnd.api+json")
	for {
		resp, err := tc.httpClient.Do(req)
		if err != nil {
			log.Println("ERROR: could not execute TIDAL http request")
			return err
		}

		defer resp.Body.Close()

		if isFailedRequest(resp.StatusCode, acceptedStatuses) && shouldRetry(resp.StatusCode) {
			select {
			case <-req.Context().Done():
				log.Println("DEBUG: cancelled context on request exec")
			case <-time.After(retryDuration(resp)):
				continue
			}
		}

		if resp.StatusCode == http.StatusNoContent {
			return nil
		}

		if (resp.StatusCode >= 300 || resp.StatusCode > 200) && isFailedRequest(resp.StatusCode, acceptedStatuses) {
			return tc.decodeErrorResponse(resp)
		}

		if result != nil {
			err := json.NewDecoder(resp.Body).Decode(result)
			if err != nil {
				return err
			}
		}

		break
	}
	return nil
}

func (tc *TidalClient) get(ctx context.Context, url string, result interface{}) error {
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("accept", "application/vnd.api+json")
		req.Header.Set("content-type", "application/vnd.tidal.v1+json")

		if err != nil {
			log.Println("TIDAL error 12")
			spew.Dump(err)
			return err
		}

		resp, err := tc.httpClient.Do(req)
		if err != nil {
			log.Println("TIDAL error 1", err)
			return err
		}

		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			select {
			case <-ctx.Done():

			case <-time.After(retryDuration(resp)):
				continue
			}
		}

		if resp.StatusCode == http.StatusNoContent {
			log.Println("TIDAL error 2")

			return nil
		}

		if resp.StatusCode != http.StatusOK {
			return tc.decodeErrorResponse(resp)
		}

		err = json.NewDecoder(resp.Body).Decode(result)
		if err != nil {
			log.Println("TIDAL error 3", err)
			return err
		}
		break
	}
	return nil
}

func (tc *TidalClient) decodeErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if len(body) == 0 {
		return fmt.Errorf("tidal client: HTTP %d: %s (body empty)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	buf := bytes.NewBuffer(body)

	var e struct {
		Err Error `json:"error"`
	}

	err = json.NewDecoder(buf).Decode(&e)

	if err != nil {
		return fmt.Errorf("tidal client: could not decode TIDAL error: (%d) [%s]", len(body), body)
	}
	return nil
}

func shouldRetry(status int) bool {
	return status == http.StatusAccepted || status == http.StatusTooManyRequests
}

func isFailedRequest(code int, validStatuses []int) bool {
	return !slices.Contains(validStatuses, code)
}

func retryDuration(resp *http.Response) time.Duration {
	rawHeader := resp.Header.Get("retry-after")
	if rawHeader == "" {
		return defaultRetryDuration
	}

	seconds, err := strconv.ParseInt(rawHeader, 10, 32)
	if err != nil {
		return defaultRetryDuration
	}

	return time.Duration(seconds) * time.Second
}

// parseISO8601Duration parses an ISO 8601 format returned by TIDAL API.
// A sample ISO 8601 looks like: PT11M27S.
//   - P: means Period of Time. This is always the start of the duration string
//   - T11: means 11 minutes
//   - 27S: means 27 seconds
//
// https://docs.digi.com/resources/documentation/digidocs/90001488-13/reference/r_iso_8601_duration_format.htm
func ParseISO8601Duration(iso8601 string) (time.Duration, error) {
	// Regex to match PT{hours}H{minutes}M{seconds}S format
	re := regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?$`)
	matches := re.FindStringSubmatch(iso8601)

	if matches == nil {
		return 0, fmt.Errorf("invalid ISO 8601 duration format: %s", iso8601)
	}

	var duration time.Duration

	// Hours
	if matches[1] != "" {
		hours, _ := strconv.Atoi(matches[1])
		duration += time.Duration(hours) * time.Hour
	}

	// Minutes
	if matches[2] != "" {
		minutes, _ := strconv.Atoi(matches[2])
		duration += time.Duration(minutes) * time.Minute
	}

	// Seconds (can be float)
	if matches[3] != "" {
		seconds, _ := strconv.ParseFloat(matches[3], 64)
		duration += time.Duration(seconds * float64(time.Second))
	}

	return duration, nil
}
