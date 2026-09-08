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

// NewServer starts a TLS server answering from the embedded fixtures. The
// caller closes it. The service root is readable without credentials, as the
// Redfish specification requires; everything below it is protected.
func NewServer() *httptest.Server {
	notFound := mustRead("notfound.json")
	serverError := mustRead("servererror.html")

	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isPublic(r.URL.Path) && !authorized(r) {
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
	}))
}

// Config returns a client configuration pointing at a fixture server.
func Config(server *httptest.Server) redfish.Config {
	return redfish.Config{
		Endpoint:  server.URL,
		Username:  User,
		Password:  Password,
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
