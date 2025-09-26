package testutils

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"golang.org/x/net/context"
)

// TestSuite represents the singleton test suite
type TestSuite struct {
	DB          *sql.DB
	RedisClient *redis.Client
	migrator    *migrate.Migrate
	mu          sync.RWMutex
}

var (
	instance *TestSuite
	once     sync.Once
)

// Config holds database configuration
type Config struct {
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string
	RedisHost        string
	RedisPort        string
	RedisPassword    string
	RedisDB          int
	MigrationsPath   string
}

// DefaultConfig returns default configuration for testing
func DefaultConfig() *Config {
	return &Config{
		PostgresHost:     "localhost",
		PostgresPort:     "5432",
		PostgresUser:     "orchdiotestusercont",
		PostgresPassword: "testpassword",
		PostgresDB:       "testdb",
		PostgresSSLMode:  "disable",
		RedisHost:        "localhost",
		RedisPort:        "6379",
		RedisPassword:    "",
		RedisDB:          0,
		MigrationsPath:   "file://migrations",
	}
}

// GetTestSuite returns the singleton instance of TestSuite
func GetTestSuite() *TestSuite {
	once.Do(func() {
		config := DefaultConfig()
		instance = &TestSuite{}

		if err := instance.initialize(config); err != nil {
			log.Fatalf("Failed to initialize test suite: %v", err)
		}
	})
	return instance
}

// GetTestSuiteWithConfig returns the singleton instance with custom config
func GetTestSuiteWithConfig(config *Config) *TestSuite {
	once.Do(func() {
		instance = &TestSuite{}

		if err := instance.initialize(config); err != nil {
			log.Fatalf("Failed to initialize test suite: %v", err)
		}
	})
	return instance
}

// initialize sets up the database connections and migrator
func (ts *TestSuite) initialize(config *Config) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Initialize PostgreSQL connection
	if err := ts.initPostgres(config); err != nil {
		return fmt.Errorf("failed to initialize postgres: %w", err)
	}

	// Initialize Redis connection
	if err := ts.initRedis(config); err != nil {
		return fmt.Errorf("failed to initialize redis: %w", err)
	}

	// Initialize migrator
	if err := ts.initMigrator(config); err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}

	return nil
}

// initPostgres initializes the PostgreSQL connection
func (ts *TestSuite) initPostgres(config *Config) error {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.PostgresHost,
		config.PostgresPort,
		config.PostgresUser,
		config.PostgresPassword,
		config.PostgresDB,
		config.PostgresSSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Test the connection with retries
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		if err := db.PingContext(ctx); err == nil {
			break
		}
		if i == 4 {
			return fmt.Errorf("failed to ping postgres after 5 attempts: %w", err)
		}
		time.Sleep(time.Second * 2)
	}

	ts.DB = db
	return nil
}

// initRedis initializes the Redis connection
func (ts *TestSuite) initRedis(config *Config) error {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.RedisHost, config.RedisPort),
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to ping redis: %w", err)
	}

	ts.RedisClient = rdb
	return nil
}

// initMigrator initializes the database migrator
func (ts *TestSuite) initMigrator(config *Config) error {
	driver, err := postgres.WithInstance(ts.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	migrator, err := migrate.NewWithDatabaseInstance(
		config.MigrationsPath,
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	ts.migrator = migrator
	return nil
}

// RunMigrations runs all pending migrations
func (ts *TestSuite) RunMigrations() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.migrator == nil {
		return fmt.Errorf("migrator not initialized")
	}

	if err := ts.migrator.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// ResetDatabase drops all tables and re-runs migrations
func (ts *TestSuite) ResetDatabase() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.migrator == nil {
		return fmt.Errorf("migrator not initialized")
	}

	// Drop all migrations
	if err := ts.migrator.Drop(); err != nil {
		return fmt.Errorf("failed to drop migrations: %w", err)
	}

	// Re-run migrations
	if err := ts.migrator.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations after reset: %w", err)
	}

	return nil
}

// CleanupRedis flushes all Redis data
func (ts *TestSuite) CleanupRedis() error {
	ctx := context.Background()
	return ts.RedisClient.FlushDB(ctx).Err()
}

// Close closes all database connections
func (ts *TestSuite) Close() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	var errs []error

	if ts.DB != nil {
		if err := ts.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close postgres: %w", err))
		}
	}

	if ts.RedisClient != nil {
		if err := ts.RedisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close redis: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}
