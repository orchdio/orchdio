package integration_test

import (
	"log"
	"orchdio/testutils"
	"os"
	"testing"
)

var SharedTestDeps *testutils.TestSuite

func TestMain(m *testing.M) {
	SharedTestDeps = testutils.SetupTestSuite()
	log.Println("Test suite initialized")

	exitCode := m.Run()

	log.Println("Running global cleanup...")
	_, err := SharedTestDeps.DB.Exec("TRUNCATE apps, follows, organizations, tasks, user_apps, users, waitlists CASCADE")
	if err != nil {
		log.Printf("ERROR TRUNCATING TABLES: %v", err)
	} else {
		log.Println("Database cleaned successfully")
	}

	os.Exit(exitCode)
}
