package testutils

import (
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfrastructureSetup(t *testing.T) {
	log.Print("RUNNING INFRASTRUCTURE SETUP")
	infra := GetOrCreateInfrastructure(t)

	t.Run("postgres connection", func(t *testing.T) {
		var result int
		err := infra.DB.Get(&result, "SELECT 1")
		require.NoError(t, err)
		assert.Equal(t, 1, result)
	})

	t.Run("redis connection", func(t *testing.T) {
		ctx := context.Background()
		err := infra.RedisClient.Set(ctx, "test_key", "test_value", 0).Err()
		require.NoError(t, err)

		val, err := infra.RedisClient.Get(ctx, "test_key").Result()
		require.NoError(t, err)
		assert.Equal(t, "test_value", val)
	})

	t.Run("database tables exist", func(t *testing.T) {
		tables := []string{"users", "apps", "organizations", "tasks", "follows"}

		for _, table := range tables {
			var exists bool
			query := `SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = $1
			)`
			err := infra.DB.Get(&exists, query, table)
			require.NoError(t, err)
			assert.True(t, exists, "Table %s should exist", table)
		}
	})
}

func TestNewDatabase(t *testing.T) {
	infra := GetOrCreateInfrastructure(t)

	testDB := infra.NewDatabase(t, "isolated_test_db")

	// Test that we can use the isolated database
	var result int
	err := testDB.Get(&result, "SELECT 1")
	require.NoError(t, err)
	assert.Equal(t, 1, result)

	// Test that tables exist in isolated database
	var exists bool
	err = testDB.Get(&exists, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'users'
		)
	`)
	require.NoError(t, err)
	assert.True(t, exists)
}
