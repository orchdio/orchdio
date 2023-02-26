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
func (d *NewDB) CreateNewApp(name, description, redirectURL, webhookURL, publicKey, developerId, secretKey, verifySecret string) ([]byte, error) {
	log.Printf("[db][CreateNewApp] developer -  creating new app: %s\n", name)
	// create a new app
	uid := uuid.NewString()
	_, err := d.DB.Exec(queries.CreateNewApp, uid, name, description, redirectURL, webhookURL, publicKey, developerId, secretKey, verifySecret)
	if err != nil {
		log.Printf("[db][CreateNewApp] developer -  error: could not create new developer app: %v\n", err)
		return nil, err
	}
	log.Printf("[db][CreateNewApp] developer -  new app created: %s\n", name)
	return []byte(uid), nil
}

// FetchAppByAppId fetches an app using the appId
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

// FetchAppByPublicKey fetches an app using the publicKey
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

// UpdateApp updates an app with the passed data. It does an upsert and fields that want to be updated need to be passed.
func (d *NewDB) UpdateApp(appId string, app blueprint.UpdateDeveloperAppData) error {
	log.Printf("[db][UpdateApp] developer -  updating app: %s\n", appId)
	_, err := d.DB.Exec(queries.UpdateApp, app.Description, app.Name, app.RedirectURL, app.WebhookURL, appId)
	if err != nil {
		log.Printf("[db][UpdateApp] developer -  error: could not update app: %v\n", err)
		return err
	}

	log.Printf("[db][UpdateApp] developer -  app updated: %s\n", appId)
	return nil
}

// DeleteApp deletes an app
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

// FetchDeveloperAppWithSecretKey fetches a developer for an authorized app, meaning the app is active.
func (d *NewDB) FetchDeveloperAppWithSecretKey(secretKey string) (*blueprint.User, error) {
	log.Printf("[db][FetchAuthorizedDeveloperApp] developer -  fetching authorized developer app by secret key:\n")
	var developer blueprint.User
	err := d.DB.QueryRowx(queries.FetchAuthorizedAppDeveloperBySecretKey, secretKey).StructScan(&developer)
	if err != nil {
		log.Printf("[db][FetchAuthorizedDeveloperApp] developer -  error: could not fetch authorized developer app: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[db][FetchAuthorizedDeveloperApp][fetchbysecret] developer - App does not exist %v\n", err)
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	return &developer, nil
}

// FetchDeveloperAppWithPublicKey fetches a developer for an authorized app, meaning the app is active.
func (d *NewDB) FetchDeveloperAppWithPublicKey(publicKey string) (*blueprint.User, error) {
	log.Printf("[db][FetchAuthorizedDeveloperApp][fetchbypublickey] developer -  fetching authorized developer by pubkey:\n")
	var developer blueprint.User
	err := d.DB.QueryRowx(queries.FetchAuthorizedAppDeveloperByPublicKey, publicKey).StructScan(&developer)
	if err != nil {
		log.Printf("[db][FetchAuthorizedDeveloperApp] developer -  error: could not fetch authorized developer app: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[db][FetchAuthorizedDeveloperApp] developer - App does not exist%v\n", err)
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &developer, nil
}

// DisableApp sets an app's authorized state to false.
func (d *NewDB) DisableApp(appId string) error {
	log.Printf("[db][DisableApp] developer -  disabling app: %s\n", appId)
	_, err := d.DB.Exec(queries.DisableApp, appId)
	if err != nil {
		log.Printf("[db][DisableApp] developer -  error: could not disable app: %v\n", err)
		return err
	}

	log.Printf("[db][DisableApp] developer -  app disabled: %s\n", appId)
	return nil
}

// EnableApp sets an app's authorized state to true
func (d *NewDB) EnableApp(appId string) error {
	log.Printf("[db][EnableApp] developer -  enabling app: %s\n", appId)
	_, err := d.DB.Exec(queries.EnableApp, appId)
	if err != nil {
		log.Printf("[db][EnableApp] developer -  error: could not enable app: %v\n", err)
		return err
	}

	log.Printf("[db][EnableApp] developer -  app enabled: %s\n", appId)
	return nil
}

// FetchAppKeys fetches keys associated with an app. The fetched keys are public and secret keys
func (d *NewDB) FetchAppKeys(appId string) (*blueprint.AppKeys, error) {
	log.Printf("[db][FetchAppKeys] developer -  fetching app keys: %s\n", appId)
	keys := blueprint.AppKeys{}
	err := d.DB.QueryRowx(queries.FetchAppKeysByID, appId).StructScan(&keys)
	if err != nil {
		log.Printf("[db][FetchAppKeys] developer -  error: could not fetch app keys: %v\n", err)
		return nil, err
	}
	return &keys, nil
}

// FetchApps fetches all the apps that belong to a developer.
func (d *NewDB) FetchApps(developerId string) (*[]blueprint.AppInfo, error) {
	log.Printf("[db][FetchAppKeys] developer - fetching apps that belong to developer: %s\n", developerId)
	var apps []blueprint.AppInfo
	rows, err := d.DB.Queryx(queries.FetchAppsByDeveloper, developerId)
	if err != nil {
		log.Printf("[db][FetchAppKeys] developer - error: could not fetch apps that belong to developer: %v\n", err)
		return nil, err
	}

	for rows.Next() {
		var app blueprint.DeveloperApp
		err = rows.StructScan(&app)
		if err != nil {
			log.Printf("[db][FetchAppKeys] developer - error: could not fetch apps that belong to developer: %v\n", err)
			return nil, err
		}
		appInfo := blueprint.AppInfo{
			AppID:       app.UID.String(),
			Name:        app.Name,
			Description: app.Description,
			RedirectURL: app.RedirectURL,
			WebhookURL:  app.WebhookURL,
			PublicKey:   app.PublicKey.String(),
			Authorized:  app.Authorized,
		}
		apps = append(apps, appInfo)
	}

	log.Printf("[db][FetchAppKeys] developer - apps fetched: %s\n", developerId)
	return &apps, nil
}

// UpdateAppKeys updates the public and secret keys associated with an app. It also updates the verify secret key for webhook verification
func (d *NewDB) UpdateAppKeys(publicKey, secretKey, verifySecret, appId string) error {
	log.Printf("[db][UpdateAppKeys] developer - updating app keys: %s\n", publicKey)
	_, err := d.DB.Exec(queries.UpdateAppKeys, publicKey, secretKey, verifySecret, appId)
	if err != nil {
		log.Printf("[db][UpdateAppKeys] developer - error: could not update app keys: %v\n", err)
		return err
	}
	log.Printf("[db][UpdateAppKeys] developer - app keys updated: %s\n", publicKey)
	return nil
}
