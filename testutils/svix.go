package testutils

import (
	"log"

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
