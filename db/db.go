package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"log"
	"net/mail"
	"orchdio/blueprint"
	"orchdio/db/queries"
	logger2 "orchdio/logger"
	"orchdio/util"
	"time"
)

// NewDB represents a new DB layer struct for performing DB related operations
type NewDB struct {
	DB *sqlx.DB
	// Logger is the zap logger instance. Due to the age of this part of the code and the fact that it is used in a lot of places,
	// the instance is checked for nil and a new instance is created if it is nil. This is to prevent breaking changes in the code
	Logger *zap.Logger
}

func New(db *sqlx.DB, logger *zap.Logger) *NewDB {
	return &NewDB{DB: db, Logger: logger}
}

// FindUserByEmail finds a user by their email
func (d *NewDB) FindUserByEmail(email string) (*blueprint.User, error) {
	result := d.DB.QueryRowx(queries.FindUserByEmail, email)
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	user := &blueprint.User{}
	err := result.StructScan(user)
	if err != nil {
		orchdioLogger.Error("[controller][db] error scanning row result. %v\n", zap.Error(err))
		return nil, err
	}
	return user, nil
}

// FindUserProfileByEmail fetches a user profile by email.
func (d *NewDB) FindUserProfileByEmail(email string) (*blueprint.UserProfile, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	result := d.DB.QueryRowx(queries.FindUserProfileByEmail, email)
	profile := &blueprint.UserProfile{}
	err := result.StructScan(profile)

	if err != nil {
		orchdioLogger.Error("[controller][db] error scanning row result. %v\n", zap.Error(err), zap.String("email", email))
		return nil, err
	}

	var usernames map[string]string
	err = json.Unmarshal(profile.Usernames.([]byte), &usernames)
	if err != nil {
		orchdioLogger.Warn("[controller][db] warning - error deserializing usernames", zap.Error(err), zap.Any("usernames", usernames))
		return nil, err
	}
	profile.Usernames = usernames
	return profile, nil
}

// FindUserByUUID finds a user by their UUID
func (d *NewDB) FindUserByUUID(id string) (*blueprint.User, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	result := d.DB.QueryRowx(queries.FindUserByUUID, id)
	user := &blueprint.User{}

	err := result.StructScan(user)
	if err != nil {
		orchdioLogger.Error("[controller][db] error scanning row result. %v\n", zap.Error(err))
		return nil, err
	}
	return user, nil
}

// FetchUserApikey fetches the user api key
func (d *NewDB) FetchUserApikey(email string) (*blueprint.ApiKey, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	orchdioLogger.Info("[db][FetchUserApikey] Running query", zap.String("query", queries.FetchUserApiKey), zap.String("email", email))
	result := d.DB.QueryRowx(queries.FetchUserApiKey, email)
	apiKey := &blueprint.ApiKey{}

	err := result.StructScan(apiKey)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			orchdioLogger.Error("[controller][user][FetchUserApiKey] error - error scanning row. Something went wrong and this is not an expected error.", zap.Error(err))
			return nil, err
		}
		orchdioLogger.Warn("[controller][user][FetchUserApiKey] warning - no api key found for user", zap.String("email", email))
		return nil, err
	}
	return apiKey, nil
}

// RevokeApiKey sets the revoked column to true
func (d *NewDB) RevokeApiKey(key string) error {
	log.Printf("[db][RevokeApiKey] Running query %s %s\n", queries.RevokeApiKey, key)
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.RevokeApiKey, key)
	if err != nil {
		orchdioLogger.Error("[db][RevokeApiKey] error executing query", zap.String("query", queries.RevokeApiKey), zap.Error(err), zap.String("key", key))
		return err
	}
	orchdioLogger.Info("[db][RevokeApiKey] Ran query", zap.String("query", queries.RevokeApiKey), zap.String("key", key))
	return nil
}

// UnRevokeApiKey sets the revoked column to true
func (d *NewDB) UnRevokeApiKey(key string) error {
	log.Printf("[db][UnRevokeApiKey] Running query %s %s\n", queries.UnRevokeApiKey, key)
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.UnRevokeApiKey, key)
	if err != nil {
		orchdioLogger.Error("[db][UnRevokeApiKey] error executing query", zap.String("query", queries.UnRevokeApiKey), zap.Error(err), zap.String("key", key))
		return err
	}
	log.Printf("[db][UnRevokeApiKey] Ran query %s\n", queries.UnRevokeApiKey)
	orchdioLogger.Info("[db][UnRevokeApiKey] Ran query", zap.String("query", queries.UnRevokeApiKey), zap.String("key", key))
	return nil
}

