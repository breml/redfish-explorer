package redfish

import (
	"strings"
)

// maskedPassword stands in for the real password in a rendered curl command.
const maskedPassword = "********"

// Curl renders the curl command equivalent to the request rfx makes for
// resource, with the password masked unless Config.ShowPassword says otherwise.
// It is one line so that it can be selected and copied in a single gesture,
// which is what it is for: pasting into a bug report or a script.
//
// The User-Agent rfx sets on the real request (see Client.Fetch) is left out.
// curl sends its own, the header plays no part in what a service answers, and
// carrying it would only make the command harder to read and to copy.
func Curl(cfg Config, resource string) string {
	return renderCurl(cfg, resource, curlPassword(cfg))
}

// CurlWithPassword renders the same command as Curl but always with the real
// password, whatever Config.ShowPassword says. It is what a copy puts on the
// clipboard: masking is there to keep the password off a screen that may be
// shared, and a clipboard is not a screen. A copied command that has to be
// hand-edited before it runs is of no use to anyone.
func CurlWithPassword(cfg Config, resource string) string {
	return renderCurl(cfg, resource, cfg.Password)
}

// renderCurl renders the command carrying the given password.
func renderCurl(cfg Config, resource string, password string) string {
	var b strings.Builder

	b.WriteString("curl -s")

	if cfg.Insecure {
		b.WriteString(" -k")
	}

	b.WriteString(" -u ")
	b.WriteString(shellQuote(cfg.Username + ":" + password))
	b.WriteString(" -H ")
	b.WriteString(shellQuote("Accept: " + contentTypeJSON))
	b.WriteString(" ")
	b.WriteString(shellQuote(cfg.Endpoint + resource))

	return b.String()
}

// curlPassword returns the password as it should appear on screen.
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
