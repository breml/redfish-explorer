package tui

import (
	"bytes"
	"net/http"
	"slices"
	"strings"

	"github.com/breml/redfish-explorer/internal/redfish"
)

// renderBody composes the response pane: the curl command that produced the
// response, then its status and headers, then its body.
func (m Model) renderBody() string {
	var b strings.Builder

	// On a failure the command shown is the one that failed, not the one for
	// the location the user is still standing on: it is there to be retried.
	b.WriteString(m.theme.Curl.Render(redfish.Curl(m.cfg, m.curlResource())))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(m.theme.Error.Render(m.err.Error()))

		return b.String()
	}

	if m.resp == nil {
		b.WriteString(m.theme.Dim.Render("no response yet"))

		return b.String()
	}

	b.WriteString(m.theme.Status.Render(m.resp.Proto + " " + m.resp.Status))
	b.WriteString("\n")
	b.WriteString(m.theme.Header.Render(renderHeaders(m.resp.Header)))
	b.WriteString("\n")
	b.WriteString(m.renderResponseBody())

	return b.String()
}

// curlResource is the resource the rendered curl command addresses.
func (m Model) curlResource() string {
	if m.err != nil && m.errResource != "" {
		return m.errResource
	}

	return m.current
}

// renderHeaders lists the response headers, one per line, in a stable order.
func renderHeaders(header http.Header) string {
	names := make([]string, 0, len(header))
	for name := range header {
		names = append(names, name)
	}

	slices.Sort(names)

	var b strings.Builder

	for _, name := range names {
		for _, value := range header[name] {
			b.WriteString(name)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderResponseBody pretty-prints the body, or shows it verbatim when it is
// not JSON. Some BMCs answer with an HTML error page, and seeing that raw is
// exactly what is wanted.
func (m Model) renderResponseBody() string {
	body := m.resp.Body
	if len(bytes.TrimSpace(body)) == 0 {
		return m.theme.Dim.Render("empty response body")
	}

	pretty, ok := Pretty(body)
	if !ok {
		return m.theme.Error.Render("⚠ response body is not valid JSON") + "\n" + string(body)
	}

	return Colorize(pretty, m.theme)
}
