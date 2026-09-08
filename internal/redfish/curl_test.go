package redfish_test

import (
	"strings"
	"testing"

	"github.com/breml/redfish-explorer/internal/redfish"
)

func TestCurl(t *testing.T) {
	t.Parallel()

	base := redfish.Config{
		Endpoint:  "https://10.0.0.5",
		Username:  "admin",
		Password:  "s3cret",
		UserAgent: "rfx/0.1.0",
	}

	insecure := base
	insecure.Insecure = true

	revealed := base
	revealed.ShowPassword = true

	quoted := base
	quoted.Password = "it's"
	quoted.ShowPassword = true

	tests := []struct {
		name     string
		cfg      redfish.Config
		resource string
		want     string
	}{
		{
			name:     "password is masked by default",
			cfg:      base,
			resource: "/redfish/v1/Systems/1",
			want: "curl -s -u 'admin:********' -H 'Accept: application/json' " +
				"'https://10.0.0.5/redfish/v1/Systems/1'",
		},
		{
			name:     "insecure adds -k",
			cfg:      insecure,
			resource: "/redfish/v1",
			want: "curl -s -k -u 'admin:********' -H 'Accept: application/json' " +
				"'https://10.0.0.5/redfish/v1'",
		},
		{
			name:     "show password reveals it",
			cfg:      revealed,
			resource: "/redfish/v1",
			want: "curl -s -u 'admin:s3cret' -H 'Accept: application/json' " +
				"'https://10.0.0.5/redfish/v1'",
		},
		{
			name:     "single quote in password is escaped",
			cfg:      quoted,
			resource: "/redfish/v1",
			want: `curl -s -u 'admin:it'\''s' -H 'Accept: application/json' ` +
				"'https://10.0.0.5/redfish/v1'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := redfish.Curl(test.cfg, test.resource)
			if got != test.want {
				t.Errorf("Curl() =\n%s\n\nwant:\n%s", got, test.want)
			}
		})
	}
}

// The command has to survive being pasted into a shell as one line, and the
// User-Agent rfx sets on the real request is deliberately not part of it.
func TestCurlIsOneLineWithoutTheUserAgent(t *testing.T) {
	t.Parallel()

	cfg := redfish.Config{
		Endpoint:  "https://10.0.0.5",
		Username:  "admin",
		Password:  "s3cret",
		UserAgent: "rfx/0.1.0",
	}

	got := redfish.Curl(cfg, "/redfish/v1")

	if strings.ContainsAny(got, "\n\\") {
		t.Errorf("Curl() = %q, want a single line with no continuations", got)
	}

	if strings.Contains(got, "User-Agent") {
		t.Errorf("Curl() = %q, want no User-Agent header", got)
	}
}
