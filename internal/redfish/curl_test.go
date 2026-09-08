package redfish_test

import (
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

	noAgent := base
	noAgent.UserAgent = ""

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
			want: "curl -s \\\n" +
				"  -u 'admin:********' \\\n" +
				"  -H 'Accept: application/json' \\\n" +
				"  -H 'User-Agent: rfx/0.1.0' \\\n" +
				"  'https://10.0.0.5/redfish/v1/Systems/1'",
		},
		{
			name:     "insecure adds -k",
			cfg:      insecure,
			resource: "/redfish/v1",
			want: "curl -s -k \\\n" +
				"  -u 'admin:********' \\\n" +
				"  -H 'Accept: application/json' \\\n" +
				"  -H 'User-Agent: rfx/0.1.0' \\\n" +
				"  'https://10.0.0.5/redfish/v1'",
		},
		{
			name:     "show password reveals it",
			cfg:      revealed,
			resource: "/redfish/v1",
			want: "curl -s \\\n" +
				"  -u 'admin:s3cret' \\\n" +
				"  -H 'Accept: application/json' \\\n" +
				"  -H 'User-Agent: rfx/0.1.0' \\\n" +
				"  'https://10.0.0.5/redfish/v1'",
		},
		{
			name:     "single quote in password is escaped",
			cfg:      quoted,
			resource: "/redfish/v1",
			want: "curl -s \\\n" +
				`  -u 'admin:it'\''s' \` + "\n" +
				"  -H 'Accept: application/json' \\\n" +
				"  -H 'User-Agent: rfx/0.1.0' \\\n" +
				"  'https://10.0.0.5/redfish/v1'",
		},
		{
			name:     "no user agent omits the header",
			cfg:      noAgent,
			resource: "/redfish/v1",
			want: "curl -s \\\n" +
				"  -u 'admin:********' \\\n" +
				"  -H 'Accept: application/json' \\\n" +
				"  'https://10.0.0.5/redfish/v1'",
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
