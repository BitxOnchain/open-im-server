package redis

import (
	"os"
	"testing"
)

// requireIntegration skips unless OPENIM_INTEGRATION_TEST=1. These tests expect Redis/Mongo
// (see im-service docker-compose and config defaults).
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("OPENIM_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENIM_INTEGRATION_TEST=1 to run Redis integration tests (requires reachable hosts in this package)")
	}
}
