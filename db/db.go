package db

import (
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
)

// NewDB represents a new DB layer struct for performing DB related operations
type NewDB struct {
	DB *sqlx.DB
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
func (d *NewDB) FetchUserApikey(uid uuid.UUID) (*blueprint.ApiKey, error) {
	result := d.DB.QueryRowx(queries.FetchUserApiKey, uid)
	apiKey := &blueprint.ApiKey{}

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

	webhook := blueprint.Webhook{}

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

// CreateOrUpdateTask creates or updates a task and returns the id of the task or an error
func (d *NewDB) CreateOrUpdateTask(uid, user, entity_id string) ([]byte, error) {
	log.Printf("[db][CreateOrUpdateNewTask] Running query %s with '%s', '%s', '%s'\n", queries.CreateOrUpdateTask, uid, user, entity_id)
	r := d.DB.QueryRowx(queries.CreateOrUpdateTask, uid, user, entity_id)
	var res string
	execErr := r.Scan(&res)
	if execErr != nil {
		log.Printf("[db][CreateOrUpdateNewTask] error creating or updating new task. %v\n", execErr)
		return nil, execErr
	}
	log.Printf("[db][CreateOrUpdateNewTask] created or updated new task\n")
	return []byte(res), nil
}

// UpdateTaskStatus updates a task's status and returns an error
func (d *NewDB) UpdateTaskStatus(uid, status string) error {
	log.Printf("[db][UpdateTaskStatus] Running query %s with '%s'\n", queries.UpdateTaskStatus, status)
	_, execErr := d.DB.Exec(queries.UpdateTaskStatus, uid, status)
	if execErr != nil {
		log.Printf("[db][UpdateTaskStatus] error updating task status. %v\n", execErr)
		return execErr
	}
	log.Printf("[db][UpdateTaskStatus] updated task status\n")
	return nil
}

// UpdateTask updates a task and returns the result of the task or an error
func (d *NewDB) UpdateTask(uid, data string) (*blueprint.PlaylistConversion, error) {
	log.Printf("[db][UpdateTask] Running query %s with '%s'\n", queries.UpdateTask, uid)
	r := d.DB.QueryRowx(queries.UpdateTask, uid, data)
	//var res blueprint.PlaylistConversion
	var res string
	execErr := r.Scan(&res)

	if execErr != nil {
		log.Printf("[db][UpdateTask] error updating task. %v\n", execErr)
		return nil, execErr
	}

	// deserialize into a playlist conversion
	var pc blueprint.PlaylistConversion
	err := json.Unmarshal([]byte(res), &pc)
	if err != nil {
		log.Printf("[db][UpdateTask] error deserializing task. %v\n", err)
		return nil, err
	}
	return &pc, nil
}

// FetchTask fetches a task and returns the task or an error
func (d *NewDB) FetchTask(uid string) (*blueprint.TaskRecord, error) {
	log.Printf("[db][FetchTask] Running query %s with '%s'\n", queries.FetchTask, uid)
	r := d.DB.QueryRowx(queries.FetchTask, uid)
	//var res blueprint.PlaylistConversion
	var res blueprint.TaskRecord
	err := r.StructScan(&res)

	// deserialize into a playlist conversion
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[db][FetchTask] no task found with uid %s\n", uid)
			return nil, sql.ErrNoRows
		}
		log.Printf("[db][FetchTask] error deserializing task. %v\n", err)
		return nil, err
	}
	return &res, nil
}

// DeleteTask deletes a task
func (d *NewDB) DeleteTask(uid string) error {
	log.Printf("[db][DeleteTask] Running query %s with '%s'\n", queries.DeleteTask, uid)
	_, execErr := d.DB.Exec(queries.DeleteTask, uid)
	if execErr != nil {
		log.Printf("[db][DeleteTask] error deleting task. %v\n", execErr)
		return execErr
	}
	log.Printf("[db][DeleteTask] deleted task\n")
	return nil
}
