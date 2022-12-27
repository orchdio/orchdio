package db

import (
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
)

// CreateNewApp creates a new app for the developer and returns a uuid of the newly created app
func (d *NewDB) CreateNewApp(name, description, redirectURL, webhookURL, publicKey, developerId string, secretKey []byte) ([]byte, error) {
	log.Printf("[db][CreateNewApp] developer -  creating new app: %s\n", name)
	// create a new app
	uid := uuid.NewString()
	_, err := d.DB.Exec(queries.CreateNewApp, uid, name, description, redirectURL, webhookURL, publicKey, developerId, secretKey)
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

func (d *NewDB) FetchAppByPublicKey(pubKey string) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchAppByAppId] developer -  fetching app by public key: %s\n", pubKey)
	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByPubKey, pubKey).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchAppByAppId] developer -  error: could not fetch app by public id: %v\n", err)
		return nil, err
	}

	log.Printf("[db][FetchAppByAppId] developer -  app fetched by publicKey: %s\n", app.Name)
	return &app, nil
}

// FetchAppBySecretKey fetches an app by its secret key, the secret key is the api key, as its called for clients. In the backend, we simply call it secret key (much more nuanced)
func (d *NewDB) FetchAppBySecretKey(secretKey []byte) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchAppByDeveloperId] developer -  fetching apps by developer id:\n")
	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppBySecretKey, secretKey).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchAppByDeveloperId] developer -  error: could not fetch apps by developer id: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[db][FetchAppByDeveloperId] developer - App does not exist%v\n", err)
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	log.Printf("[db][FetchAppByDeveloperId] developer -  apps fetched: %s\n", app.Name)
	return &app, nil
}

func (d *NewDB) UpdateApp(appId string, app blueprint.UpdateDeveloperAppData) error {
	log.Printf("[db][UpdateApp] developer -  updating app: %s\n", appId)
	_, err := d.DB.Exec(queries.UpdateApp, app.Name, app.Description, app.RedirectURL, app.WebhookURL)
	if err != nil {
		log.Printf("[db][UpdateApp] developer -  error: could not update app: %v\n", err)
		return err
	}

	log.Printf("[db][UpdateApp] developer -  app updated: %s\n", appId)
	return nil
}

func (d *NewDB) DeleteApp(appId string) error {
	log.Printf("[db][DeleteApp] developer -  deleting app: %s\n", appId)
	_, err := d.DB.Exec(queries.DeleteApp, appId)
	if err != nil {
		log.Printf("[db][DeleteApp] developer -  error: could not delete app: %v\n", err)
		return err
	}

	log.Printf("[db][DeleteApp] developer -  app deleted: %s\n", appId)
	return nil
}
