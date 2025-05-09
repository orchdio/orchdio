package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"orchdio/blueprint"
	"orchdio/db/queries"
	"orchdio/util"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
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

// FindUserProfileByEmail fetches a user profile by email.
func (d *NewDB) FindUserProfileByEmail(email string) (*blueprint.UserProfile, error) {
	result := d.DB.QueryRowx(queries.FindUserProfileByEmail, email)
	profile := &blueprint.UserProfile{}
	err := result.StructScan(profile)

	if err != nil {
		log.Printf("\n[controller][db] warning - error fetching user profile by email")
		return nil, err
	}

	var usernames map[string]string
	err = json.Unmarshal(profile.Usernames.([]byte), &usernames)
	if err != nil {
		log.Printf("\n[controller][db] warning - error deserializing usernames")
		return nil, err
	}
	profile.Usernames = usernames
	return profile, nil
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
	return user, nil
}

// FetchUserApikey fetches the user api key
func (d *NewDB) FetchUserApikey(email string) (*blueprint.ApiKey, error) {
	result := d.DB.QueryRowx(queries.FetchUserApiKey, email)
	apiKey := &blueprint.ApiKey{}

	err := result.StructScan(apiKey)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("[controller][user][FetchUserApiKey] error - error scanning row. Something went wrong and this is not an expected error. %v\n", err)
			return nil, err
		}
		return nil, err
	}
	return apiKey, nil
}

// RevokeApiKey sets the revoked column to true
func (d *NewDB) RevokeApiKey(key string) error {
	_, err := d.DB.Exec(queries.RevokeApiKey, key)
	if err != nil {
		log.Printf("[db][RevokeApiKey] error executing query %s.\n %v\n %s\n", queries.RevokeApiKey, err, key)
		return err
	}
	return nil
}

// UnRevokeApiKey sets the revoked column to true
func (d *NewDB) UnRevokeApiKey(key string) error {
	_, err := d.DB.Exec(queries.UnRevokeApiKey, key)
	if err != nil {
		log.Printf("[db][UnRevokeApiKey] error executing query %s.\n %v\n\n", queries.UnRevokeApiKey, err)
		return err
	}
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
		if errors.Is(scanErr, sql.ErrNoRows) {
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

	// TODO: handle more errors FetchWebhook can return
	if err == nil {
		log.Printf("[db][CreateUserWebhook] user %s already has a webhook.\n", user)
		return blueprint.EalreadyExists
	}
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
	return &usr, nil
}

// UpdateUserWebhook updates a user's webhook
func (d *NewDB) UpdateUserWebhook(user, url, verifyToken string) error {
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
	// currently, in the db we were fetching by taskid, but we also want to fetch by the shortid
	// so we check if the taskId is a valid uuid, if it is, we fetch by taskid, if not, we fetch by shortid
	_, err := uuid.Parse(uid)
	if err != nil {

		// shortid parsing/fetching logic
		log.Printf("[controller][conversion][GetPlaylistTaskStatus] - not a valid uuid, fetching by shortid")
		r := d.DB.QueryRowx(queries.FetchTaskByShortID, uid)

		var res blueprint.TaskRecord
		sErr := r.StructScan(&res)
		// deserialize into a playlist conversion
		if sErr != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Printf("[db][FetchTask] no task found with uid %s\n", uid)
				return nil, sql.ErrNoRows
			}
			log.Printf("[db][FetchTask] error deserializing task. %v\n", err)
			return nil, err
		}
		return &res, nil
	}

	r := d.DB.QueryRowx(queries.FetchTask, uid)
	var res blueprint.TaskRecord
	err = r.StructScan(&res)

	// deserialize into a playlist conversion
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	rows := d.DB.QueryRowx(queries.FetchFollowedTask, entityId)
	var res blueprint.FollowTask
	err := rows.StructScan(&res)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
func (d *NewDB) CreateFollowTask(developer, app, uid, entityId, entityURL string, subscribers interface{}) ([]byte, error) {
	r := d.DB.QueryRowx(queries.CreateOrAddSubscriberFollow, uid, developer, entityId, subscribers, entityURL, app)
	var res string
	err := r.Scan(&res)
	if err != nil {
		log.Printf("[db][CreateFollowTask] error creating follow task. %v\n", err)
		return nil, err
	}
	return []byte(res), nil
}

