package redfish_test

import (
	"net/http/httptest"
	"os"
	"os/signal"
	"testing"

	"github.com/breml/redfish-explorer/internal/redfish"
	"github.com/breml/redfish-explorer/internal/redfishtest"
)

// fixtureServerEnv opts in to running the long-lived fixture server.
const fixtureServerEnv = "RFX_FIXTURE_SERVER"

// newFixtureServer starts the fixture service for one test.
func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := redfishtest.NewServer()
	t.Cleanup(server.Close)

	return server
}

// fixtureConfig returns a Config pointing at server.
func fixtureConfig(server *httptest.Server) redfish.Config {
	return redfishtest.Config(server)
}

// TestFixtureServer runs the fixture service until interrupted, so that the TUI
// can be driven against it by hand:
//
//	RFX_FIXTURE_SERVER=1 go test ./internal/redfish -run TestFixtureServer -v
//	bin/rfx --host <printed URL> -u admin -p admin -k
func TestFixtureServer(t *testing.T) {
	if os.Getenv(fixtureServerEnv) == "" {
		t.Skipf("set %s=1 to run the fixture server", fixtureServerEnv)
	}

	server := newFixtureServer(t)

	t.Logf("fixture server listening on %s (user %s, password %s)",
		server.URL, redfishtest.User, redfishtest.Password)

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)

	<-interrupted
}
