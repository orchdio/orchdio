package db

import (
	"github.com/google/uuid"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
)

// CreateNewApp creates a new app for the developer and returns a uuid of the newly created app
func (d *NewDB) CreateNewApp(name, description, redirectURL, webhookURL string) ([]byte, error) {
	log.Printf("[db][CreateNewApp] developer -  creating new app: %s\n", name)
	// create a new app
	uid := uuid.NewString()
	_, err := d.DB.Exec(queries.CreateNewApp, uid, name, description, redirectURL, webhookURL)
	if err != nil {
		log.Printf("[db][CreateNewApp] developer -  error: could not create new developer app: %v\n", err)
		return nil, err
	}
	log.Printf("[db][CreateNewApp] developer -  new app created: %s\n", name)
	return []byte(uid), nil
}

func (d *NewDB) FetchAppByAppId(appId string) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchAppByAppId] developer -  fetching app by app id: %s\n", appId)
	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByAppID, appId).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchAppByAppId] developer -  error: could not fetch app by app id: %v\n", err)
		return nil, err
	}

	log.Printf("[db][FetchAppByAppId] developer -  app fetched: %s\n", app.Name)
	return &app, nil
}
