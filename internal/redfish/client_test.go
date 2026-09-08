package redfish_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/breml/redfish-explorer/internal/redfish"
)

// connect dials the fixture server and fails the test if it cannot.
func connect(t *testing.T, cfg redfish.Config) *redfish.Client {
	t.Helper()

	client, err := redfish.Connect(t.Context(), cfg)
	if err != nil {
		t.Fatalf("Connect: unexpected error: %v", err)
	}

	t.Cleanup(client.Close)

	return client
}

func TestConnectReadsServiceRoot(t *testing.T) {
	t.Parallel()

	client := connect(t, fixtureConfig(newFixtureServer(t)))

	service := client.Service()
	if service.RedfishVersion != "1.18.0" {
		t.Errorf("RedfishVersion = %q, want %q", service.RedfishVersion, "1.18.0")
	}

	if service.Vendor != "Contoso" {
		t.Errorf("Vendor = %q, want %q", service.Vendor, "Contoso")
	}

	if len(service.OEMVendors) != 1 || service.OEMVendors[0] != "Contoso" {
		t.Errorf("OEMVendors = %v, want [Contoso]", service.OEMVendors)
	}
}

func TestConnectRejectsUnverifiedCertificate(t *testing.T) {
	t.Parallel()

	cfg := fixtureConfig(newFixtureServer(t))
	cfg.Insecure = false

	_, err := redfish.Connect(t.Context(), cfg)
	if err == nil {
		t.Fatal("Connect: want an error for an unverified certificate")
	}

	// The message has to name the way out, a bare x509 error does not.
	if !strings.Contains(err.Error(), "--insecure") {
		t.Errorf("Connect error = %q, want it to mention --insecure", err)
	}
}

func TestConnectRejectsWrongCredentials(t *testing.T) {
	t.Parallel()

	cfg := fixtureConfig(newFixtureServer(t))
	cfg.Password = "wrong"

	_, err := redfish.Connect(t.Context(), cfg)
	if err == nil {
		t.Fatal("Connect: want an error for wrong credentials")
	}

	if !strings.Contains(err.Error(), "--password") {
		t.Errorf("Connect error = %q, want it to mention --password", err)
	}
}

func TestFetchSuccess(t *testing.T) {
	t.Parallel()

	client := connect(t, fixtureConfig(newFixtureServer(t)))

	resp, err := client.Fetch(t.Context(), "/redfish/v1/Systems/1")
	if err != nil {
		t.Fatalf("Fetch: unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if resp.Path != "/redfish/v1/Systems/1" {
		t.Errorf("Path = %q, want %q", resp.Path, "/redfish/v1/Systems/1")
	}

	if got := resp.Header.Get("OData-Version"); got != "4.0" {
		t.Errorf("OData-Version header = %q, want %q", got, "4.0")
	}

	if !json.Valid(resp.Body) {
		t.Errorf("Body is not valid JSON: %s", resp.Body)
	}

	if !strings.Contains(string(resp.Body), `"@odata.id": "/redfish/v1/Systems/1"`) {
		t.Errorf("Body does not contain the expected self link: %s", resp.Body)
	}

	if resp.Duration <= 0 {
		t.Errorf("Duration = %v, want a positive duration", resp.Duration)
	}
}

func TestFetchEmptyResourceIsServiceRoot(t *testing.T) {
	t.Parallel()

	client := connect(t, fixtureConfig(newFixtureServer(t)))

	resp, err := client.Fetch(t.Context(), "")
	if err != nil {
		t.Fatalf("Fetch: unexpected error: %v", err)
	}

	if resp.Path != redfish.RootPath {
		t.Errorf("Path = %q, want %q", resp.Path, redfish.RootPath)
	}
}

func TestFetchKeepsErrorResponses(t *testing.T) {
	t.Parallel()

	client := connect(t, fixtureConfig(newFixtureServer(t)))

	tests := []struct {
		name       string
		resource   string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "redfish error body survives a 404",
			resource:   "/redfish/v1/DoesNotExist",
			wantStatus: http.StatusNotFound,
			wantBody:   "Base.1.18.ResourceMissingAtURI",
		},
		{
			name:       "non-json body comes through untouched",
			resource:   brokenPath,
			wantStatus: http.StatusInternalServerError,
			wantBody:   "<h1>Internal Server Error</h1>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// An error status must not become a Go error: showing the body is
			// the whole point.
			resp, err := client.Fetch(t.Context(), test.resource)
			if err != nil {
				t.Fatalf("Fetch: unexpected error: %v", err)
			}

			if resp.StatusCode != test.wantStatus {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, test.wantStatus)
			}

			if !strings.Contains(string(resp.Body), test.wantBody) {
				t.Errorf("Body = %s, want it to contain %q", resp.Body, test.wantBody)
			}

			if resp.Header.Get("Content-Type") == "" {
				t.Error("Content-Type header is missing")
			}
		})
	}
}

func TestFetchRejectsResourcesOffTheEndpoint(t *testing.T) {
	t.Parallel()

	client := connect(t, fixtureConfig(newFixtureServer(t)))

	tests := []struct {
		name     string
		resource string
	}{
		{name: "another host", resource: "https://192.0.2.1/redfish/v1"},
		{name: "relative path", resource: "Systems/1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.Fetch(t.Context(), test.resource)
			if err == nil {
				t.Fatalf("Fetch(%q): want an error", test.resource)
			}
		})
	}
}

func TestFetchHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	client := connect(t, fixtureConfig(newFixtureServer(t)))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := client.Fetch(ctx, redfish.RootPath)
	if err == nil {
		t.Fatal("Fetch: want an error for a cancelled context")
	}
}