// CreateTrackTaskRecord creates a new task record for a track.
func (d *NewDB) CreateTrackTaskRecord(uid, shortId, entityId, appId string, result []byte) ([]byte, error) {
	r := d.DB.QueryRowx(queries.CreateNewTrackTaskRecord, uid, shortId, entityId, string(result), appId)
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
	row := d.DB.QueryRowx(queries.FetchFollowByEntityId, entityId)
	var res blueprint.FollowTask
	err := row.StructScan(&res)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
		mErr := json.Unmarshal((r.Subscribers).([]byte), &subscribers)
		if mErr != nil {
			log.Printf("[db][FetchFollowsToProcess] error unmarshalling subscribers. %v\n", mErr)
			return nil, err
		}

		log.Printf("[db][FetchFollowsToProcess] fetched follow task %v\n", subscribers)
		res = append(res, r)
	}
	log.Printf("[db][FetchFollowsToProcess] fetched %d follow tasks\n", len(res))
	return &res, nil
}

func (d *NewDB) UpdateFollowStatus(followId, status string) error {
	_, err := d.DB.Exec(queries.UpdateFollowStatus, status, followId)
	if err != nil {
		log.Printf("[db][UpdateFollowStatus] error updating follow status. %v\n", err)
		return err
	}
	log.Printf("[db][UpdateFollowStatus] updated follow: '%s' status to '%s'\n", queries.UpdateFollowStatus, "error")
	return nil
}

func (d *NewDB) AlreadyInWaitList(user string) bool {
	r := d.DB.QueryRowx(queries.FetchUserFromWaitlist, user)
	var res string
	err := r.Scan(&res)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[db][FetchUserFromWaitlist] user %s not found in waitlist\n", user)
		}
		log.Printf("[db][FetchUserFromWaitlist] email has not been added to waitlist%v\n", err)
		return false
	}
	return true
}

// CreateOrg creates a new org in the database
func (d *NewDB) CreateOrg(uid, name, description, owner string) ([]byte, error) {
	r := d.DB.QueryRowx(queries.CreateNewOrg, uid, name, description, owner)
	var res string

	// note that Scan is for single fields, in this case just the newly created org id.
	// structScan is for structs/json objects.
	err := r.Scan(&res)
	if err != nil {
		log.Printf("[db][CreateOrg] error creating new org. %v\n", err)
		return nil, err
	}
	log.Printf("[db][CreateOrg] created new org %s\n", res)
	return []byte(res), nil
}

// DeleteOrg deletes an org from the database
func (d *NewDB) DeleteOrg(uid, owner string) error {
	_, err := d.DB.Exec(queries.DeleteOrg, uid, owner)
	if err != nil {
		log.Printf("[db][DeleteOrg] error deleting org. %v\n", err)
		return err
	}
	log.Printf("[db][DeleteOrg] deleted org %s\n", uid)
	return nil
}

// UpdateOrg updates an org in the database
func (d *NewDB) UpdateOrg(appId, owner string, data *blueprint.UpdateOrganizationData) error {
	_, err := d.DB.Exec(queries.UpdateOrg, data.Description, data.Name, appId, owner)
	if err != nil {
		log.Printf("[db][UpdateOrg] error updating org. %v\n", err)
		return err
	}
	log.Printf("[db][UpdateOrg] updated org %s\n", appId)
	return nil
}

// FetchOrg fetches the org belonging to a user. Orgs are limited to 1 for now, for each user.
func (d *NewDB) FetchOrg(owner string) (*blueprint.Organization, error) {
	row := d.DB.QueryRowx(queries.FetchUserOrg, owner)
	var res blueprint.Organization
	err := row.StructScan(&res)
	if err != nil {
		log.Printf("[db][FetchOrg] error fetching orgs. %v\n", err)
		return nil, err
	}
	log.Printf("[db][FetchOrg] fetched orgs %v\n", res)
	return &res, nil
}

