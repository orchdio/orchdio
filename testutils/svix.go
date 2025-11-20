package testutils

import (
	"log"
	"orchdio/blueprint"

	svix "github.com/svix/svix-webhooks/go"
)

type MockSvix struct{}

func (m *MockSvix) CreateApp(name, id string) (*svix.ApplicationOut, *svix.AppPortalAccessOut, error) {
	log.Println("Mocked webhook CreateApp...")

	return &svix.ApplicationOut{
		Id: "mocked-webhook-id",
	}, nil, nil
}

func (m *MockSvix) CreateEndpoint(appId, uid, endpoint string) (*svix.EndpointOut, error) {
	log.Println("Mocked webhook CreateApp")

	return nil, nil
}

func (m *MockSvix) SendEvent(appId, eventType string, payload interface{}) (*svix.MessageOut, error) {
	log.Println("Mocked send event")
	return nil, nil
}

func (m *MockSvix) SendPlaylistMetadataEvent(info *blueprint.LinkInfo, result *blueprint.PlaylistConversionEventMetadata) bool {
	log.Println("Mocked Send playlist Metadata event")
	return false
}

func (m *MockSvix) SendTrackEvent(appId string, out *blueprint.PlaylistConversionEventTrack) bool {
	log.Println("Mocked send track event")
	return false
}

func (m *MockSvix) GetEndpoint(appId, endpoint string) (*svix.EndpointOut, error) {
	log.Println("Mocked Get endpoint event")
	return nil, nil
}

func (m *MockSvix) UpdateEndpoint(appId, endpointId, endpoint string) (*svix.EndpointOut, error) {
	log.Println("Mocked UpdateEndpoint")
	return nil, nil
}
