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
func (d *NewDB) FindUserByEmail(email, platform string) (*blueprint.User, error) {
	result := d.DB.QueryRowx(queries.FindUserByEmail, email, platform)
	user := &blueprint.User{}

	err := result.StructScan(user)
	if err != nil {
		log.Printf("[controller][db] error scanning row result. %v\n", err)
		return nil, err
	}
	var userNames map[string]string
	err = json.Unmarshal(user.Usernames.([]byte), &userNames)

	if err != nil {
		log.Printf("[controller][db] error unmarshalling usernames. %v\n", err)
		return nil, err
	}
	user.Usernames = userNames
	return user, nil
}

// FindUserByUUID finds a user by their UUID
func (d *NewDB) FindUserByUUID(id string) (*blueprint.User, error) {
	result := d.DB.QueryRowx(queries.FindUserByUUID, id)
	user := &blueprint.User{}

	err := result.StructScan(user)
	if err != nil {
		log.Printf("[controller][db] error scanning row result. %v\n", err)
		return nil, err
	}
	var userNames map[string]string
	err = json.Unmarshal(user.Usernames.([]byte), &userNames)

	if err != nil {
		log.Printf("[controller][db] error unmarshalling usernames. %v\n", err)
		return nil, err
	}
	user.Usernames = userNames
	return user, nil
}

// FetchUserApikey fetches the user api key
func (d *NewDB) FetchUserApikey(email string) (*blueprint.ApiKey, error) {
	log.Printf("[db][FetchUserApikey] Running query %s with '%s'\n", queries.FetchUserApiKey, email)
	result := d.DB.QueryRowx(queries.FetchUserApiKey, email)
	apiKey := &blueprint.ApiKey{}

	err := result.StructScan(apiKey)

	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[controller][user][FetchUserApiKey] error - error scanning row. Something went wrong and this is not an expected error. %v\n", err)
			return nil, err
		}
		return nil, err
	}
	return apiKey, nil
}

// RevokeApiKey sets the revoked column to true
func (d *NewDB) RevokeApiKey(key string) error {
	log.Printf("[db][RevokeApiKey] Running query %s %s\n", queries.RevokeApiKey, key)
	_, err := d.DB.Exec(queries.RevokeApiKey, key)
	if err != nil {
		log.Printf("[db][RevokeApiKey] error executing query %s.\n %v\n %s\n", queries.RevokeApiKey, err, key)
		return err
	}
	log.Printf("[db][RevokeApiKey] Ran query %s\n", queries.RevokeApiKey)
	return nil
}

