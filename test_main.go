//go:build integration
// +build integration

package main

import (
	"flag"
	"log"
	"orchdio/testutils"
	"os"
	"testing"
)

var forceCleanup = flag.Bool("force-cleanup", false, "Force cleanup of all containers after tests")

func TestMain(m *testing.M) {
	flag.Parse()

	log.Println("Starting integration tests...")

	// Global test setup can go here if needed
	code := m.Run()

	// Cleanup based on flags
	if *forceCleanup {
		log.Println("Force cleanup requested - terminating all containers")
		testutils.ForceCleanupContainers()
	} else {
		log.Println("Normal cleanup - preserving containers for reuse")
		testutils.CleanupGlobalInfrastructure()
	}

	log.Printf("Integration tests completed with code: %d", code)
	os.Exit(code)
}
