package tui

import (
	"bytes"
	"net/http"
	"slices"
	"strings"
)

// renderBody composes the response pane: the status and headers of the
// response, then its body. The curl command that produced it lives in the
// header, where it is one line and easy to copy.
func (m Model) renderBody() string {
	var b strings.Builder

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
