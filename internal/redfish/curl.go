package redfish

import (
	"strings"
)

// maskedPassword stands in for the real password in a rendered curl command.
const maskedPassword = "********"

// Curl renders the curl command equivalent to the request rfx makes for
// resource. The flags appear in the order the request actually sets them, so
// the command can be pasted into a shell and reproduce the exchange.
func Curl(cfg Config, resource string) string {
	var b strings.Builder

	b.WriteString("curl -s")

	if cfg.Insecure {
		b.WriteString(" -k")
	}

	b.WriteString(" \\\n  -u ")
	b.WriteString(shellQuote(cfg.Username + ":" + curlPassword(cfg)))
	b.WriteString(" \\\n  -H ")
	b.WriteString(shellQuote("Accept: " + contentTypeJSON))

	if cfg.UserAgent != "" {
		b.WriteString(" \\\n  -H ")
		b.WriteString(shellQuote("User-Agent: " + cfg.UserAgent))
	}

	b.WriteString(" \\\n  ")
	b.WriteString(shellQuote(cfg.Endpoint + resource))

	return b.String()
}

// curlPassword returns the password as it should appear in a curl command.
func curlPassword(cfg Config) string {
	if cfg.ShowPassword {
		return cfg.Password
	}

	return maskedPassword
}

// shellQuote wraps s in single quotes so that a shell passes it through
// unchanged, escaping any single quote it contains.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
