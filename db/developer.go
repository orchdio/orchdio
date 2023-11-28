package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
	logger2 "orchdio/logger"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/util"
	"os"
)

// CreateNewApp creates a new app for the developer and returns a uuid of the newly created app
func (d *NewDB) CreateNewApp(name, description, redirectURL, webhookURL, publicKey, developerId, secretKey, verifySecret, orgID, deezerState string) ([]byte, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}
	d.Logger.Info("[db][CreateNewApp] developer -  creating new app", zap.String("name", name), zap.String("developerId", developerId))

	// create a new app
	uid := uuid.NewString()
	_, err := d.DB.Exec(queries.CreateNewApp, uid,
		name, description, redirectURL, webhookURL, publicKey,
		developerId, secretKey, verifySecret, orgID, deezerState)
	if err != nil {
		d.Logger.Error("[db][CreateNewApp] developer -  error: could not create new developer app", zap.Error(err))
		return nil, err
	}

	d.Logger.Info("[db][CreateNewApp] developer -  new app created", zap.String("name", name), zap.String("developerId", developerId))
	return []byte(uid), nil
}

// UpdateIntegrationCredentials updates the integration credentials for an app. this is the app id and secret for the platform
func (d *NewDB) UpdateIntegrationCredentials(credentials []byte, appId, platform, redirectURL, webhookURL, endpointId string) error {
	log.Printf("[db][UpdateIntegrationCredentials] developer -  updating integration credentials for app: %s\n", appId)
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	// create a new app
	_, err := d.DB.Exec(queries.UpdateAppIntegrationCredentials, credentials, appId, platform, webhookURL, redirectURL, endpointId)
	if err != nil {
		d.Logger.Error("[db][UpdateIntegrationCredentials] developer -  error: could not update integration credentials for app", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][UpdateIntegrationCredentials] developer -  integration credentials updated for app", zap.String("app_id", appId))
	return nil
}

// FetchAppByAppId fetches an app using the appId
func (d *NewDB) FetchAppByAppId(appId string) (*blueprint.DeveloperApp, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByAppID, appId).StructScan(&app)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Warn("[db][FetchAppByAppId] developer - App does not exist", zap.String("app_id", appId))
			return nil, sql.ErrNoRows
		}
		d.Logger.Error("[db][FetchAppByAppId] developer -  error: could not fetch app by app id", zap.Error(err))
		return nil, err
	}
	d.Logger.Info("[db][FetchAppByAppId] developer -  app fetched", zap.String("app_id", appId))
	return &app, nil
}

// FetchAppByAppIdWithoutDevId fetches an app using the appId
func (d *NewDB) FetchAppByAppIdWithoutDevId(appId string) (*blueprint.DeveloperApp, error) {
	log.Printf("[db][FetchAppByAppIdWithoutDevId] developer -  fetching app by app id: %s\n", appId)
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByAppIDWithoutDev, appId).StructScan(&app)
	if err != nil {
		log.Printf("[db][FetchAppByAppIdWithoutDevId] developer -  error: could not fetch app by app id: %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Warn("[db][FetchAppByAppIdWithoutDevId] developer - App does not exist", zap.String("app_id", appId))
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	d.Logger.Info("[db][FetchAppByAppIdWithoutDevId] developer -  app fetched", zap.String("app_id", appId))
	return &app, nil
}

// FetchAppByPublicKey fetches an app using the publicKey
func (d *NewDB) FetchAppByPublicKey(pubKey, developer string) (*blueprint.DeveloperApp, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByPubKey, pubKey, developer).StructScan(&app)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[db][FetchAppByAppId] developer - App does not exist")
			d.Logger.Warn("[db][FetchAppByAppId] developer - App does not exist", zap.String("public_key", pubKey))
			return nil, sql.ErrNoRows
		}
		d.Logger.Error("[db][FetchAppByAppId] developer -  error: could not fetch app by public id", zap.Error(err), zap.String("public_key", pubKey))
		return nil, err
	}
	d.Logger.Info("[db][FetchAppByAppId] developer -  app fetched by publicKey", zap.String("public_key", pubKey))
	return &app, nil
}

