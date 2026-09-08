// Command rfx is a terminal explorer for the Redfish API of a BMC.
//
// It connects to a single Redfish service and lets the user drill down the
// resource tree by following the links found in each response, with a
// particular focus on surfacing vendor OEM extensions.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

// version is replaced at build time via -ldflags "-X main.version=...".
var version = "dev"

const (
	// defaultCacheTTL is how long a visited endpoint stays in the cache.
	defaultCacheTTL = 5 * time.Minute

	// passwordEnvVar keeps the password out of argv and shell history.
	passwordEnvVar = "RFX_PASSWORD"

	// maskedPassword stands in for the real password in rendered curl commands.
	maskedPassword = "********"
)

// errDone reports that a flag such as -version or -help has already produced
// all the output the invocation asked for.
var errDone = errors.New("done")

// config holds everything the command line and the environment contribute.
type config struct {
	host         string
	username     string
	password     string
	insecure     bool
	cacheTTL     time.Duration
	showPassword bool
}

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rfx: %v\n", err)
		os.Exit(1)
	}
}

// run wires the command together and reports the first failure. It exists so
// that os.Exit is confined to main.
func run(args []string, stdout io.Writer, stderr io.Writer) error {
	cfg, err := parseFlags(args, stdout, stderr)
	if err != nil {
		if errors.Is(err, errDone) {
			return nil
		}

		return err
	}

	// The connection and the TUI arrive with the following tasks; until then
	// report the resolved configuration so the wiring can be verified.
	fmt.Fprintf(stderr, "rfx %s: would connect to %s as %s (insecure=%t, cache-ttl=%s, password=%s)\n",
		version, cfg.host, cfg.username, cfg.insecure, cfg.cacheTTL, cfg.displayPassword())

	return nil
}

// displayPassword returns the password as it should appear in user-visible
// output, masked unless the user opted out.
func (c *config) displayPassword() string {
	if c.showPassword {
		return c.password
	}

	return maskedPassword
}

// parseFlags reads the command line and the environment into a config. It
// returns errDone when a flag has already produced the requested output.
func parseFlags(args []string, stdout io.Writer, stderr io.Writer) (config, error) {
	var (
		cfg         config
		showVersion bool
	)

	fs := flag.NewFlagSet("rfx", flag.ContinueOnError)

	// Discard the flag package's own diagnostics: it would print the parse
	// error that run reports again a moment later. Usage still reaches stderr
	// through the closure below, which flag calls on -help and on a bad flag.
	fs.SetOutput(io.Discard)
	fs.Usage = func() { usage(stderr) }

	// Every flag is registered twice so that both the long and the short form
	// resolve to the same variable.
	fs.StringVar(&cfg.host, "host", "", "Redfish host, e.g. 10.0.0.5, 10.0.0.5:8443 or https://10.0.0.5")
	fs.StringVar(&cfg.host, "H", "", "shorthand for -host")
	fs.StringVar(&cfg.username, "username", "", "user name to authenticate with")
	fs.StringVar(&cfg.username, "u", "", "shorthand for -username")
	fs.StringVar(&cfg.password, "password", "", "password; defaults to $"+passwordEnvVar)
	fs.StringVar(&cfg.password, "p", "", "shorthand for -password")
	fs.BoolVar(&cfg.insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&cfg.insecure, "k", false, "shorthand for -insecure")
	fs.DurationVar(
		&cfg.cacheTTL,
		"cache-ttl",
		defaultCacheTTL,
		"how long to cache visited endpoints; 0 disables the cache",
	)
	fs.BoolVar(&cfg.showPassword, "show-password", false, "show the real password in the rendered curl command")
	fs.BoolVar(&showVersion, "version", false, "print the version and exit")

	err := fs.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config{}, errDone
		}

		return config{}, fmt.Errorf("parsing flags: %w", err)
	}

	if showVersion {
		fmt.Fprintf(stdout, "rfx %s\n", version)

		return config{}, errDone
	}

	err = cfg.resolve()
	if err != nil {
		usage(stderr)

		return config{}, err
	}

	return cfg, nil
}

// resolve fills in what the flags left open and rejects an incomplete config.
func (c *config) resolve() error {
	if c.host == "" {
		return errors.New("no host given, use -host")
	}

	if c.username == "" {
		return errors.New("no user name given, use -username")
	}

	// An empty password is never assumed: an anonymous request that happens to
	// succeed is more confusing than a clear error.
	if c.password == "" {
		c.password = os.Getenv(passwordEnvVar)
	}

	if c.password == "" {
		return fmt.Errorf("no password given, use -password or set $%s", passwordEnvVar)
	}

	return nil
}

// usage writes the help text. It is deliberately hand-written rather than
// generated, so that the short and long form of each flag appear together.
func usage(w io.Writer) {
	fmt.Fprint(w, `rfx explores the Redfish API of a BMC.

Usage:
  rfx --host <host> --username <user> [--password <password>] [flags]

Flags:
  -H, --host <host>          Redfish host, e.g. 10.0.0.5, 10.0.0.5:8443 or https://10.0.0.5
  -u, --username <user>      user name to authenticate with
  -p, --password <password>  password; defaults to $RFX_PASSWORD
  -k, --insecure             skip TLS certificate verification
      --cache-ttl <d>        how long to cache visited endpoints (default 5m0s, 0 disables)
      --show-password        show the real password in the rendered curl command
      --version              print the version and exit

The password is visible in ps and in the shell history when passed as a flag;
set $RFX_PASSWORD instead to avoid that.
`)
}
