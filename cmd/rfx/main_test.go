package main_test

import (
	"io"
	"strings"
	"testing"

	main "github.com/breml/redfish-explorer/cmd/rfx"
)

// Credentials are optional, because not every Redfish service asks for them,
// but half a pair is always a mistake.
func TestClientConfigFromArgsCredentials(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		env          string
		wantUsername string
		wantPassword string
		wantErr      string
	}{
		{
			name:         "user name and password",
			args:         []string{"-H", "10.0.0.5", "-u", "admin", "-p", "s3cret"},
			wantUsername: "admin",
			wantPassword: "s3cret",
		},
		{
			name: "neither is anonymous",
			args: []string{"-H", "10.0.0.5"},
		},
		{
			name:         "password from the environment",
			args:         []string{"-H", "10.0.0.5", "-u", "admin"},
			env:          "s3cret",
			wantUsername: "admin",
			wantPassword: "s3cret",
		},
		{
			name:    "user name without a password",
			args:    []string{"-H", "10.0.0.5", "-u", "admin"},
			wantErr: "no password given",
		},
		{
			name:    "password without a user name",
			args:    []string{"-H", "10.0.0.5", "-p", "s3cret"},
			wantErr: "without a user name",
		},
		{
			name:    "environment password without a user name",
			args:    []string{"-H", "10.0.0.5"},
			env:     "s3cret",
			wantErr: "without a user name",
		},
		{
			name:    "no host",
			args:    []string{"-u", "admin", "-p", "s3cret"},
			wantErr: "no host given",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("RFX_PASSWORD", test.env)

			cfg, err := main.ClientConfigFromArgs(test.args, io.Discard, io.Discard)

			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("ClientConfigFromArgs: want an error containing %q", test.wantErr)
				}

				if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("ClientConfigFromArgs error = %q, want it to contain %q", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ClientConfigFromArgs: unexpected error: %v", err)
			}

			if cfg.Username != test.wantUsername {
				t.Errorf("Username = %q, want %q", cfg.Username, test.wantUsername)
			}

			if cfg.Password != test.wantPassword {
				t.Errorf("Password = %q, want %q", cfg.Password, test.wantPassword)
			}

			if cfg.Authenticated() != (test.wantUsername != "") {
				t.Errorf("Authenticated() = %t, want %t", cfg.Authenticated(), test.wantUsername != "")
			}
		})
	}
}