// FetchUserByIdentifier fetches a user by the identifier (email or id) and a flag specifying which one
// it is. This is used for fetching user's info (basic info and app/platform infos).
func (d *NewDB) FetchUserByIdentifier(identifier, app string) (*[]blueprint.UserAppAndPlatformInfo, error) {

	valid, opt := util.FetchIdentifierOption(identifier)
	if !valid {
		log.Printf("[db][FetchUserByIdentifier] Identifier %s is not a valid identifier\n", identifier)
		return nil, nil
	}

	if !lo.Contains(blueprint.ValidUserIdentifiers, strings.ToLower(string(opt))) {
		log.Printf("[db][FetchUserByIdentifier] - invalid opt '%s'\n", opt)
		return nil, errors.New("invalid opt")
	}

	row, err := d.DB.Queryx(queries.FetchUserAppAndInfo, identifier, app, opt)
	if err != nil {
		log.Printf("[db][FetchUserByIdentifier] error fetching user. %v\n", err)
		return nil, err
	}

	var res []blueprint.UserAppAndPlatformInfo
	for row.Next() {
		var r blueprint.UserAppAndPlatformInfo
		err = row.StructScan(&r)
		if err != nil {
			log.Printf("[db][FetchUserByIdentifier] error scanning user. %v\n", err)
			return nil, err
		}

		res = append(res, r)
	}

	log.Printf("[db][FetchUserByIdentifier] fetched user's info and apps info. They have %d apps\n", len(res))
	return &res, nil
}

// FetchPlatformAndUserInfoByIdentifier fetches a user by the identifier (email or id) and a flag specifying which one and the platform the user
func (d *NewDB) FetchPlatformAndUserInfoByIdentifier(identifier, app, platform string) (*blueprint.UserAppAndPlatformInfo, error) {
	valid, opt := util.FetchIdentifierOption(identifier)
	if !valid {
		log.Printf("[db][FetchPlatformAndUserInfoByIdentifier] Identifier %s is not a valid identifier\n", identifier)
	}

	log.Printf("[db][FetchPlatformAndUserInfoByIdentifier] Running query with %s  %s %s\n", identifier, app, opt)
	// 1. uuid / email — the real value passed.
	// 2. app id
	// identifier — id or email
	// 3. platform
	row := d.DB.QueryRowx(queries.FetchUserAppAndInfoByPlatform, identifier, app, opt, platform)
	var res blueprint.UserAppAndPlatformInfo
	err := row.StructScan(&res)
	if err != nil {
		log.Printf("[db][FetchPlatformAndUserInfoByIdentifier] error scanning user: could not fetch user app and info by platform. %v\n", err)
		return nil, err
	}

	return &res, nil
}

// UpdateUserPassword updates a user's password
func (d *NewDB) UpdateUserPassword(hash, userId string) error {
	_, err := d.DB.Exec(queries.UpdateUserPassword, hash, userId)
	if err != nil {
		log.Printf("[db][UpdateUserPassword] error updating user password. %v\n", err)
		return err
	}
	log.Printf("[db][UpdateUserPassword] updated user password %s\n", userId)
	return nil
}

func (d *NewDB) SaveUserResetToken(id, token string, expiry time.Time) error {
	_, err := d.DB.Exec(queries.SaveUserResetToken, id, token, expiry)
	if err != nil {
		log.Printf("[db][SaveUserResetToken] error saving user reset token. %v\n", err)
		return err
	}
	log.Printf("[db][SaveUserResetToken] saved user reset token %s\n", token)
	return nil
}

// FindUserByResetToken finds a user by the reset token
func (d *NewDB) FindUserByResetToken(token string) (*blueprint.User, error) {
	row := d.DB.QueryRowx(queries.FindUserByResetToken, token)
	var res blueprint.User
	err := row.StructScan(&res)
	if err != nil {
		log.Printf("[db][FindUserByResetToken] error finding user by reset token. %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	log.Printf("[db][FindUserByResetToken] found user by reset token %s\n", token)
	return &res, nil
}
