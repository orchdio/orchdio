package db

import (
	"database/sql"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
)

type NewDB struct {
	DB *sqlx.DB
}

type ApiKey struct {
	ID      int       `json:"id"`
	Key     uuid.UUID `json:"key"`
	User    uuid.UUID `json:"user"`
	Revoked bool      `json:"revoked"`
}

// FindUserByEmail finds a user by their email
func (d *NewDB) FindUserByEmail(email string) (*blueprint.User, error) {
	result := d.DB.QueryRowx(queries.FindUserByEmail, email)
	user := &blueprint.User{}

	err := result.StructScan(user)
	if err != nil {
		log.Printf("[controller][db] error scanning row result. %v\n", err)
		return nil, err
	}
	return user, nil
}

// FetchUserApikey fetches the user api key
func (d *NewDB) FetchUserApikey(uid uuid.UUID) (*ApiKey, error) {
	result := d.DB.QueryRowx(queries.FetchUserApiKey, uid)
	apiKey := &ApiKey{}

	err := result.StructScan(apiKey)

	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[controller][user][FetchUserApiKey] error - error scanning row. Something went wrong and this is not an expected error. %v\n", err)
			return nil, err
		}
		return nil, nil
	}
	return apiKey, nil
}

// RevokeApiKey sets the revoked column to true
func (d *NewDB) RevokeApiKey(key, user string) error {
	log.Printf("[db][RevokeApiKey] Running query %s %s %s\n", queries.RevokeApiKey, key, user)
	_, err := d.DB.Exec(queries.RevokeApiKey, key, user)
	if err != nil {
		log.Printf("[db][RevokeApiKey] error executing query %s.\n %v\n %s, %s \n", queries.RevokeApiKey, err, key, user)
		return err
	}
	log.Printf("[db][RevokeApiKey] Ran query %s\n", queries.RevokeApiKey)
	return nil
}

// UnRevokeApiKey sets the revoked column to true
func (d *NewDB) UnRevokeApiKey(key, user string) error {
	log.Printf("[db][UnRevokeApiKey] Running query %s %s %s\n", queries.UnRevokeApiKey, key, user)
	_, err := d.DB.Exec(queries.UnRevokeApiKey, key, user)
	if err != nil {
		log.Printf("[db][UnRevokeApiKey] error executing query %s.\n %v\n %s, %s\n", queries.RevokeApiKey, err, key, user)
		return err
	}
	log.Printf("[db][UnRevokeApiKey] Ran query %s\n", queries.UnRevokeApiKey)
	return nil
}

// DeleteApiKey deletes a user's api key
func (d *NewDB) DeleteApiKey(key, user string) ([]byte, error) {
	log.Printf("[db][DeleteKey] Ran Query: %s\n", queries.DeleteApiKey)
	result := d.DB.QueryRowx(queries.DeleteApiKey, key, user)
	if result == nil {
		log.Printf("[db][DeleteApikey] could not delete key. Seems there is no row to delete\n")
		return nil, sql.ErrNoRows
	}

	deleteRes := struct {
		Key string
	}{}

	scanErr := result.StructScan(&deleteRes)
	if scanErr != nil {
		log.Printf("[db][DeleteApiKey] - could not scan query result %v\n", scanErr)
		return nil, scanErr
	}

	log.Printf("[db][DeleteApiKey] - Deleted apiKey")
	return nil, nil
}

// FetchWebhook fetches the webhook for a user
func (d *NewDB) FetchWebhook(user string) ([]byte, error) {
	log.Printf("[db][FetchWebhook] fetching webhook for user %s\n. Running query: %s\n", user, queries.FetchUserWebhook)
	result := d.DB.QueryRowx(queries.FetchUserWebhook, user)

	if result.Err() != nil {
		log.Printf("[db][FetchWebhook] error fetching webhook for user %s\n", user)
		return nil, result.Err()
	}

	webhook := blueprint.WebhookUrl{}

	scanErr := result.StructScan(&webhook)
	if scanErr != nil {
		if scanErr == sql.ErrNoRows {
			log.Printf("[db][FetchWebhook] no webhook found for user %s\n", user)
			return nil, sql.ErrNoRows
		}
		log.Printf("[db][FetchWebhook] error scanning row result. %v\n", scanErr)
		return nil, scanErr
	}

	webhookUrl := []byte(webhook.Url)

	log.Printf("[db][FetchWebhook] fetched webhook for user %s\n", user)
	return webhookUrl, nil
}

// CreateUserWebhook creates a webhook for a user
func (d *NewDB) CreateUserWebhook(user, url string) error {
	// first fetch the user's webhook
	_, err := d.FetchWebhook(user)

	if err == nil {
		log.Printf("[db][CreateUserWebhook] user %s already has a webhook.\n", user)
		return blueprint.EALREADY_EXISTS
	}
	// TODO: handle more errors FetchWebhook can return

	log.Printf("[db][CreateUserWebhook] creating webhook for user %s\n. Running query: %s\n", user, queries.CreateWebhook)
	_, execErr := d.DB.Exec(queries.CreateWebhook, url, user)

	if execErr != nil {
		log.Printf("[db][CreateUserWebhook] error creating webhook for user %s. %v\n", user, execErr)
		return execErr
	}

	log.Printf("[db][CreateUserWebhook] created webhook for user %s\n", user)
	return nil
}

// FetchUserWithApiKey fetches a user with an api key
func (d *NewDB) FetchUserWithApiKey(key string) (*blueprint.User, error) {
	log.Printf("[db][FetchUserWithApiKey] Running query %s %s\n", queries.FetchUserWithApiKey, key)
	result := d.DB.QueryRowx(queries.FetchUserWithApiKey, key)

	if result == nil {
		log.Printf("[db][FetchUserWithApiKey] no user found with api key %s\n", key)
		return nil, sql.ErrNoRows
	}
	log.Printf("[db][FetchUserWithApiKey] Ran query %s\n", queries.FetchUserWithApiKey)
	usr := blueprint.User{}
	scanErr := result.StructScan(&usr)
	if scanErr != nil {
		log.Printf("[db][FetchUserWithApiKey] error scanning row result. %v\n", scanErr)
		return nil, scanErr
	}
	log.Printf("[db][FetchUserWithApiKey] fetched user %s\n", usr.Username)
	return &usr, nil
}

// UpdateUserWebhook updates a user's webhook
func (d *NewDB) UpdateUserWebhook(user, url string) error {
	log.Printf("[db][UpdateUserWebhook] Running query %s with '%s', '%s' \n", queries.UpdateUserWebhook, user, url)
	_, execErr := d.DB.Exec(queries.UpdateUserWebhook, url, user)
	if execErr != nil {
		log.Printf("[db][UpdateUserWebhook] error updating user webhook. %v\n", execErr)
		return execErr
	}
	log.Printf("[db][UpdateUserWebhook] updated user webhook\n")
	return nil
}

// DeleteUserWebhook deletes a user's webhook
func (d *NewDB) DeleteUserWebhook(user string) error {
	log.Printf("[db][DeleteUserWebhook] Running query %s with '%s'\n", queries.DeleteUserWebhook, user)
	_, execErr := d.DB.Exec(queries.DeleteUserWebhook, user)
	if execErr != nil {
		log.Printf("[db][DeleteUserWebhook] error deleting user webhook. %v\n", execErr)
		return execErr
	}
	log.Printf("[db][DeleteUserWebhook] deleted user webhook\n")
	return nil
}
