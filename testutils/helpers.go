package testutils

import (
	"encoding/json"
	"log"
	"orchdio/blueprint"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// TestDataBuilder helps create test data
type TestDataBuilder struct {
	db *sqlx.DB
	t  *testing.T
}

// NewTestDataBuilder creates a new test data builder
func NewTestDataBuilder(t *testing.T, db *sqlx.DB) *TestDataBuilder {
	t.Helper()
	return &TestDataBuilder{db: db, t: t}
}

// CreateTestUser creates a test user
func (b *TestDataBuilder) CreateTestUser(email string) *blueprint.User {
	b.t.Helper()

	user := &blueprint.User{
		Email: email,
		UUID:  uuid.New(),
	}

	query := `INSERT INTO users (email, uuid, created_at, updated_at)
			  VALUES ($1, $2, NOW(), NOW()) RETURNING id`

	err := b.db.Get(&user.ID, query, user.Email, user.UUID)
	require.NoError(b.t, err)

	return user
}

// CreateTestOrganization creates a test organization
func (b *TestDataBuilder) CreateTestOrganization(name, description string, owner uuid.UUID) *blueprint.Organization {
	b.t.Helper()

	org := &blueprint.Organization{
		Name:        name,
		Description: description,
		Owner:       owner,
		UID:         uuid.New(),
	}

	query := `INSERT INTO organizations (name, description, owner, uuid, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, NOW(), NOW()) RETURNING id`

	err := b.db.Get(&org.ID, query, org.Name, org.Description, org.Owner, org.UID)
	require.NoError(b.t, err)

	return org
}

// CreateTestApp creates a test developer app
func (b *TestDataBuilder) CreateTestApp(name, description string, developer, organization uuid.UUID) *blueprint.DeveloperApp {
	b.t.Helper()

	app := &blueprint.DeveloperApp{
		UID:          uuid.New(),
		Name:         name,
		Description:  description,
		Developer:    developer,
		PublicKey:    uuid.New(),
		Authorized:   true,
		Organization: organization.String(),
	}

	query := `INSERT INTO apps (uuid, name, description, developer, public_key, authorized, organization, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW()) RETURNING id`

	err := b.db.Get(&app.ID, query, app.UID, app.Name, app.Description, app.Developer, app.PublicKey, app.Authorized, app.Organization)
	require.NoError(b.t, err)

	return app
}

// CreateTestTask creates a test task
func (b *TestDataBuilder) CreateTestTask(entityID, status string, appUUID uuid.UUID, result interface{}) *blueprint.TaskRecord {
	b.t.Helper()

	task := &blueprint.TaskRecord{
		UID:      uuid.New(),
		EntityID: entityID,
		Status:   status,
		App:      appUUID.String(),
		UniqueID: GenerateTestShortID(),
	}

	if result != nil {
		resultBytes, err := json.Marshal(result)
		require.NoError(b.t, err)
		task.Result = string(resultBytes)
	}

	query := `INSERT INTO tasks (uuid, entity_id, status, app, shortid, result, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW()) RETURNING id`

	err := b.db.Get(&task.Id, query, task.UID, task.EntityID, task.Status, task.App, task.UniqueID, task.Result)
	require.NoError(b.t, err)

	return task
}

// CleanupTestData removes all test data from database
func (b *TestDataBuilder) CleanupTestData() {
	b.t.Helper()

	log.Print("CLEANING UP TEST DATA")

	queries := []string{
		"DELETE FROM tasks",
		"DELETE FROM user_apps",
		"DELETE FROM apps",
		"DELETE FROM organizations",
		"DELETE FROM users",
		"DELETE FROM follows",
		"DELETE FROM waitlists",
	}

	for _, query := range queries {
		_, err := b.db.Exec(query)
		require.NoError(b.t, err)
	}
}

// GenerateTestShortID generates a test short ID
func GenerateTestShortID() string {
	return uuid.New().String()[:8]
}
