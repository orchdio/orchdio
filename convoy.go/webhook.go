package webhook

import (
	convoy_go ""
	"context"
	"encoding/json"
	convoy_go "github.com/frain-dev/convoy-go/v2"
	"go.uber.org/zap"
	"orchdio/blueprint"
	logger2 "orchdio/logger"
	"os"
)

type Convoy struct {
	Client *convoy_go.Client
}

const (
	ORCHDIO_EVENT_TYPE_TRACK_CONVERTED = "track:conversion"
	BASE_URL                           = "https://dashboard.getconvoy.io/api/v1"
)

func NewConvoy() *Convoy {
	logger := convoy_go.NewLogger(os.Stdout, convoy_go.DebugLevel)
	c := convoy_go.New(BASE_URL, os.Getenv("CONVOY_API_KEY"),
		os.Getenv("CONVOY_PROJECT_ID"), convoy_go.OptionLogger(logger))
	return &Convoy{
		Client: c,
	}
}

func (c *Convoy) CreateEndpoint(url, description, name string) (*blueprint.ConvoyWebhookCreate, error) {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := logger2.NewZapSentryLogger(loggerOpts)
	endpoint, err := c.Client.Endpoints.Create(context.Background(), &convoy_go.CreateEndpointRequest{
		URL:         url,
		Description: description,
		Name:        name,
	}, nil)

	if err != nil {
		logger.Error("[convoy][CreateEndpoint] - error creating endpoint.", zap.Error(err), zap.String("webhook_url", url))
		return nil, err
	}

	subscriptionOpts := &convoy_go.CreateSubscriptionRequest{
		EndpointID: endpoint.UID,
		Name:       "orchdio",
	}
	_, err = c.Client.Subscriptions.Create(context.Background(), subscriptionOpts)
	if err != nil {
		logger.Error("[convoy][CreateEndpoint] - error creating subscription.", zap.Error(err), zap.String("webhook_url", url))
		return nil, err
	}

	return &blueprint.ConvoyWebhookCreate{
		ID:          endpoint.UID,
		Description: endpoint.Description,
		URL:         endpoint.TargetUrl,
	}, nil
}

func (c *Convoy) UpdateEndpoint(uid, url, description, name string) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := logger2.NewZapSentryLogger(loggerOpts)

	_, err := c.Client.Endpoints.Update(context.Background(), uid, &convoy_go.CreateEndpointRequest{
		URL:         url,
		Description: description,
		Name:        name,
	}, nil)

	if err != nil {
		logger.Error("[convoy][UpdateEndpoint] - error updating endpoint.", zap.Error(err), zap.String("webhook_url", url))
		return err
	}

	return nil
}

func (c *Convoy) DeleteEndpoint(uid string) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := logger2.NewZapSentryLogger(loggerOpts)

	err := c.Client.Endpoints.Delete(context.Background(), uid, nil)
	if err != nil {
		logger.Error("[convoy][DeleteEndpoint] - error deleting endpoint.", zap.Error(err), zap.String("endpoint_id", uid))
		return err
	}

	return nil
}

func (c *Convoy) PauseEndpoint(uid string) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := logger2.NewZapSentryLogger(loggerOpts)

	_, err := c.Client.Endpoints.Pause(context.Background(), uid)
	if err != nil {
		logger.Error("[convoy][PauseEndpoint] - error pausing endpoint.", zap.Error(err))
		return err
	}

	return nil
}

func (c *Convoy) SendEvent(endpointId, event string, data interface{}) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{}
	logger := logger2.NewZapSentryLogger(loggerOpts)

	decodedMsg, err := json.Marshal(data)
	if err != nil {
		logger.Error("[convoy][SendEvent] - Error sending event. Could not parse data", zap.Error(err), zap.String("endpoint_id", endpointId))
		return err
	}

	logger.Info("[convoy][SendEvent] - Sending event", zap.String("endpoint_id", endpointId), zap.String("event", event), zap.Any("data", data))

	defer func() {
		logger.Info("executing deferred function")
	}()

	_, e := c.Client.Events.Create(context.Background(), &convoy_go.CreateEventRequest{
		EndpointID: endpointId,
		EventType:  event,
		Data:       decodedMsg,
	})

	if e != nil {
		logger.Error("[convoy][SendEvent] - Error sending event", zap.Error(e), zap.String("endpoint_id", endpointId))
		return e
	}

	return nil
}

//func (c *Convoy) SendTrackConvertedEvent(endpointId string, data interface{}, event string) error {
//	loggerOpts := &blueprint.OrchdioLoggerOptions{}
//	logger := logger2.NewZapSentryLogger(loggerOpts)
//
//	decodedMsg, err := json.Marshal(data)
//	if err != nil {
//		logger.Error("[convoy][SendTrackConvertedEvent] - Error sending event. Could not parse data", zap.Error(err), zap.String("endpoint_id", endpointId))
//		return err
//	}
//
//	logger.Info("[convoy][SendTrackConvertedEvent] - Sending event", zap.String("endpoint_id", endpointId), zap.String("event", event), zap.Any("data", data))
//
//	defer func() {
//		logger.Info("executing deferred function")
//	}()
//
//	_, e := c.Client.Events.Create(context.Background(), &convoy_go.CreateEventRequest{
//		EndpointID: endpointId,
//		EventType:  event,
//		Data:       decodedMsg,
//	})
//
//	if e != nil {
//		logger.Error("[convoy][SendTrackConvertedEvent] - Error sending event", zap.Error(e), zap.String("endpoint_id", endpointId))
//		return e
//	}
//
//	return nil
//}
