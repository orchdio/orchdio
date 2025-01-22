package svixwebhook

import (
	"context"
	"fmt"
	svix "github.com/svix/svix-webhooks/go"
	"log"
	"orchdio/blueprint"
	xlogger "orchdio/logger"
)

type SvixWebhook struct {
	AuthToken string `json:"auth_token"`
	Client    *svix.Svix
}

//type WebhookProvider interface {
//	SendTrackEvent(appId string, payload *blueprint.PlaylistConversionEventTrack) bool
//}

func New(authToken string, debug bool) *SvixWebhook {
	c := svix.New(authToken, &svix.SvixOptions{
		Debug: debug,
	})

	return &SvixWebhook{
		AuthToken: authToken,
		Client:    c,
	}
}

func (s *SvixWebhook) CreateApp(name, uid string) (*svix.ApplicationOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	// todo: configure application options, including context argument (and other places using ctx)
	whApp, err := s.Client.Application.Create(context.TODO(), &svix.ApplicationIn{
		Name: name,
		Uid:  &uid,
	})

	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not create new svix app.")
		return nil, err
	}

	return whApp, err
}

func (s *SvixWebhook) GetApp(appId string) (*svix.ApplicationOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	whApp, err := s.Client.Application.Get(context.TODO(), appId)
	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not get svix app.")
		return nil, err
	}

	return whApp, err
}

func (s *SvixWebhook) DeleteApp(appId string) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	err := s.Client.Application.Delete(context.TODO(), appId)
	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not delete svix app.")
		return err
	}
	return nil
}

func (s *SvixWebhook) CreateEndpoint(appId, uid, endpoint string) (*svix.EndpointOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	whEnd, err := s.Client.Endpoint.Create(context.Background(), appId, &svix.EndpointIn{
		Url: endpoint,
		// todo: add more events.
		FilterTypes: []string{blueprint.PlaylistConversionMetadataEvent, blueprint.PlaylistConversionTrackEvent, blueprint.PlaylistConversionDoneEvent},
		Uid:         &uid,
	})

	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not create new endpoint.")
		return nil, err
	}

	return whEnd, nil

}

func (s *SvixWebhook) GetEndpoint(appId, endpoint string) (*svix.EndpointOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	whEnd, err := s.Client.Endpoint.Get(context.Background(), appId, endpoint)
	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not get endpoint.")
		return nil, err
	}

	return whEnd, err
}

func (s *SvixWebhook) UpdateEndpoint(appId, endpointId, endpoint string) (*svix.EndpointOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	whResponse, err := s.Client.Endpoint.Update(context.TODO(), appId, endpointId, &svix.EndpointUpdate{
		Url: endpoint,
	})

	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not update endpoint.")
		return nil, err
	}

	return whResponse, err
}

func (s *SvixWebhook) ListEndpoints(appId string) (*svix.ListResponseEndpointOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	whEndpoints, err := s.Client.Endpoint.List(context.Background(), appId, &svix.EndpointListOptions{
		Limit: svix.Int32(250),
	})

	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not list endpoints.")
		return nil, err
	}

	return whEndpoints, nil
}

func (s *SvixWebhook) DeleteEndpoint(appId, endpointId string) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	err := s.Client.Endpoint.Delete(context.Background(), appId, endpointId)
	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not delete endpoint.")
		return err
	}

	return nil
}

func (s *SvixWebhook) SendEvent(appId, eventType string, payload interface{}) (*svix.MessageOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	whMsg, err := s.Client.Message.Create(context.TODO(), appId, &svix.MessageIn{
		// todo: use constant event types.
		EventType: eventType,
		Payload: map[string]interface{}{
			"data": payload,
		},
	})

	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not send event.")
		return nil, err
	}

	return whMsg, nil
}

func (s *SvixWebhook) CreateEventType(eventName, description string) (*svix.EventTypeOut, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := xlogger.NewZapSentryLogger(loggerOpts)

	whEType, err := s.Client.EventType.Create(context.TODO(), &svix.EventTypeIn{
		Name:        eventName,
		Description: description,
	})

	if err != nil {
		logger.Error("[webhooks][svix-webhook] error - could not create new event type.")
		return nil, err
	}
	return whEType, nil
}

func FormatSvixEndpointUID(devAppId string) string {
	return fmt.Sprintf("endpoint_%s", devAppId)
}

func FormatSvixAppUID(devAppId string) string {
	return fmt.Sprintf("orch_app_%s", devAppId)
}

func (s *SvixWebhook) SendTrackEvent(appId string, out *blueprint.PlaylistConversionEventTrack) bool {
	_, whErr := s.SendEvent(appId, blueprint.PlaylistConversionTrackEvent, out)
	if whErr != nil {
		log.Printf("\n[services] error - Could not send webhook event: %v\n", whErr)
		return false
	}
	return true
}
