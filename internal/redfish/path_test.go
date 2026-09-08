package redfish_test

import (
	"testing"

	"github.com/breml/redfish-explorer/internal/redfish"
)

func TestNormalizeHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "bare host", input: "10.0.0.5", want: "https://10.0.0.5"},
		{name: "host and port", input: "10.0.0.5:8443", want: "https://10.0.0.5:8443"},
		{name: "host name", input: "bmc.example.com", want: "https://bmc.example.com"},
		{name: "explicit https", input: "https://10.0.0.5", want: "https://10.0.0.5"},
		{name: "explicit http", input: "http://10.0.0.5:8000", want: "http://10.0.0.5:8000"},
		{name: "trailing slash", input: "https://10.0.0.5/", want: "https://10.0.0.5"},
		{name: "pasted service root", input: "https://10.0.0.5/redfish/v1", want: "https://10.0.0.5"},
		{name: "pasted resource", input: "https://10.0.0.5/redfish/v1/Systems/1", want: "https://10.0.0.5"},
		{name: "query and fragment", input: "https://10.0.0.5/redfish/v1?x=1#f", want: "https://10.0.0.5"},
		{name: "surrounding space", input: "  10.0.0.5  ", want: "https://10.0.0.5"},
		{name: "ipv6 with port", input: "[::1]:8443", want: "https://[::1]:8443"},
		{name: "empty", input: "", wantErr: true},
		{name: "only whitespace", input: "   ", wantErr: true},
		{name: "path only", input: "/redfish/v1", wantErr: true},
		{name: "unsupported scheme", input: "ftp://10.0.0.5", wantErr: true},
		{name: "invalid port", input: "10.0.0.5:notaport", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := redfish.NormalizeHost(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("NormalizeHost(%q) = %q, want an error", test.input, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("NormalizeHost(%q): unexpected error: %v", test.input, err)
			}

			if got != test.want {
				t.Errorf("NormalizeHost(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestParent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "nested resource", input: "/redfish/v1/Systems/1/SecureBoot", want: "/redfish/v1/Systems/1"},
		{name: "collection member", input: "/redfish/v1/Systems/1", want: "/redfish/v1/Systems"},
		{name: "collection", input: "/redfish/v1/Systems", want: "/redfish/v1"},
		{name: "root stays root", input: "/redfish/v1", want: "/redfish/v1"},
		{name: "root with slash", input: "/redfish/v1/", want: "/redfish/v1"},
		{name: "trailing slash", input: "/redfish/v1/Systems/1/", want: "/redfish/v1/Systems"},
		{name: "above root", input: "/redfish", want: "/redfish/v1"},
		{name: "empty", input: "", want: "/redfish/v1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := redfish.Parent(test.input)
			if got != test.want {
				t.Errorf("Parent(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestBreadcrumb(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "root", input: "/redfish/v1", want: []string{"root"}},
		{name: "root with slash", input: "/redfish/v1/", want: []string{"root"}},
		{
			name:  "nested resource",
			input: "/redfish/v1/Systems/1/SecureBoot",
			want:  []string{"root", "Systems", "1", "SecureBoot"},
		},
		{
			name:  "percent encoded segment",
			input: "/redfish/v1/Managers/BMC%20One",
			want:  []string{"root", "Managers", "BMC One"},
		},
		{
			name:  "trailing slash",
			input: "/redfish/v1/Chassis/1/",
			want:  []string{"root", "Chassis", "1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := redfish.Breadcrumb(test.input)
			if len(got) != len(test.want) {
				t.Fatalf("Breadcrumb(%q) = %v, want %v", test.input, got, test.want)
			}

			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("Breadcrumb(%q) = %v, want %v", test.input, got, test.want)
				}
			}
		})
	}
}
