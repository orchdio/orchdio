package soundcloud_sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"time"

	"golang.org/x/oauth2"
)

type SoundcloudClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewSoundcloudClient(Client *http.Client) *SoundcloudClient {
	client := &SoundcloudClient{
		httpClient: Client,
		// todo: add baseURL here.
		baseURL: "https://api.soundcloud.com/",
	}

	return client
}

type Error struct {
	Errors []map[string]string `json:"errors"`
}

const defaultRetryDuration = time.Second * 5

func (sc *SoundcloudClient) Token() (*oauth2.Token, error) {
	transport, ok := sc.httpClient.Transport.(*oauth2.Transport)
	if !ok {
		return nil, errors.New("soundcloud client: client not backed by oauth2 transport")
	}
	t, err := transport.Source.Token()
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (sc *SoundcloudClient) execute(req *http.Request, result interface{}, acceptedStatuses ...int) error {

	req.Header.Set("accept", "application/json")
	// req.Header.Set("content-type", "application/json")
	for {
		resp, err := sc.httpClient.Do(req)
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
			return sc.decodeErrorResponse(resp)
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

func (sc *SoundcloudClient) decodeErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if len(body) == 0 {
		return fmt.Errorf("soundcloud client: HTTP %d: %s (body empty)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	buf := bytes.NewBuffer(body)

	var e struct {
		Err Error `json:"error"`
	}

	err = json.NewDecoder(buf).Decode(&e)

	if err != nil {
		return fmt.Errorf("soundcloud client: could not decode TIDAL error: (%d) [%s]", len(body), body)
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

func (sc *SoundcloudClient) get(ctx context.Context, url string, result interface{}) error {
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("accept", "application/json")
		req.Header.Set("content-type", "application/json")

		if err != nil {
			return err
		}

		resp, err := sc.httpClient.Do(req)
		if err != nil {
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
			return nil
		}

		if resp.StatusCode != http.StatusOK {
			return sc.decodeErrorResponse(resp)
		}

		err = json.NewDecoder(resp.Body).Decode(result)
		if err != nil {
			return err
		}
		break
	}
	return nil
}
