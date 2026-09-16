// Package redfishtest serves a small Redfish service from embedded fixtures, so
// that the client and the UI can be exercised without a BMC.
package redfishtest

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"

	"github.com/breml/redfish-explorer/internal/redfish"
)

// Credentials the fixture service accepts.
const (
	// User is the fixture user name.
	User = "admin"
	// Password is the fixture password.
	Password = "admin"
)

// BrokenPath answers with a non-JSON body, as some BMCs do on failure.
const BrokenPath = "/redfish/v1/Broken"

// fixtures holds the resource tree and the two error bodies. The tree mirrors
// the URL layout, so a resource is just a file.
//
//go:embed testdata
var fixtures embed.FS

// fixtureRoot is where the embedded files sit.
const fixtureRoot = "testdata"

// gate reports whether a request may be answered as it stands.
type gate func(r *http.Request) bool

// NewServer starts a TLS server answering from the embedded fixtures. The
// caller closes it. The service root is readable without credentials, as the
// Redfish specification requires; everything below it is protected.
func NewServer() *httptest.Server {
	return httptest.NewTLSServer(handler(protected))
}

// NewOpenServer starts a TLS server answering the same fixtures to anyone, as
// the Redfish services that require no authentication do. The caller closes it.
func NewOpenServer() *httptest.Server {
	return httptest.NewTLSServer(handler(open))
}

// handler serves the fixtures behind the given gate.
func handler(allowed gate) http.Handler {
	notFound := mustRead("notfound.json")
	serverError := mustRead("servererror.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="redfish"`)
			write(w, http.StatusUnauthorized, "application/json", notFound)

			return
		}

		if r.URL.Path == BrokenPath {
			write(w, http.StatusInternalServerError, "text/html", serverError)

			return
		}

		body, err := resource(r.URL.Path)
		if err != nil {
			write(w, http.StatusNotFound, "application/json", notFound)

			return
		}

		w.Header().Set("OData-Version", "4.0")
		write(w, http.StatusOK, "application/json;charset=utf-8", body)
	})
}

// protected admits the public resources and any request carrying the fixture
// credentials.
func protected(r *http.Request) bool {
	return isPublic(r.URL.Path) || authorized(r)
}

// open admits everything.
func open(_ *http.Request) bool {
	return true
}

// Config returns a client configuration pointing at a fixture server.
func Config(server *httptest.Server) redfish.Config {
	cfg := AnonymousConfig(server)
	cfg.Username = User
	cfg.Password = Password

	return cfg
}

// AnonymousConfig returns a client configuration carrying no credentials, for
// exercising a service that asks for none.
func AnonymousConfig(server *httptest.Server) redfish.Config {
	return redfish.Config{
		Endpoint:  server.URL,
		Insecure:  true,
		UserAgent: "rfx/test",
	}
}

// Fixture returns one embedded file by its path below testdata.
func Fixture(name string) ([]byte, error) {
	body, err := fixtures.ReadFile(path.Join(fixtureRoot, name))
	if err != nil {
		return nil, fs.ErrNotExist
	}

	return body, nil
}

// resource maps a URL path onto the embedded tree.
func resource(urlPath string) ([]byte, error) {
	clean := path.Clean("/" + strings.Trim(urlPath, "/"))

	return Fixture(path.Join("tree", clean, "index.json"))
}

// isPublic reports whether a path is readable without authentication.
func isPublic(urlPath string) bool {
	trimmed := strings.TrimRight(urlPath, "/")

	return trimmed == redfish.RootPath || trimmed == "/redfish"
}

// authorized reports whether a request carries the fixture credentials.
func authorized(r *http.Request) bool {
	user, password, ok := r.BasicAuth()

	return ok && user == User && password == Password
}

// write sends one fixture response.
func write(w http.ResponseWriter, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// mustRead loads an embedded fixture that is known to exist.
func mustRead(name string) []byte {
	body, err := Fixture(name)
	if err != nil {
		panic("redfishtest: missing embedded fixture " + name)
	}

	return body
}