// DeleteApiKey deletes a user's api key
func (d *NewDB) DeleteApiKey(key, user string) ([]byte, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	result := d.DB.QueryRowx(queries.DeleteApiKey, key, user)
	if result == nil {
		orchdioLogger.Warn("[db][DeleteApikey] could not delete key. Seems there is no row to delete", zap.String("key", key))
		return nil, sql.ErrNoRows
	}

	deleteRes := struct {
		Key string
	}{}

	scanErr := result.StructScan(&deleteRes)
	if scanErr != nil {
		orchdioLogger.Error("[db][DeleteApiKey] - could not scan query result", zap.Error(scanErr))
		return nil, scanErr
	}

	orchdioLogger.Info("[db][DeleteApiKey] - Deleted apiKey", zap.String("key", key))
	return nil, nil
}

// FetchWebhook fetches the webhook for a user
// todo: check if i need to refactor this.
func (d *NewDB) FetchWebhook(user string) (*blueprint.Webhook, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	orchdioLogger.Info("[db][FetchWebhook] Running query", zap.String("query", queries.FetchUserWebhook), zap.String("user", user))
	result := d.DB.QueryRowx(queries.FetchUserWebhook, user)

	if result.Err() != nil {
		orchdioLogger.Error("[db][FetchWebhook] error fetching webhook for user", zap.String("user", user), zap.Error(result.Err()))
		return nil, result.Err()
	}

	webhook := blueprint.Webhook{}
	scanErr := result.StructScan(&webhook)
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			orchdioLogger.Warn("[db][FetchWebhook] no webhook found for user", zap.String("user", user))
			return nil, sql.ErrNoRows
		}
		orchdioLogger.Error("[db][FetchWebhook] error scanning row result", zap.Error(scanErr))
		return nil, scanErr
	}

	orchdioLogger.Info("[db][FetchWebhook] Ran query", zap.String("query", queries.FetchUserWebhook), zap.String("user", user))
	return &webhook, nil
}

// CreateUserWebhook creates a webhook for a user
func (d *NewDB) CreateUserWebhook(user, url, verifyToken string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	// first fetch the user's webhook
	_, err := d.FetchWebhook(user)
	uniqueID, _ := uuid.NewUUID()

	if err == nil {
		orchdioLogger.Warn("[db][CreateUserWebhook] user already has a webhook", zap.String("user", user))
		return blueprint.EALREADY_EXISTS
	}
	// TODO: handle more errors FetchWebhook can return
	orchdioLogger.Info("[db][CreateUserWebhook] Running query", zap.String("query", queries.CreateWebhook), zap.String("user", user), zap.String("url", url))
	_, execErr := d.DB.Exec(queries.CreateWebhook, url, user, verifyToken, uniqueID.String())

	if execErr != nil {
		orchdioLogger.Error("[db][CreateUserWebhook] error creating webhook for user", zap.String("user", user), zap.Error(execErr))
		return execErr
	}

	orchdioLogger.Info("[db][CreateUserWebhook] Ran query", zap.String("query", queries.CreateWebhook), zap.String("user", user))
	return nil
}

// FetchUserWithApiKey fetches a user with an api key
func (d *NewDB) FetchUserWithApiKey(key string) (*blueprint.User, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][FetchUserWithApiKey] Running query", zap.String("query", queries.FetchUserWithApiKey), zap.String("key", key))
	result := d.DB.QueryRowx(queries.FetchUserWithApiKey, key)

	if result == nil {
		orchdioLogger.Warn("[db][FetchUserWithApiKey] no user found with api key", zap.String("key", key))
		return nil, sql.ErrNoRows
	}
	orchdioLogger.Info("[db][FetchUserWithApiKey] Ran query", zap.String("query", queries.FetchUserWithApiKey), zap.String("key", key))
	usr := blueprint.User{}
	scanErr := result.StructScan(&usr)
	if scanErr != nil {
		orchdioLogger.Error("[db][FetchUserWithApiKey] error scanning row result", zap.Error(scanErr))
		return nil, scanErr
	}
	return &usr, nil
}