func (d *NewDB) FetchAppByPublicKeyWithoutDevId(pubKey string) (*blueprint.DeveloperApp, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByPubKeyWithoutDev, pubKey).StructScan(&app)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Warn("[db][FetchAppByAppIdWithoutDevId] developer - App does not exist", zap.String("public_key", pubKey))
			return nil, sql.ErrNoRows
		}
		d.Logger.Error("[db][FetchAppByAppIdWithoutDevId] developer -  error: could not fetch app by public id", zap.Error(err), zap.String("public_key", pubKey))
		return nil, err
	}
	d.Logger.Info("[db][FetchAppByAppIdWithoutDevId] developer -  app fetched by publicKey", zap.String("public_key", pubKey))
	return &app, nil
}

// FetchAppBySecretKey fetches an app by its secret key, the secret key is the api key, as its called for clients. In the backend, we simply call it secret key (much more nuanced)
func (d *NewDB) FetchAppBySecretKey(secretKey []byte) (*blueprint.DeveloperApp, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppBySecretKey, secretKey).StructScan(&app)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Warn("[db][FetchAppByDeveloperId] developer - App does not exist")
			return nil, sql.ErrNoRows
		}
		d.Logger.Error("[db][FetchAppByDeveloperId] developer -  error: could not fetch apps by developer id", zap.Error(err))
		return nil, err
	}
	d.Logger.Info("[db][FetchAppByDeveloperId] developer -  apps fetched", zap.String("app_name", app.Name))
	return &app, nil
}

// UpdateApp updates an app with the passed data. It does an upsert and fields that want to be updated need to be passed.
func (d *NewDB) UpdateApp(appId, platform, developer string, app blueprint.UpdateDeveloperAppData) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	// fetch the app to check if the integration credentials are already set
	devApp, err := d.FetchAppByAppId(appId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Warn("[db][UpdateApp] developer - App does not exist", zap.String("app_id", appId), zap.String("developer_id", developer))
			return sql.ErrNoRows
		}
		d.Logger.Error("[db][UpdateApp] developer -  error: could not update app", zap.Error(err))
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
		d.Logger.Warn("[db][UpdateApp] developer  - No integration credentials found for app for platform", zap.String("platform", platform), zap.String("app_id", appId))
		// decrypt the credentials
		decryptedData, decErr := util.Decrypt(outByte, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			d.Logger.Error("[db][UpdateApp] developer -  error: could not update app. Could not decrypt existing credentials for platform", zap.Error(decErr), zap.String("platform", platform))
			return err
		}

		err = json.Unmarshal(decryptedData, &existingCredentials)
		if err != nil {
			d.Logger.Error("[db][UpdateApp] developer -  error: could not update app. Could not deserialize existing credentials for platform", zap.Error(err), zap.String("platform", platform))
			return err
		}
	}

	if app.IntegrationAppID != "" {
		existingCredentials.AppID = app.IntegrationAppID
	}
	if app.IntegrationAppSecret != "" {
		existingCredentials.AppSecret = app.IntegrationAppSecret
	}
	if app.IntegrationRefreshToken != "" {
		existingCredentials.AppRefreshToken = app.IntegrationRefreshToken
	}

	// if the platform is not tidal oor apple music, we should not have a refresh token,
	// so we abort if its so.
	// Apple music uses a credential called API_KEY which is a JWT, like TIDAL's developer refresh token. so we encode
	// both as RefreshToken but in apple music service, we refer to it as API_KEY
	if !lo.Contains([]string{applemusic.IDENTIFIER, tidal.IDENTIFIER}, platform) {
		if app.IntegrationRefreshToken != "" {
			d.Logger.Warn("[db][UpdateApp] developer -  warning: App has refreshtoken credentials but is not a platform that requires it. Only TIDAL and Apple Music do.", zap.String("platform", platform))
			return blueprint.EBADCREDENTIALS
		}
	}

	integrationCredentials := blueprint.IntegrationCredentials{
		AppID:     existingCredentials.AppID,
		AppSecret: existingCredentials.AppSecret,
		// FIXME: seems this is not needed to be stored in the credentials since its ([:platform]_credential) named column in the db
		Platform:        app.IntegrationPlatform,
		AppRefreshToken: existingCredentials.AppRefreshToken,
	}

	credentials, err := json.Marshal(&integrationCredentials)
	if err != nil {
		d.Logger.Error("[db][UpdateApp] developer -  error: could not update app: could not serialize the integration credentials", zap.Error(err), zap.String("platform", platform))
		return err
	}

	encryptedData, err := util.Encrypt(credentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		d.Logger.Error("[db][UpdateApp] developer -  error: could not update app: could not encrypt the credentials", zap.Error(err), zap.String("platform", platform))
		return err
	}
	_, err = d.DB.Exec(queries.UpdateApp,
		app.Description,
		app.Name,
		app.RedirectURL,
		app.WebhookURL,
		appId,
		developer,
		encryptedData,
		platform)
	if err != nil {
		d.Logger.Error("[db][UpdateApp] developer -  error: could not update app", zap.Error(err), zap.String("platform", platform), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][UpdateApp] developer -  app updated", zap.String("app_id", appId))
	return nil
}

