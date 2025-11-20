package services

import (
	"database/sql"
	"errors"
	"log"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/samber/lo"
)

type UserDevApp struct {
	DB *sqlx.DB
}

func NewUserDevAppController(db *sqlx.DB) *UserDevApp {
	return &UserDevApp{DB: db}
}

func (u *UserDevApp) CreateOrUpdateUserApp(body *blueprint.CreateNewUserAppData) ([]byte, error) {
	log.Print("[controllers][developer][user_app] - creating user app")
	platforms := []string{applemusic.IDENTIFIER, deezer.IDENTIFIER, tidal.IDENTIFIER, spotify.IDENTIFIER}

	var userApp blueprint.UserApp
	database := db.NewDB{DB: u.DB}

	var userAppID []byte
	uniqueID := uuid.NewString()

	// check if the platform is valid
	if !lo.Contains(platforms, body.Platform) {
		log.Printf("[controllers][developer][user_app] - invalid platform: %v", body.Platform)
		return nil, blueprint.ErrInvalidPlatform
	}

	// fetch the user app with the platform. if it exists, we do not want to create. we are checking
	// in code instead of the database constraint because it seems easier to worth it doing from here
	// For example, a user may auth multiple spotify apps and we want to store them all but they may differ
	// by only the dev app id

	exRow := database.DB.QueryRowx(queries.FetchUserAppByPlatform, body.Platform, body.User, body.App)
	exRowErr := exRow.StructScan(&userApp)
	if exRowErr != nil && !errors.Is(exRowErr, sql.ErrNoRows) {
		log.Printf("[controllers][developer][user_app] - error fetching existing user app on platform %s: %v", body.Platform, exRowErr)
		return nil, exRowErr
	}

	if errors.Is(exRowErr, sql.ErrNoRows) {
		log.Printf("[controllers][developer][user_app] - user app does not exist for platform %s", body.Platform)
		// it means the user app does not exist as we check above if the error is not sql.ErrNoRows
		row := database.DB.QueryRowx(queries.CreateUserApp, uniqueID, body.RefreshToken,
			pq.Array(body.Scopes), body.User, body.Platform, body.App, body.ExpiresIn, body.AccessToken)
		err := row.Scan(&userAppID)
		if err != nil {
			log.Printf("[controllers][developer][user_app] - error creating user app: %v", err)
			return nil, err
		}
		log.Printf("[controllers][developer][user_app] - created user app %s for platform %s", userAppID, body.Platform)
		newAppUUID, err := uuid.Parse(uniqueID)
		if err != nil {
			log.Printf("[controllers][developer][user_app] - error parsing user app uuid: %v", err)
			return nil, err
		}
		userApp.UUID = newAppUUID
	}
	// hack?: the query will return a row with all the fields as their default values. in this case, the UUID
	// will be 00000000-0000-0000-0000-000000000000. the error returned is errNoRows. so we check if the UUID
	// is the default value and if it is, we know that the user app does not exist and we can create it

	if userApp.UUID.String() != "00000000-0000-0000-0000-000000000000" {
		log.Printf("[controllers][developer][user_app] - updating user %s app and scopes", body.Platform)
		var appId []byte
		row := database.DB.QueryRowx(queries.UpdateUserAppTokensAndScopes,
			body.RefreshToken, strings.Join(body.Scopes, ", "), body.App, body.User, body.Platform, userApp.UUID.String(), body.ExpiresIn, body.AccessToken)
		uErr := row.Scan(&appId)
		if uErr != nil {
			log.Printf("[controllers][developer][user_app] - error updating user app tokens and scopes: %v", uErr)
			return nil, uErr
		}
		log.Printf("[controllers][developer][user_app] - updated user app %s for platform %s", userApp.UUID.String(), body.Platform)
		return appId, nil
	}

	return userAppID, nil
}