// UpdateUserWebhook updates a user's webhook
func (d *NewDB) UpdateUserWebhook(user, url, verifyToken string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	orchdioLogger.Info("[db][UpdateUserWebhook] Running query", zap.String("query", queries.UpdateUserWebhook), zap.String("user", user), zap.String("url", url))
	// temporary struct to deserialize the record update into.
	// not creating inside blueprint because its small and used here alone. if this changes, move to blueprint
	webhookUpdate := &struct {
		UUID uuid.UUID `json:"uuid" db:"uuid"`
	}{}

	updatedWH := d.DB.QueryRowx(queries.UpdateUserWebhook, url, user, verifyToken)
	execErr := updatedWH.StructScan(webhookUpdate)

	if execErr != nil {
		orchdioLogger.Error("[db][UpdateUserWebhook] error updating user webhook", zap.Error(execErr))
		return execErr
	}

	if webhookUpdate.UUID.String() == "" {
		orchdioLogger.Warn("[db][UpdateUserWebhook] no webhook to update for this user", zap.String("user", user))
		return sql.ErrNoRows
	}

	orchdioLogger.Info("[db][UpdateUserWebhook] Ran query", zap.String("query", queries.UpdateUserWebhook), zap.String("user", user))
	return nil
}

// DeleteUserWebhook deletes a user's webhook
func (d *NewDB) DeleteUserWebhook(user string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][DeleteUserWebhook] Running query", zap.String("query", queries.DeleteUserWebhook), zap.String("user", user))
	_, execErr := d.DB.Exec(queries.DeleteUserWebhook, user)
	if execErr != nil {
		orchdioLogger.Error("[db][DeleteUserWebhook] error deleting user webhook", zap.Error(execErr))
		return execErr
	}
	orchdioLogger.Info("[db][DeleteUserWebhook] Ran query", zap.String("query", queries.DeleteUserWebhook), zap.String("user", user))
	return nil
}

// CreateOrUpdateTask creates or updates a task and returns the id of the task or an error
func (d *NewDB) CreateOrUpdateTask(uid, shortid, user, entityId string) ([]byte, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][CreateOrUpdateNewTask] Running query", zap.String("query", queries.CreateOrUpdateTask), zap.String("uid", uid), zap.String("user", user), zap.String("entityId", entityId))
	r := d.DB.QueryRowx(queries.CreateOrUpdateTask, uid, shortid, user, entityId)
	var res string
	execErr := r.Scan(&res)
	if execErr != nil {
		orchdioLogger.Error("[db][CreateOrUpdateNewTask] error creating or updating new task", zap.Error(execErr))
		return nil, execErr
	}
	orchdioLogger.Info("[db][CreateOrUpdateNewTask] Ran query", zap.String("query", queries.CreateOrUpdateTask), zap.String("uid", uid), zap.String("user", user), zap.String("entityId", entityId))
	return []byte(res), nil
}

// UpdateTaskStatus updates a task's status and returns an error
func (d *NewDB) UpdateTaskStatus(uid, status string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][UpdateTaskStatus] Running query", zap.String("query", queries.UpdateTaskStatus), zap.String("uid", uid), zap.String("status", status))
	_, execErr := d.DB.Exec(queries.UpdateTaskStatus, uid, status)
	if execErr != nil {
		orchdioLogger.Error("[db][UpdateTaskStatus] error updating task status", zap.Error(execErr))
		return execErr
	}
	orchdioLogger.Info("[db][UpdateTaskStatus] Ran query", zap.String("query", queries.UpdateTaskStatus), zap.String("uid", uid), zap.String("status", status))
	return nil
}

// UpdateTaskResult updates a task and returns the result of the task or an error
func (d *NewDB) UpdateTaskResult(uid, data string) (*blueprint.PlaylistConversion, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][UpdateTaskResult] Running query", zap.String("query", queries.UpdateTaskResult), zap.String("uid", uid), zap.String("data", data))
	r := d.DB.QueryRowx(queries.UpdateTaskResult, uid, data)
	//var res blueprint.PlaylistConversion
	var res string
	execErr := r.Scan(&res)

	if execErr != nil {
		orchdioLogger.Error("[db][UpdateTaskResult] error updating task", zap.Error(execErr))
		return nil, execErr
	}

	// deserialize into a playlist conversion
	var pc blueprint.PlaylistConversion
	err := json.Unmarshal([]byte(res), &pc)
	if err != nil {
		orchdioLogger.Error("[db][UpdateTaskResult] error deserializing task", zap.Error(err))
		return nil, err
	}
	return &pc, nil
}

