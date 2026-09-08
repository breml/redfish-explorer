package redfish_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"

	"github.com/breml/redfish-explorer/internal/redfish"
)

const (
	// fixtureUser and fixturePassword are the credentials the fixture service
	// accepts.
	fixtureUser     = "admin"
	fixturePassword = "admin"

	// fixtureServerEnv opts in to running the long-lived fixture server.
	fixtureServerEnv = "RFX_FIXTURE_SERVER"

	// treeDir holds the fixture resource tree, mirroring the URL layout.
	treeDir = "testdata/tree"

	// brokenPath answers with a non-JSON body, as some BMCs do on failure.
	brokenPath = "/redfish/v1/Broken"
)

// newFixtureServer starts a TLS server answering from the fixture tree. It
// requires basic auth and reports a Redfish error body for unknown resources,
// so that both the happy path and the failure paths can be exercised.
func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	notFound := readFile(t, "testdata/notfound.json")
	serverError := readFile(t, "testdata/servererror.html")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redfish requires the service root to be readable without
		// credentials; everything below it is protected.
		if !isPublic(r.URL.Path) && !authorized(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="redfish"`)
			write(w, http.StatusUnauthorized, "application/json", notFound)

			return
		}

		if r.URL.Path == brokenPath {
			write(w, http.StatusInternalServerError, "text/html", serverError)

			return
		}

		body, err := readFixture(r.URL.Path)
		if err != nil {
			write(w, http.StatusNotFound, "application/json", notFound)

			return
		}

		w.Header().Set("OData-Version", "4.0")
		write(w, http.StatusOK, "application/json;charset=utf-8", body)
	}))

	t.Cleanup(server.Close)

	return server
}

// isPublic reports whether a path is readable without authentication, as the
// Redfish specification requires for the service root.
func isPublic(urlPath string) bool {
	trimmed := strings.TrimRight(urlPath, "/")

	return trimmed == redfish.RootPath || trimmed == "/redfish"
}

// authorized reports whether the request carries the fixture credentials.
func authorized(r *http.Request) bool {
	user, password, ok := r.BasicAuth()

	return ok && user == fixtureUser && password == fixturePassword
}

// write sends one fixture response.
func write(w http.ResponseWriter, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// readFile loads a fixture or fails the test.
func readFile(t *testing.T, name string) []byte {
	t.Helper()

	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}

	return body
}

// readFixture maps a URL path onto the fixture tree on disk.
func readFixture(urlPath string) ([]byte, error) {
	clean := filepath.Clean("/" + strings.Trim(urlPath, "/"))

	body, err := os.ReadFile(filepath.Join(treeDir, clean, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("reading fixture for %q: %w", urlPath, err)
	}

	return body, nil
}

// fixtureConfig returns a Config pointing at server.
func fixtureConfig(server *httptest.Server) redfish.Config {
	return redfish.Config{
		Endpoint:  server.URL,
		Username:  fixtureUser,
		Password:  fixturePassword,
		Insecure:  true,
		UserAgent: "rfx/test",
	}
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

	t.Logf("fixture server listening on %s (user %s, password %s)", server.URL, fixtureUser, fixturePassword)

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)

	<-interrupted
}