// DeleteApp deletes an app
func (d *NewDB) DeleteApp(appId, developer string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.DeleteApp, appId, developer)
	if err != nil {
		d.Logger.Error("[db][DeleteApp] developer -  error: could not delete app", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][DeleteApp] developer -  app deleted", zap.String("app_id", appId))
	return nil
}

// FetchDeveloperAppWithSecretKey fetches a developer for an authorized app, meaning the app is active.
func (d *NewDB) FetchDeveloperAppWithSecretKey(secretKey string) (*blueprint.User, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	var developer blueprint.User
	err := d.DB.QueryRowx(queries.FetchAuthorizedAppDeveloperBySecretKey, secretKey).StructScan(&developer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Error("[db][FetchAuthorizedDeveloperApp] developer - App does not exist", zap.Error(err))
			return nil, sql.ErrNoRows
		}
		d.Logger.Error("[db][FetchAuthorizedDeveloperApp] developer -  error: could not fetch authorized developer app", zap.Error(err))
		return nil, err
	}

	return &developer, nil
}

// FetchDeveloperAppWithPublicKey fetches a developer for an authorized app, meaning the app is active.
func (d *NewDB) FetchDeveloperAppWithPublicKey(publicKey string) (*blueprint.User, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}
	var dev blueprint.User
	err := d.DB.QueryRowx(queries.FetchAuthorizedAppDeveloperByPublicKey, publicKey).StructScan(&dev)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.Logger.Warn("[db][FetchAuthorizedDeveloperApp] developer - App does not exist", zap.String("public_key", publicKey))
			return nil, sql.ErrNoRows
		}
		d.Logger.Error("[db][FetchAuthorizedDeveloperApp] developer -  error: could not fetch authorized developer app", zap.Error(err))
		return nil, err
	}
	return &dev, nil
}

// DisableApp sets an app's authorized state to false.
func (d *NewDB) DisableApp(appId, developer string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}
	_, err := d.DB.Exec(queries.DisableApp, appId, developer)
	if err != nil {
		d.Logger.Error("[db][DisableApp] developer -  error: could not disable app", zap.Error(err))
		return err
	}

	d.Logger.Info("[db][DisableApp] developer -  app disabled", zap.String("app_id", appId))
	return nil
}

// EnableApp sets an app's authorized state to true
func (d *NewDB) EnableApp(appId, developer string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.EnableApp, appId, developer)
	if err != nil {
		d.Logger.Error("[db][EnableApp] developer -  error: could not enable app", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][EnableApp] developer -  app enabled", zap.String("app_id", appId))
	return nil
}

// FetchAppKeys fetches keys associated with an app. The fetched keys are public and secret keys
func (d *NewDB) FetchAppKeys(appId, developer string) (*blueprint.AppKeys, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}
	keys := blueprint.AppKeys{}
	err := d.DB.QueryRowx(queries.FetchAppKeysByID, appId, developer).StructScan(&keys)
	if err != nil {
		d.Logger.Error("[db][FetchAppKeys] developer -  error: could not fetch app keys", zap.Error(err))
		return nil, err
	}
	return &keys, nil
}