// FetchTask fetches a task and returns the task or an error
func (d *NewDB) FetchTask(uid string) (*blueprint.TaskRecord, error) {
	log.Printf("[db][FetchTask] Running query %s with '%s'\n", queries.FetchTask, uid)
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	// currently, in the db we were fetching by taskid, but we also want to fetch by the shortid
	// so we check if the taskId is a valid uuid, if it is, we fetch by taskid, if not, we fetch by shortid
	_, err := uuid.Parse(uid)
	if err != nil {
		// shortid parsing/fetching logic
		orchdioLogger.Warn("[db][FetchTask] warning - not a valid uuid, fetching by shortid", zap.String("uid", uid))
		orchdioLogger.Info("[db][FetchTask] Running query", zap.String("query", queries.FetchTaskByShortID), zap.String("uid", uid))
		//var res blueprint.PlaylistConversion
		r := d.DB.QueryRowx(queries.FetchTaskByShortID, uid)

		var res blueprint.TaskRecord
		scErr := r.StructScan(&res)
		// deserialize into a playlist conversion
		if scErr != nil {
			if errors.Is(scErr, err) {
				orchdioLogger.Warn("[db][FetchTask] warning - no task found with uid", zap.String("uid", uid))
				return nil, sql.ErrNoRows
			}
			orchdioLogger.Error("[db][FetchTask] error deserializing task", zap.Error(err))
			return nil, err
		}
		orchdioLogger.Info("[db][FetchTask] Ran query", zap.String("query", queries.FetchTaskByShortID), zap.String("uid", uid))
		return &res, nil
	}

	r := d.DB.QueryRowx(queries.FetchTask, uid)
	//var res blueprint.PlaylistConversion
	var res blueprint.TaskRecord
	err = r.StructScan(&res)

	// deserialize into a playlist conversion
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			orchdioLogger.Warn("[db][FetchTask] warning - no task found with uid", zap.String("uid", uid))
			return nil, sql.ErrNoRows
		}
		orchdioLogger.Error("[db][FetchTask] error deserializing task", zap.Error(err))
		return nil, err
	}

	return &res, nil
}

// DeleteTask deletes a task
func (d *NewDB) DeleteTask(uid string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][DeleteTask] Running query", zap.String("query", queries.DeleteTask), zap.String("uid", uid))
	_, execErr := d.DB.Exec(queries.DeleteTask, uid)
	if execErr != nil {
		orchdioLogger.Error("[db][DeleteTask] error deleting task", zap.Error(execErr))
		return execErr
	}
	orchdioLogger.Info("[db][DeleteTask] Ran query", zap.String("query", queries.DeleteTask), zap.String("uid", uid))
	return nil
}

// FetchFollowTask fetches a task that a developer already sends a request to add a subscriber to. A task is basically
// a job that runs at interval to check if the playlist has been updated. This method basically fetches this task. The "user"
// here is the developer.
func (d *NewDB) FetchFollowTask(entityId string) (*blueprint.FollowTask, error) {
	// todo: add orchdio logger
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
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	orchdioLogger.Info("[db][FetchTaskByIDAndType] Running query", zap.String("query", queries.FetchTaskByEntityIdAndType), zap.String("entityId", entityId), zap.String("taskType", taskType))
	rows := d.DB.QueryRowx(queries.FetchTaskByEntityIdAndType, entityId, taskType)
	var res blueprint.FollowTask
	err := rows.StructScan(&res)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			orchdioLogger.Error("[db][FetchTaskByIDAndType] no task found task ID", zap.String("entityId", entityId), zap.String("taskType", taskType))
			return nil, sql.ErrNoRows
		}
		orchdioLogger.Error("[db][FetchTaskByIDAndType] error fetching task", zap.Error(err))
		return nil, err
	}
	return &res, nil
}

