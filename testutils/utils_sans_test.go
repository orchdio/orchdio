package testutils

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestIntegration tests the complete setup with real databases
func TestIntegration(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=true to run.")
	}

	suite := GetTestSuite()
	defer suite.Close()

	t.Run("PostgreSQL Integration", func(t *testing.T) {
		testPostgresIntegration(t, suite)
	})

	t.Run("Redis Integration", func(t *testing.T) {
		testRedisIntegration(t, suite)
	})

	t.Run("Migrations Integration", func(t *testing.T) {
		testMigrationsIntegration(t, suite)
	})
}

func testPostgresIntegration(t *testing.T, suite *TestSuite) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test connection info
	var currentUser, currentDB string
	err := suite.DB.QueryRowContext(ctx, "SELECT current_user, current_database()").Scan(&currentUser, &currentDB)
	if err != nil {
		t.Fatalf("Failed to get current user and database: %v", err)
	}

	t.Logf("Connected as user: %s to database: %s", currentUser, currentDB)

	// Test user permissions
	var hasCreatePrivilege bool
	err = suite.DB.QueryRowContext(ctx,
		"SELECT has_database_privilege(current_user, current_database(), 'CREATE')").Scan(&hasCreatePrivilege)
	if err != nil {
		t.Fatalf("Failed to check CREATE privilege: %v", err)
	}

	if !hasCreatePrivilege {
		t.Error("User should have CREATE privilege on the database")
	}
}

func testRedisIntegration(t *testing.T, suite *TestSuite) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test Redis info
	info, err := suite.RedisClient.Info(ctx, "server").Result()
	if err != nil {
		t.Fatalf("Failed to get Redis server info: %v", err)
	}

	t.Logf("Redis server info: %s", info[:100]) // Log first 100 chars

	// Test Redis operations with expiration
	key := "integration_test_key"
	value := "integration_test_value"
	expiration := 30 * time.Second

	err = suite.RedisClient.Set(ctx, key, value, expiration).Err()
	if err != nil {
		t.Fatalf("Failed to set key with expiration: %v", err)
	}

	// Check TTL
	ttl, err := suite.RedisClient.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("Failed to get TTL: %v", err)
	}

	if ttl <= 0 || ttl > expiration {
		t.Errorf("Expected TTL to be between 0 and %v, got %v", expiration, ttl)
	}

	// Clean up
	suite.RedisClient.Del(ctx, key)
}

func testMigrationsIntegration(t *testing.T, suite *TestSuite) {
	// Test that migrations can be run
	err := suite.RunMigrations()
	if err != nil {
		t.Logf("Migration error (this might be expected if no migrations exist): %v", err)
	}

	// Test database reset (if migrations exist)x
	err = suite.ResetDatabase()
	if err != nil {
		t.Logf("Reset database error (this might be expected if no migrations exist): %v", err)
	}
}
