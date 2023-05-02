package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/util"
	"os"
)

// CreateNewApp creates a new app for the developer and returns a uuid of the newly created app
func (d *NewDB) CreateNewApp(name, description, redirectURL, webhookURL, publicKey, developerId, secretKey, verifySecret, orgID string) ([]byte, error) {
	log.Printf("[db][CreateNewApp] developer -  creating new app: %s\n", name)

	log.Printf("[db][CreateNewApp] developer ==> ==> ==> ==> incoming data is %s %s %s %s %s %s %s %s %s\n", name, description, redirectURL, webhookURL, publicKey, developerId, secretKey, verifySecret, orgID)
	// create a new app
	uid := uuid.NewString()
	_, err := d.DB.Exec(queries.CreateNewApp, uid,
		name, description, redirectURL, webhookURL, publicKey,
		developerId, secretKey, verifySecret, orgID)
	if err != nil {
		log.Printf("[db][CreateNewApp] developer -  error: could not create new developer app: %v\n", err)
		return nil, err
	}
	//
	//// update the app redirect
	//_, err = d.DB.Exec(queries.UpdateAppRedirect, redirectURL, uid)
	//if err != nil {
	//	log.Printf("[db][CreateNewApp] developer -  error: could not update app redirect url with integrattion URL: %v\n", err)
	//	return nil, err
	//}
	log.Printf("[db][CreateNewApp] developer -  new app created: %s\n", name)
	return []byte(uid), nil
}

// UpdateIntegrationCredentials updates the integration credentials for an app. this is the app id and secret for the platform
func (d *NewDB) UpdateIntegrationCredentials(credentials []byte, appId, redirectURL, platform string) error {
	log.Printf("[db][UpdateIntegrationCredentials] developer -  updating integration credentials for app: %s\n", appId)
	// create a new app
	_, err := d.DB.Exec(queries.UpdateAppIntegrationCredentials, credentials, appId, platform, redirectURL)
	if err != nil {
		log.Printf("[db][UpdateIntegrationCredentials] developer -  error: could not update integration credentials for app: %v\n", err)
		return err
	}
	log.Printf("[db][UpdateIntegrationCredentials] developer -  integration credentials updated for app: %s\n", appId)
	return nil
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

// FetchAppByAppIdWithoutDevId fetches an app using the appId
func (d *NewDB) FetchAppByAppIdWithoutDevId(appId string) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchAppByAppIdWithoutDevId] developer -  fetching app by app id: %s\n", appId)
	var app blueprint.DeveloperApp
	log.Printf("running query: %s", queries.FetchAppByAppIDWithoutDev)
	err := d.DB.QueryRowx(queries.FetchAppByAppIDWithoutDev, appId).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchAppByAppIdWithoutDevId] developer -  error: could not fetch app by app id: %v\n", err)
		return nil, err
	}

	log.Printf("[db][FetchAppByAppId] developer -  app fetched: %s\n", app.Name)
	return &app, nil
}

// FetchAppByPublicKey fetches an app using the publicKey
func (d *NewDB) FetchAppByPublicKey(pubKey, developer string) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchAppByAppId] developer -  fetching app by public key: %s\n", pubKey)
	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByPubKey, pubKey, developer).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchAppByAppId] developer -  error: could not fetch app by public id: %v\n", err)
		return nil, err
	}

	log.Printf("[db][FetchAppByAppId] developer -  app fetched by publicKey: %s\n", app.Name)
	return &app, nil
}

func (d *NewDB) FetchAppByPublicKeyWithoutDevId(pubKey string) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchAppByAppIdWithoutDevId] developer -  fetching app by public key: %s\n", pubKey)
	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByPubKeyWithoutDev, pubKey).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchAppByAppIdWithoutDevId] developer -  error: could not fetch app by public id: %v\n", err)
		return nil, err
	}

	log.Printf("[db][FetchAppByAppIdWithoutDevId] developer -  app fetched by publicKey: %s\n", app.Name)
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
func (d *NewDB) UpdateApp(appId, platform, developer string, app blueprint.UpdateDeveloperAppData) error {
	log.Printf("[db][UpdateApp] developer -  updating app: %s\n", appId)

	log.Printf("[db][UpdateApp] developer -  App ID is %s, developer %s is trying to update app %s credentials\n", appId, developer, platform)
	// fetch the app to check if the integration credentials are already set
	devApp, err := d.FetchAppByAppId(appId)
	if err != nil {
		log.Printf("[db][UpdateApp] developer -  error: could not update app: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[db][UpdateApp] developer - App does not exist %s does not exist for developer %s\n", appId, developer)
			return sql.ErrNoRows
		}
		return err
	}

	var existingCredentials blueprint.IntegrationCredentials
	var outByte []byte

	switch platform {
	case applemusic.IDENTIFIER:
		outByte = devApp.AppleMusicCredentials
	case spotify.IDENTIFIER:
		outByte = devApp.SpotifyCredentials
	case deezer.IDENTIFIER:
		outByte = devApp.DeezerCredentials
	case tidal.IDENTIFIER:
		outByte = devApp.TidalCredentials
	}

	if string(outByte) != "" {
		log.Printf("[db][UpdateApp] developer  - No integration credentials found for app for platform %s %s\n", platform, appId)
		// decrypt the credentials
		decryptedData, err := util.Decrypt(outByte, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[db][UpdateApp] developer -  error: could not update app. Could not decrypt existing credentials for platform %s: %v\n", err, platform)
			return err
		}

		err = json.Unmarshal(decryptedData, &existingCredentials)
		if err != nil {
			log.Printf("[db][UpdateApp] developer -  error: could not update app. Could not deserialize existing credentials for platform %s: %v\n", err, platform)
			return err
		}
	}

	if app.IntegrationAppID != "" {
		existingCredentials.AppID = app.IntegrationAppID
	}
	if app.IntegrationAppSecret != "" {
		existingCredentials.AppSecret = app.IntegrationAppSecret
	}

	integrationCredentials := blueprint.IntegrationCredentials{
		AppID:     existingCredentials.AppID,
		AppSecret: existingCredentials.AppSecret,
		// FIXME: seems this is not needed to be stored in the credentials since its ([:platform]_credential) named column in the db
		Platform: app.IntegrationPlatform,
	}

	credentials, err := json.Marshal(&integrationCredentials)
	if err != nil {
		log.Printf("[db][UpdateApp] developer -  error: could not update app: could not serialize the integration credentials %v\n", err)
		return err
	}

	encryptedData, err := util.Encrypt(credentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("[db][UpdateApp] developer -  error: could not update app: could not encrypt the credentials %v\n", err)
		return err
	}
	log.Printf("Running update query: %s for app: %s", queries.UpdateApp, appId)
	_, err = d.DB.Exec(queries.UpdateApp,
		app.Description,
		app.Name,
		app.IntegrationRedirectURL,
		app.WebhookURL,
		appId,
		developer,
		encryptedData,
		platform)
	if err != nil {
		log.Printf("[db][UpdateApp] developer -  error: could not update app: %v\n", err)
		return err
	}

	log.Printf("[db][UpdateApp] developer -  app updated: %s\n", appId)
	return nil
}