// CreateFollowTask creates a follow task if it does not exist and updates a task if it exists and the subscriber has been subscribed
func (d *NewDB) CreateFollowTask(developer, app, uid, entityId, entityURL string, subscribers interface{}) ([]byte, error) {
	log.Printf("[db][CreateFollowTask] Running query %s with '%s', '%s', '%s' \n", queries.CreateOrAddSubscriberFollow, uid, entityId, developer)
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	r := d.DB.QueryRowx(queries.CreateOrAddSubscriberFollow, uid, developer, entityId, subscribers, entityURL, app)
	var res string
	err := r.Scan(&res)
	if err != nil {
		orchdioLogger.Error("[db][CreateFollowTask] error creating follow task", zap.Error(err))
		return nil, err
	}
	return []byte(res), nil
}

// CreateTrackTaskRecord creates a new task record for a track.
func (d *NewDB) CreateTrackTaskRecord(uid, shortId, entityId, appId string, result []byte) ([]byte, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	r := d.DB.QueryRowx(queries.CreateNewTrackTaskRecord, uid, shortId, entityId, string(result), appId)
	var res string
	err := r.Scan(&res)
	if err != nil {
		orchdioLogger.Error("[db][CreateTrackTaskRecord] error creating track task record", zap.Error(err))
		return nil, err
	}
	orchdioLogger.Info("[db][CreateTrackTaskRecord] created track task record", zap.String("uid", uid), zap.String("shortId", shortId), zap.String("entityId", entityId), zap.String("appId", appId))
	return []byte(res), nil
}

func (d *NewDB) FetchFollowByEntityID(entityId string) (*blueprint.FollowTask, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][FetchFollowByEntityID] Running query", zap.String("query", queries.FetchFollowByEntityId), zap.String("entityId", entityId))
	row := d.DB.QueryRowx(queries.FetchFollowByEntityId, entityId)
	var res blueprint.FollowTask
	err := row.StructScan(&res)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			orchdioLogger.Warn("[db][FetchFollowByEntityID] no follow found for entity", zap.String("entityId", entityId))
			return nil, sql.ErrNoRows
		}
		orchdioLogger.Error("[db][FetchFollowByEntityID] error fetching follow task", zap.Error(err))
		return nil, err
	}

	var subscribers []blueprint.User
	err = json.Unmarshal(res.Subscribers.([]byte), &subscribers)
	if err != nil {
		orchdioLogger.Error("[db][FetchFollowByEntityID] error unmarshalling subscribers", zap.Error(err))
		return nil, err
	}
	orchdioLogger.Info("[db][FetchFollowByEntityID] Ran query", zap.String("query", queries.FetchFollowByEntityId), zap.String("entityId", entityId))
	res.Subscribers = subscribers
	return &res, nil
}

func (d *NewDB) CreateFollowNotification(user, followID string, data interface{}) error {
	// todo: add orchdio logger
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
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	r := d.DB.QueryRowx(queries.UpdateFollowSubscriber, subscriber, entityId)
	var res string
	err := r.Scan(&res)
	if err != nil {
		log.Printf("[db][UpdateTaskSubscriber] error updating follow task. %v\n", err)
		orchdioLogger.Error("[db][UpdateTaskSubscriber] error updating follow task", zap.Error(err))
		// if the error is "no rows in result set" then the subscriber already exists
		return nil, err
	}
	return []byte(res), nil
}

// FetchFollowsToProcess fetches all follow tasks that need to be processed
func (d *NewDB) FetchFollowsToProcess() (*[]blueprint.FollowsToProcess, error) {
	log.Printf("[db][FetchFollowsToProcess] Running query %s\n", queries.FetchPlaylistFollowsToProcess)
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	rows, err := d.DB.Queryx(queries.FetchPlaylistFollowsToProcess)
	if err != nil {
		orchdioLogger.Error("[db][FetchFollowsToProcess] error fetching follows to process", zap.Error(err))
		return nil, err
	}

	var res []blueprint.FollowsToProcess
	for rows.Next() {
		var r blueprint.FollowsToProcess
		scanErr := rows.StructScan(&r)
		if scanErr != nil {
			orchdioLogger.Error("[db][FetchFollowsToProcess] error fetching follow task", zap.Error(err))
			return nil, err
		}

		var subscribers []blueprint.User
		err = json.Unmarshal((r.Subscribers).([]byte), &subscribers)
		if err != nil {
			orchdioLogger.Error("[db][FetchFollowsToProcess] error unmarshalling subscribers", zap.Error(err))
			return nil, err
		}

		if err != nil {
			orchdioLogger.Error("[db][FetchFollowsToProcess] error deserializing follow task. Unexpected error", zap.Error(err))
			return nil, err
		}
		res = append(res, r)
	}
	log.Printf("[db][FetchFollowsToProcess] fetched %d follow tasks\n", len(res))
	return &res, nil
}