// FetchApps fetches all the apps that belong to a developer.
func (d *NewDB) FetchApps(developerId, orgID string) (*[]blueprint.AppInfo, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	d.Logger.Info("[db][FetchAppKeys] developer - fetching apps that belong to developer and org", zap.String("developer_id", developerId), zap.String("org_id", orgID))
	var apps []blueprint.AppInfo
	rows, err := d.DB.Queryx(queries.FetchAppsByDeveloper, developerId, orgID)
	if err != nil {
		d.Logger.Error("[db][FetchAppKeys] developer - error: could not fetch apps that belong to developer", zap.Error(err))
		return nil, err
	}

	for rows.Next() {
		var app blueprint.DeveloperApp
		err = rows.StructScan(&app)
		if err != nil {
			d.Logger.Error("[db][FetchAppKeys] developer - error: could not fetch apps that belong to developer", zap.Error(err))
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
			DeezerState: app.DeezerState,
		}
		apps = append(apps, appInfo)
	}

	if len(apps) == 0 {
		d.Logger.Warn("[db][FetchAppKeys] developer - error: no apps found for developer", zap.String("developer_id", developerId))
		return nil, sql.ErrNoRows
	}
	return &apps, nil
}

// UpdateAppKeys updates the public and secret keys associated with an app. It also updates the verify secret key for webhook verification
func (d *NewDB) UpdateAppKeys(publicKey, secretKey, verifySecret, appId, deezerState string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.UpdateAppKeys, publicKey, secretKey, verifySecret, deezerState, appId)
	if err != nil {
		d.Logger.Error("[db][UpdateAppKeys] developer - error: could not update app keys", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Error("[db][UpdateAppKeys] developer - app keys updated", zap.String("app_id", appId))
	return nil
}

func (d *NewDB) RevokeSecretKey(appId, newSecret string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.RevokeSecretKey, appId, newSecret)
	if err != nil {
		d.Logger.Error("[db][RevokeSecretKey] developer - error: could not revoke secret key", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][RevokeSecretKey] developer - secret key revoked", zap.String("app_id", appId))
	return nil
}

func (d *NewDB) RevokeVerifySecret(appId, newVerifyToken string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}
	_, err := d.DB.Exec(queries.RevokeVerifySecret, appId, newVerifyToken)
	if err != nil {
		d.Logger.Error("[db][RevokeVerifySecret] developer - error: could not revoke verify secret", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	log.Printf("[db][RevokeVerifySecret] developer - verify secret revoked: %s\n", appId)
	return nil
}

func (d *NewDB) RevokeDeezerState(appId, newDeezerState string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.RevokeDeezerState, appId, newDeezerState)
	if err != nil {
		d.Logger.Error("[db][RevokeDeezerState] developer - error: could not revoke deezer state", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][RevokeDeezerState] developer - deezer state revoked", zap.String("app_id", appId))
	return nil
}

func (d *NewDB) RevokePublicKey(appId, newPublicKey string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.RevokePublicKey, appId, newPublicKey)
	if err != nil {
		d.Logger.Error("[db][RevokePublicKey] developer - error: could not revoke public key", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][RevokePublicKey] developer - public key revoked", zap.String("app_id", appId))
	return nil
}

// FetchAppByDeezerState finds an app by its deezer state
func (d *NewDB) FetchAppByDeezerState(state string) (*blueprint.DeveloperApp, error) {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	var app blueprint.DeveloperApp
	err := d.DB.QueryRowx(queries.FetchAppByDeezerState, state).StructScan(&app)
	if err != nil {
		d.Logger.Error("[db][FetchDeezerAppByState] developer - error: could not fetch deezer app by state", zap.Error(err))
		return nil, err
	}
	return &app, nil
}

func (d *NewDB) UpdateUserAppScopes(userAppID, userID, platform, app string, scopes []string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.UpdateUserAppScopes, scopes, userAppID, userID, platform, app)
	if err != nil {
		d.Logger.Error("[db][UpdateUserAppScopes] developer - error: could not update user app scopes", zap.Error(err), zap.String("app_id", app), zap.Any("scopes", scopes))
		return err
	}
	d.Logger.Info("[db][UpdateUserAppScopes] developer - user app scopes updated", zap.String("app_id", app), zap.Any("scopes", scopes))
	return nil
}

func (d *NewDB) DeletePlatformIntegrationCredentials(appId, platform, developerId string) error {
	if d.Logger == nil {
		d.Logger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.DeletePlatformIntegrationCredentials, appId, platform, developerId)
	if err != nil {
		d.Logger.Error("[db][DeletePlatformIntegrationCredentials] developer - error: could not delete platform integration credentials", zap.Error(err), zap.String("app_id", appId))
		return err
	}
	d.Logger.Info("[db][DeletePlatformIntegrationCredentials] developer - platform integration credentials deleted", zap.String("app_id", appId))
	return nil
}