// DeleteApp deletes an app
func (d *NewDB) DeleteApp(appId, developer string) error {
	log.Printf("[db][DeleteApp] developer -  deleting app: %s\n", appId)
	_, err := d.DB.Exec(queries.DeleteApp, appId, developer)
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
	var dev blueprint.User
	err := d.DB.QueryRowx(queries.FetchAuthorizedAppDeveloperByPublicKey, publicKey).StructScan(&dev)
	if err != nil {
		log.Printf("[db][FetchAuthorizedDeveloperApp] developer -  error: could not fetch authorized developer app: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[db][FetchAuthorizedDeveloperApp] developer - App does not exist%v\n", err)
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &dev, nil
}

// DisableApp sets an app's authorized state to false.
func (d *NewDB) DisableApp(appId, developer string) error {
	log.Printf("[db][DisableApp] developer -  disabling app: %s\n", appId)
	_, err := d.DB.Exec(queries.DisableApp, appId, developer)
	if err != nil {
		log.Printf("[db][DisableApp] developer -  error: could not disable app: %v\n", err)
		return err
	}

	log.Printf("[db][DisableApp] developer -  app disabled: %s\n", appId)
	return nil
}

// EnableApp sets an app's authorized state to true
func (d *NewDB) EnableApp(appId, developer string) error {
	log.Printf("[db][EnableApp] developer -  enabling app: %s\n", appId)
	_, err := d.DB.Exec(queries.EnableApp, appId, developer)
	if err != nil {
		log.Printf("[db][EnableApp] developer -  error: could not enable app: %v\n", err)
		return err
	}

	log.Printf("[db][EnableApp] developer -  app enabled: %s\n", appId)
	return nil
}

// FetchAppKeys fetches keys associated with an app. The fetched keys are public and secret keys
func (d *NewDB) FetchAppKeys(appId, developer string) (*blueprint.AppKeys, error) {
	log.Printf("[db][FetchAppKeys] developer -  fetching app keys: %s\n", appId)
	keys := blueprint.AppKeys{}
	err := d.DB.QueryRowx(queries.FetchAppKeysByID, appId, developer).StructScan(&keys)
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
func (d *NewDB) UpdateAppKeys(publicKey, secretKey, verifySecret, appId, deezerState string) error {
	log.Printf("[db][UpdateAppKeys] developer - updating app keys: %s\n", publicKey)
	_, err := d.DB.Exec(queries.UpdateAppKeys, publicKey, secretKey, verifySecret, deezerState, appId)
	if err != nil {
		log.Printf("[db][UpdateAppKeys] developer - error: could not update app keys: %v\n", err)
		return err
	}
	log.Printf("[db][UpdateAppKeys] developer - app keys updated: %s\n", publicKey)
	return nil
}

// FetchAppByDeezerState finds an app by its deezer state
func (d *NewDB) FetchAppByDeezerState(state string) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchDeezerAppByState] developer - fetching deezer app by state: %s\n", state)
	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByDeezerState, state).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchDeezerAppByState] developer - error: could not fetch deezer app by state: %v\n", err)
		return nil, err
	}
	return &app, nil
}

func (d *NewDB) UpdateUserAppScopes(userAppID, userID, platform, app string, scopes []string) error {
	log.Printf("[db][UpdateUserAppScopes] developer - updating user app scopes: %s\n", scopes)
	_, err := d.DB.Exec(queries.UpdateUserAppScopes, scopes, userAppID, userID, platform, app)
	if err != nil {
		log.Printf("[db][UpdateUserAppScopes] developer - error: could not update user app scopes: %v\n", err)
		return err
	}
	log.Printf("[db][UpdateUserAppScopes] developer - user app scopes updated: %s\n", scopes)
	return nil
}