func (d *NewDB) UpdateFollowStatus(followId, status string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	_, err := d.DB.Exec(queries.UpdateFollowStatus, status, followId)
	if err != nil {
		orchdioLogger.Error("[db][UpdateFollowStatus] error updating follow status", zap.Error(err))
		return err
	}
	orchdioLogger.Info("[db][UpdateFollowStatus] updated follow status", zap.String("query", queries.UpdateFollowStatus), zap.String("status", status))
	return nil
}

func (d *NewDB) AlreadyInWaitList(user string) bool {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	r := d.DB.QueryRowx(queries.FetchUserFromWaitlist, user)
	var res string
	err := r.Scan(&res)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			orchdioLogger.Warn("[db][FetchUserFromWaitlist] user not found in waitlist", zap.String("user", user))
		}
		orchdioLogger.Error("[db][FetchUserFromWaitlist] error fetching user from waitlist", zap.Error(err))
		return false
	}
	return true
}

// CreateOrg creates a new org in the database
func (d *NewDB) CreateOrg(uid, name, description, owner string) ([]byte, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	r := d.DB.QueryRowx(queries.CreateNewOrg, uid, name, description, owner)
	var res string

	// note that Scan is for single fields, in this case just the newly created org id.
	// structScan is for structs/json objects.
	err := r.Scan(&res)
	if err != nil {
		orchdioLogger.Error("[db][CreateOrg] error creating new org", zap.Error(err))
		return nil, err
	}
	orchdioLogger.Info("[db][CreateOrg] Ran query", zap.String("query", queries.CreateNewOrg), zap.String("uid", uid), zap.String("name", name), zap.String("description", description), zap.String("owner", owner))
	return []byte(res), nil
}

// DeleteOrg deletes an org from the database
func (d *NewDB) DeleteOrg(uid, owner string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.DeleteOrg, uid, owner)
	if err != nil {
		orchdioLogger.Error("[db][DeleteOrg] error deleting org", zap.Error(err))
		return err
	}
	orchdioLogger.Info("[db][DeleteOrg] Ran query", zap.String("query", queries.DeleteOrg), zap.String("uid", uid), zap.String("owner", owner))
	return nil
}

// UpdateOrg updates an org in the database
func (d *NewDB) UpdateOrg(appId, owner string, data *blueprint.UpdateOrganizationData) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	_, err := d.DB.Exec(queries.UpdateOrg, data.Description, data.Name, appId, owner)
	if err != nil {
		orchdioLogger.Error("[db][UpdateOrg] error updating org", zap.Error(err))
		return err
	}
	orchdioLogger.Info("[db][UpdateOrg] updated organization.", zap.String("query", queries.UpdateOrg), zap.String("appId", appId), zap.String("owner", owner), zap.Any("data", data))
	return nil
}

// FetchOrgs fetches all orgs belonging to a user
func (d *NewDB) FetchOrgs(owner string) (*blueprint.Organization, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	row := d.DB.QueryRowx(queries.FetchUserOrg, owner)
	var res blueprint.Organization
	err := row.StructScan(&res)
	if err != nil {
		orchdioLogger.Error("[db][FetchOrgs] error fetching orgs", zap.Error(err))
		return nil, err
	}
	orchdioLogger.Info("[db][FetchOrgs] Ran query", zap.String("query", queries.FetchUserOrg), zap.String("owner", owner))
	return &res, nil
}

