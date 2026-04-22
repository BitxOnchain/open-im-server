package mgo

import (
	"os"
	"testing"
)

// requireIntegration skips unless OPENIM_INTEGRATION_TEST=1. These tests expect MongoDB
// and related services as wired in the *_test.go files.
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("OPENIM_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENIM_INTEGRATION_TEST=1 to run MongoDB integration tests")
	}
}
