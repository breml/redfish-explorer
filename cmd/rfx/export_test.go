package main

import (
	"io"

	"github.com/breml/redfish-explorer/internal/redfish"
)

// ClientConfigFromArgs parses a command line and returns the client configuration it
// resolves to. It exists for tests only.
func ClientConfigFromArgs(args []string, stdout io.Writer, stderr io.Writer) (redfish.Config, error) {
	cfg, err := parseFlags(args, stdout, stderr)
	if err != nil {
		return redfish.Config{}, err
	}

	return cfg.clientConfig()
}