// FetchUserByIdentifier fetches a user by the identifier (email or id) and a flag specifying which one
// it is. This is used for fetching user's info (basic info and app/platform infos).
func (d *NewDB) FetchUserByIdentifier(identifier, app string) (*[]blueprint.UserAppAndPlatformInfo, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}
	orchdioLogger.Info("[db][FetchUserByIdentifier] Running query", zap.String("query", queries.FetchUserAppAndInfo), zap.String("identifier", identifier), zap.String("app", app))
	var opt string
	isUUID := util.IsValidUUID(identifier)
	parsedEmail, err := mail.ParseAddress(identifier)
	if err != nil {
		opt = "id"
		orchdioLogger.Warn("[db][FetchUserByIdentifier] warning - invalid email used as identifier for fetching user info", zap.String("identifier", identifier), zap.String("option", opt))
	}

	isValidEmail := parsedEmail != nil
	if !isUUID && !isValidEmail {
		orchdioLogger.Warn("[db][FetchUserByIdentifier] warning - invalid identifier used for fetching user info", zap.String("identifier", identifier))
		return nil, errors.New("invalid identifier")
	}

	if isUUID {
		opt = "id"
	} else {
		opt = "email"
	}

	// hack: for now, we're going to declare valid opts to prevent accidental SQL injection or whatever
	opts := []string{"email", "id"}

	if !lo.Contains(opts, opt) {
		orchdioLogger.Warn("[db][FetchUserByIdentifier] warning - invalid option used for fetching user info", zap.String("opt", opt))
		return nil, errors.New("invalid opt")
	}

	row, err := d.DB.Queryx(queries.FetchUserAppAndInfo, identifier, app, opt)
	if err != nil {
		orchdioLogger.Error("[db][FetchUserByIdentifier] error fetching user", zap.Error(err))
		return nil, err
	}

	var res []blueprint.UserAppAndPlatformInfo
	for row.Next() {
		var r blueprint.UserAppAndPlatformInfo
		err = row.StructScan(&r)
		if err != nil {
			orchdioLogger.Error("[db][FetchUserByIdentifier] error scanning user", zap.Error(err))
			return nil, err
		}

		res = append(res, r)
	}
	return &res, nil
}

// FetchPlatformAndUserInfoByIdentifier fetches a user by the identifier (email or id) and a flag specifying which one and the platform the user
func (d *NewDB) FetchPlatformAndUserInfoByIdentifier(identifier, app, platform string) (*blueprint.UserAppAndPlatformInfo, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	var opt string
	isUUID := util.IsValidUUID(identifier)
	parseEmail, err := mail.ParseAddress(identifier)
	if err != nil {
		opt = "id"
		orchdioLogger.Warn("[db][FetchPlatformAndUserInfoByIdentifier] warning - invalid email used as identifier for fetching user info", zap.String("identifier", identifier), zap.String("option", opt))
	}

	if !isUUID && parseEmail == nil {
		orchdioLogger.Warn("[db][FetchPlatformAndUserInfoByIdentifier] warning - invalid identifier used for fetching user info", zap.String("identifier", identifier))
		return nil, errors.New("invalid identifier")
	}

	if isUUID {
		opt = "id"
	} else {
		opt = "email"
	}

	// 1. uuid / email
	// 2. app id
	// 3. platform
	row := d.DB.QueryRowx(queries.FetchUserAppAndInfoByPlatform, identifier, app, opt, platform)
	var res blueprint.UserAppAndPlatformInfo
	err = row.StructScan(&res)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	return &res, nil
}

// UpdateUserPassword updates a user's password
func (d *NewDB) UpdateUserPassword(hash, userId string) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.UpdateUserPassword, hash, userId)
	if err != nil {
		orchdioLogger.Error("[db][UpdateUserPassword] error updating user password", zap.Error(err))
		return err
	}
	orchdioLogger.Info("[db][UpdateUserPassword] updated user password", zap.String("userId", userId))
	return nil
}

func (d *NewDB) SaveUserResetToken(id, token string, expiry time.Time) error {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	_, err := d.DB.Exec(queries.SaveUserResetToken, id, token, expiry)
	if err != nil {
		orchdioLogger.Error("[db][SaveUserResetToken] error saving user reset token", zap.Error(err))
		return err
	}
	orchdioLogger.Info("[db][SaveUserResetToken] saved user reset token", zap.String("token", token))
	return nil
}

// FindUserByResetToken finds a user by the reset token
func (d *NewDB) FindUserByResetToken(token string) (*blueprint.User, error) {
	orchdioLogger := d.Logger
	if orchdioLogger == nil {
		orchdioLogger = logger2.NewZapSentryLogger()
	}

	row := d.DB.QueryRowx(queries.FindUserByResetToken, token)
	var res blueprint.User
	err := row.StructScan(&res)
	if err != nil {
		orchdioLogger.Error("[db][FindUserByResetToken] error finding user by reset token", zap.Error(err))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	orchdioLogger.Info("[db][FindUserByResetToken] found user by reset token", zap.String("token", token))
	return &res, nil
}