// UnRevokeApiKey sets the revoked column to true
func (d *NewDB) UnRevokeApiKey(key string) error {
	log.Printf("[db][UnRevokeApiKey] Running query %s %s\n", queries.UnRevokeApiKey, key)
	_, err := d.DB.Exec(queries.UnRevokeApiKey, key)
	if err != nil {
		log.Printf("[db][UnRevokeApiKey] error executing query %s.\n %v\n\n", queries.RevokeApiKey, err)
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
func (d *NewDB) FetchWebhook(user string) (*blueprint.Webhook, error) {
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

	log.Printf("[db][FetchWebhook] fetched webhook for user %s\n", user)
	return &webhook, nil
}

// CreateUserWebhook creates a webhook for a user
func (d *NewDB) CreateUserWebhook(user, url, verifyToken string) error {
	// first fetch the user's webhook
	_, err := d.FetchWebhook(user)
	uniqueID, _ := uuid.NewUUID()

	if err == nil {
		log.Printf("[db][CreateUserWebhook] user %s already has a webhook.\n", user)
		return blueprint.EALREADY_EXISTS
	}
	// TODO: handle more errors FetchWebhook can return

	log.Printf("[db][CreateUserWebhook] creating webhook for user %s\n. Running query: %s\n", user, queries.CreateWebhook)
	_, execErr := d.DB.Exec(queries.CreateWebhook, url, user, verifyToken, uniqueID.String())

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
	var userNames map[string]string
	usernames := json.Unmarshal(usr.Usernames.([]byte), &userNames)
	log.Printf("[db][FetchUserWithApiKey] fetched user '%v'\n", usernames)
	return &usr, nil
}

// UpdateUserWebhook updates a user's webhook
func (d *NewDB) UpdateUserWebhook(user, url, verifyToken string) error {
	log.Printf("[db][UpdateUserWebhook] Running query %s with '%s', '%s' \n", queries.UpdateUserWebhook, user, url)
	// temporary struct to deserialize the record update into.
	// not creating inside blueprint because its small and used here alone. if this changes, move to blueprint
	webhookUpdate := &struct {
		UUID uuid.UUID `json:"uuid" db:"uuid"`
	}{}

	updatedWH := d.DB.QueryRowx(queries.UpdateUserWebhook, url, user, verifyToken)
	execErr := updatedWH.StructScan(webhookUpdate)

	if execErr != nil {
		log.Printf("[db][UpdateUserWebhook] error updating user webhook. %v\n", execErr)
		return execErr
	}

	if webhookUpdate.UUID.String() == "" {
		log.Printf("[db][UpdateUserWebhook][error] no webhook to update for this user")
		return sql.ErrNoRows
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
func (d *NewDB) CreateOrUpdateTask(uid, shortid, user, entityId string) ([]byte, error) {
	log.Printf("[db][CreateOrUpdateNewTask] Running query %s with '%s', '%s', '%s'\n", queries.CreateOrUpdateTask, uid, user, entityId)
	r := d.DB.QueryRowx(queries.CreateOrUpdateTask, uid, shortid, user, entityId)
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

// UpdateTaskResult updates a task and returns the result of the task or an error
func (d *NewDB) UpdateTaskResult(uid, data string) (*blueprint.PlaylistConversion, error) {
	log.Printf("[db][UpdateTaskResult] Running query %s with '%s'\n", queries.UpdateTaskResult, uid)
	r := d.DB.QueryRowx(queries.UpdateTaskResult, uid, data)
	//var res blueprint.PlaylistConversion
	var res string
	execErr := r.Scan(&res)

	if execErr != nil {
		log.Printf("[db][UpdateTaskResult] error updating task. %v\n", execErr)
		return nil, execErr
	}

	// deserialize into a playlist conversion
	var pc blueprint.PlaylistConversion
	err := json.Unmarshal([]byte(res), &pc)
	if err != nil {
		log.Printf("[db][UpdateTaskResult] error deserializing task. %v\n", err)
		return nil, err
	}
	return &pc, nil
}

// FetchTask fetches a task and returns the task or an error
func (d *NewDB) FetchTask(uid string) (*blueprint.TaskRecord, error) {
	log.Printf("[db][FetchTask] Running query %s with '%s'\n", queries.FetchTask, uid)

	// currently, in the db we were fetching by taskid, but we also want to fetch by the shortid
	// so we check if the taskId is a valid uuid, if it is, we fetch by taskid, if not, we fetch by shortid
	_, err := uuid.Parse(uid)
	if err != nil {
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - not a valid uuid, fetching by shortid")
		log.Printf("[db][FetchTask] Running query %s with '%s'\n", queries.FetchTaskByShortID, uid)
		//var res blueprint.PlaylistConversion
		r := d.DB.QueryRowx(queries.FetchTaskByShortID, uid)

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

	r := d.DB.QueryRowx(queries.FetchTask, uid)
	//var res blueprint.PlaylistConversion
	var res blueprint.TaskRecord
	err = r.StructScan(&res)

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

// FetchFollowTask fetches a task that a developer already sends a request to add a subscriber to. A task is basically
// a job that runs at interval to check if the playlist has been updated. This method basically fetches this task. The "user"
// here is the developer.
func (d *NewDB) FetchFollowTask(entityId string) (*blueprint.FollowTask, error) {
	log.Printf("[db][FetchUserFollowedTasks] Running query '%s' with '%s'\n", queries.FetchFollowedTask, entityId)
	rows := d.DB.QueryRowx(queries.FetchFollowedTask, entityId)
	var res blueprint.FollowTask
	err := rows.StructScan(&res)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[db][FetchUserFollowedTasks] no follow found for entity %s\n", entityId)
			return nil, sql.ErrNoRows
		}
		log.Printf("[db][FetchUserFollowedTasks] error fetching user followed tasks. %v\n", err)
		return nil, err
	}
	return &res, nil
}

// FetchTaskByEntityIDAndType fetches task by entityId and taskType.
func (d *NewDB) FetchTaskByEntityIDAndType(entityId, taskType string) (*blueprint.FollowTask, error) {
	log.Printf("[db][FetchTaskByIDAndType] Running query %s with '%s', '%s'\n", queries.FetchTaskByEntityIdAndType, entityId, taskType)
	rows := d.DB.QueryRowx(queries.FetchTaskByEntityIdAndType, entityId, taskType)
	var res blueprint.FollowTask
	err := rows.StructScan(&res)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[db][FetchTaskByIDAndType] no tasks found for user %s\n", entityId)
			return nil, sql.ErrNoRows
		}
		log.Printf("[db][FetchTaskByIDAndType] error fetching user followed tasks. %v\n", err)
		return nil, err
	}
	return &res, nil
}

// CreateFollowTask creates a follow task if it does not exist and updates a task if it exists and the subscriber has been subscribed
func (d *NewDB) CreateFollowTask(developer, uid, entityId, entityURL string, subscribers interface{}) ([]byte, error) {
	log.Printf("[db][CreateFollowTask] Running query %s with '%s', '%s', '%s' \n", queries.CreateOrAddSubscriberFollow, uid, entityId, developer)
	r := d.DB.QueryRowx(queries.CreateOrAddSubscriberFollow, uid, developer, entityId, subscribers, entityURL)
	var res string
	err := r.Scan(&res)
	if err != nil {
		log.Printf("[db][CreateFollowTask] error creating follow task. %v\n", err)
		return nil, err
	}
	return []byte(res), nil
}

// CreateTrackTaskRecord creates a new task record for a track.
func (d *NewDB) CreateTrackTaskRecord(uid, shortId, entityId string, result []byte) ([]byte, error) {
	log.Printf("[db][CreateTrackTaskRecord] Running query %s with '%s', '%s', '%s' \n", queries.CreateNewTrackTaskRecord, uid, shortId, entityId)

	r := d.DB.QueryRowx(queries.CreateNewTrackTaskRecord, uid, shortId, entityId, string(result))
	var res string
	err := r.Scan(&res)
	if err != nil {
		log.Printf("[db][CreateTrackTaskRecord] error creating track task record. %v\n", err)
		return nil, err
	}
	log.Printf("[db][CreateTrackTaskRecord] created track task record.\n")
	return []byte(res), nil
}

func (d *NewDB) FetchFollowByEntityID(entityId string) (*blueprint.FollowTask, error) {
	log.Printf("[db][FetchFollowByEntityID] Running query '%s' with '%s'\n", queries.FetchFollowByEntityId, entityId)
	row := d.DB.QueryRowx(queries.FetchFollowByEntityId, entityId)
	var res blueprint.FollowTask
	err := row.StructScan(&res)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[db][FetchFollowByEntityID] no follow found for entity %s\n", entityId)
			return nil, sql.ErrNoRows
		}
		log.Printf("[db][FetchFollowByEntityID] error fetching user followed tasks. %v\n", err)
		return nil, err
	}
	var subscribers []blueprint.User
	err = json.Unmarshal(res.Subscribers.([]byte), &subscribers)
	if err != nil {
		log.Printf("[db][FetchFollowByEntityID] error unmarshalling subscribers. %v\n", err)
		return nil, err
	}
	log.Printf("[db][FetchFollowByEntityID] found %v subscribers\n", subscribers)
	res.Subscribers = subscribers
	return &res, nil
}

func (d *NewDB) CreateFollowNotification(user, followID string, data interface{}) error {
	log.Printf("[db][CreateNewFollowNotification] Running query %s with '%s', '%s', '%s'\n", queries.CreateFollowNotification, user, followID, data)
	_, execErr := d.DB.Exec(queries.CreateFollowNotification, user, followID, data)
	if execErr != nil {
		log.Printf("[db][CreateNewFollowNotification] error creating new follow notification. %v\n", execErr)
		return execErr
	}
	log.Printf("[db][CreateNewFollowNotification] created new follow notification for playlist %s and subscriber %s\n", followID, user)
	return nil
}

// UpdateFollowSubscriber adds a subscriber to a follow task if they already haven't been added
func (d *NewDB) UpdateFollowSubscriber(subscriber, entityId string) ([]byte, error) {
	log.Printf("[db][UpdateTaskSubscriber] Running query %s with '%s', '%s'\n", queries.UpdateFollowSubscriber, subscriber, entityId)
	r := d.DB.QueryRowx(queries.UpdateFollowSubscriber, subscriber, entityId)
	var res string
	err := r.Scan(&res)
	if err != nil {
		log.Printf("[db][UpdateTaskSubscriber] error updating follow task. %v\n", err)
		// if the error is "no rows in result set" then the subscriber already exists
		return nil, err
	}
	return []byte(res), nil
}

// FetchFollowsToProcess fetches all follow tasks that need to be processed
func (d *NewDB) FetchFollowsToProcess() (*[]blueprint.FollowsToProcess, error) {
	log.Printf("[db][FetchFollowsToProcess] Running query %s\n", queries.FetchPlaylistFollowsToProcess)
	rows, err := d.DB.Queryx(queries.FetchPlaylistFollowsToProcess)

	if err != nil {
		log.Printf("[db][FetchFollowsToProcess] error fetching follows to process. %v\n", err)
		return nil, err
	}

	var res []blueprint.FollowsToProcess
	for rows.Next() {
		var r blueprint.FollowsToProcess
		err := rows.StructScan(&r)
		if err != nil {
			log.Printf("[db][FetchFollowsToProcess] error fetching follow task. %v\n", err)
			return nil, err
		}

		var subscribers []blueprint.User
		err = json.Unmarshal((r.Subscribers).([]byte), &subscribers)
		if err != nil {
			log.Printf("[db][FetchFollowsToProcess] error unmarshalling subscribers. %v\n", err)
			return nil, err
		}

		log.Printf("[db][FetchFollowsToProcess] fetched follow task %v\n", subscribers)

		if err != nil {
			log.Printf("[db][FetchFollowsToProcess] error deserializing follow task. I DO NOT EXPECT THIS TO HAPPEN%v\n", err)
			return nil, err
		}
		//r.Result = &deserialize

		// log.Printf("[db][FetchFollowsToProcess] deserialized result %v\n", &deserialize)
		res = append(res, r)
	}
	log.Printf("[db][FetchFollowsToProcess] fetched %d follow tasks\n", len(res))
	return &res, nil
}

func (d *NewDB) UpdateFollowStatus(followId, status string) error {
	log.Printf("[db][UpdateFollowStatus] Running query %s\n", queries.UpdateFollowStatus)
	_, err := d.DB.Exec(queries.UpdateFollowStatus, status, followId)
	if err != nil {
		log.Printf("[db][UpdateFollowStatus] error updating follow status. %v\n", err)
		return err
	}
	log.Printf("[db][UpdateFollowStatus] updated follow: '%s' status to '%s'\n", queries.UpdateFollowStatus, "error")
	return nil
}

func (d *NewDB) UpdateRedirectURL(user, redirectURL string) error {
	log.Printf("[db][CreateOrUpdateWebhookURL] Running query %s\n", queries.UpdateRedirectURL)
	_, err := d.DB.Exec(queries.UpdateRedirectURL, user, redirectURL)
	if err != nil {
		log.Printf("[db][CreateOrUpdateWebhookURL] error creating or updating webhook url. %v\n", err)
		return err
	}
	log.Printf("[db][CreateOrUpdateWebhookURL] created or updated webhook url\n")
	return nil
}

func (d *NewDB) AlreadyInWaitList(user string) bool {
	log.Printf("[db][FetchUserFromWaitlist] Running query %s\n", queries.FetchUserFromWaitlist)
	r := d.DB.QueryRowx(queries.FetchUserFromWaitlist, user)
	var res string
	err := r.Scan(&res)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[db][FetchUserFromWaitlist] user %s not found in waitlist\n", user)
		}
		log.Printf("[db][FetchUserFromWaitlist] error fetching user from waitlist. %v\n", err)
		return false
	}
	return true
}

/// MIGHT BE USEFUL, keeping around for historical reasons, remove later
//func (d *NewDB) UpdateUserPlatformToken(token byte, email, platform string) error {
//	log.Printf("[db][UpdateUserPlatformToken] Running query %s\n", queries.UpdateUserPlatformToken)
//	_, err := d.DB.Exec(queries.UpdateUserPlatformToken, token, email, platform)
//	if err != nil {
//		log.Printf("[db][UpdateUserPlatformToken] error updating user platform token. %v\n", err)
//		return err
//	}
//	log.Printf("[db][UpdateUserPlatformToken] updated user: '%s' token to '%s'\n", email, token)
//	return nil
//}
