package testutils

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-migrate/migrate/v4"

	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go"
	pgtest "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestInfrastructure holds all testing infrastructure
type TestInfrastructure struct {
	PostgresContainer *pgtest.PostgresContainer
	RedisContainer    *tcredis.RedisContainer
	Network           *testcontainers.DockerNetwork
	DB                *sqlx.DB
	RedisClient       *redis.Client
	ctx               context.Context
	Cleanup           []func() error
}

// singleton pattern for shared test infrastructure
var (
	globalInfra *TestInfrastructure
	infraOnce   sync.Once
	infraMutex  sync.RWMutex
	testCount   int32 // Add counter for active tests
)

// TestConfig holds configuration for test setup
type TestConfig struct {
	PostgresImage    string
	RedisImage       string
	DatabaseName     string
	MigrationsPath   string
	NetworkName      string
	ReuseContainers  bool
	ContainerTimeout time.Duration
}

func findProjectRoot() (string, error) {
	// Start from current directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Look for go.mod file to identify project root
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("project root not found")
		}
		dir = parent
	}
}

// DefaultTestConfig returns default test configuration
func DefaultTestConfig() *TestConfig {
	projectRoot, err := findProjectRoot()
	if err != nil {
		// Fallback to relative path
		projectRoot = "../.."
	}

	return &TestConfig{
		PostgresImage:    "postgres",
		RedisImage:       "redis:latest",
		DatabaseName:     "orchdio_test",
		MigrationsPath:   filepath.Join(projectRoot, "db", "migration"),
		NetworkName:      "orchdio_test_network",
		ReuseContainers:  true,
		ContainerTimeout: 60 * time.Second,
	}
}

func init() {
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	os.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
}

// GetOrCreateInfrastructure returns shared test infrastructure (singleton)
func GetOrCreateInfrastructure(t *testing.T, config ...*TestConfig) *TestInfrastructure {
	t.Helper()

	infraMutex.RLock()
	if globalInfra != nil {
		defer infraMutex.RUnlock()
		return globalInfra
	}
	infraMutex.RUnlock()

	infraOnce.Do(func() {
		cfg := DefaultTestConfig()
		if len(config) > 0 && config[0] != nil {
			log.Print("DEBUG: USING TEST CONFIG DATA")
			cfg = config[0]
		}

		var err error
		log.Printf("INFRASTRUCTURE CONFIG IS: %v", cfg)
		globalInfra, err = setupInfrastructure(cfg)
		if err != nil {
			t.Fatalf("Failed to setup test infrastructure: %v", err)
		}
	})

	// ensure cleanup happens when tests complete
	t.Cleanup(func() {
		CleanupGlobalInfrastructure()
	})

	return globalInfra
}

func cleanupSingleTest(t *testing.T) {
	// Decrement counter
	remaining := atomic.AddInt32(&testCount, -1)

	if remaining == 0 {
		// This is the last test, cleanup everything
		CleanupGlobalInfrastructure()
	} else {
		// Just cleanup test-specific data, keep connections alive
		if globalInfra != nil && globalInfra.DB != nil {
			// Clean test data but don't close connection
			cleanupTestData(globalInfra.DB)
		}
	}
}

func cleanupTestData(db *sqlx.DB) {
	// Clean up test data without closing connection
	queries := []string{
		"DELETE FROM track_tasks",
		"DELETE FROM tasks",
		"DELETE FROM user_apps",
		"DELETE FROM apps",
		"DELETE FROM organizations",
		"DELETE FROM users",
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			// log error but don't fail
			fmt.Printf("Warning: failed to cleanup test data: %v\n", err)
		}
	}
}

// setupInfrastructure creates the test infrastructure
func setupInfrastructure(config *TestConfig) (*TestInfrastructure, error) {
	ctx := context.Background()
	infra := &TestInfrastructure{ctx: ctx}
	log.Printf("SETTING UP INFRASTRUCTURE...%v", config)

	connStr := fmt.Sprintf("postgres://orchdiotestusercont:password@localhost:5432/%s?sslmode=disable", config.DatabaseName)

	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := sqlx.NewDb(sqlDB, "postgres")

	infra.DB = db
	// Run migrations
	if err := runMigrations(sqlDB, config.MigrationsPath); err != nil {
		// infra.cleanup()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	pgContainer, err := pgtest.Run(ctx, config.PostgresImage)

	if err != nil {
		infra.cleanup()
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	infra.PostgresContainer = pgContainer
	infra.Cleanup = append(infra.Cleanup, func() error {
		return pgContainer.Terminate(ctx)
	})

	// setup Redis container
	redisContainer, err := tcredis.Run(infra.ctx, "redis")
	if err != nil {
		infra.cleanup()
		return nil, fmt.Errorf("failed to start redis container: %w", err)
	}
	infra.RedisContainer = redisContainer
	infra.Cleanup = append(infra.Cleanup, func() error {
		return redisContainer.Terminate(ctx)
	})

	// Get Redis connection
	redisConnStr, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		infra.cleanup()
		return nil, fmt.Errorf("failed to get redis connection string: %w", err)
	}

	opt, err := redis.ParseURL(redisConnStr)
	if err != nil {
		infra.cleanup()
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	redisClient := redis.NewClient(opt)
	infra.RedisClient = redisClient
	infra.Cleanup = append(infra.Cleanup, func() error {
		return redisClient.Close()
	})

	return infra, nil
}

// runMigrations applies database migrations
func runMigrations(db *sql.DB, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	absPath, err := filepath.Abs(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", absPath),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	// this closes the database connection itself...
	// TODO: manually close DB when necessary.
	// defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// CleanupGlobalInfrastructure cleans up the global test infrastructure
func CleanupGlobalInfrastructure() {
	infraMutex.Lock()
	defer infraMutex.Unlock()

	if globalInfra != nil {
		globalInfra.cleanup()
		globalInfra = nil
	}
}

// cleanup runs all cleanup functions in reverse order
func (infra *TestInfrastructure) cleanup() {
	for i := len(infra.Cleanup) - 1; i >= 0; i-- {
		if err := infra.Cleanup[i](); err != nil {
			// Log error but continue cleanup
			fmt.Printf("Error during cleanup: %v\n", err)
		}
	}
}

// NewDatabase creates a fresh database for isolated testing
func (infra *TestInfrastructure) NewDatabase(t *testing.T, dbName string) *sqlx.DB {
	t.Helper()

	ctx := context.Background()

	// Create new database
	_, err := infra.DB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		t.Fatalf("Failed to create test database %s: %v", dbName, err)
	}

	// Connect to new database
	connStr, err := infra.PostgresContainer.ConnectionString(ctx, "sslmode=disable", "dbname="+dbName)
	if err != nil {
		t.Fatalf("Failed to get connection string for %s: %v", dbName, err)
	}

	testDB, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		t.Fatalf("Failed to connect to test database %s: %v", dbName, err)
	}

	// Cleanup database when test completes
	t.Cleanup(func() {
		testDB.Close()
		infra.DB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	})

	return testDB
}
