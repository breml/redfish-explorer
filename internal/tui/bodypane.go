package tui

import (
	"bytes"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// renderBody composes the response pane: the status line of the response, its
// headers where they are expanded, then its body. The curl command that
// produced it lives in the header, where it is one line and easy to copy.
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
	b.WriteString(m.renderResponseHeaders())
	b.WriteString("\n")
	b.WriteString(m.renderResponseBody())

	return b.String()
}

// renderResponseHeaders draws the headers when they are expanded, and says how
// many are folded away when they are not. The body is what a Redfish response
// is read for, so the pane opens on it; the headers matter often enough — an
// ETag, a Location, an Allow — that the way back to them has to be visible.
func (m Model) renderResponseHeaders() string {
	if m.showHeaders {
		return m.theme.Header.Render(renderHeaders(m.resp.Header))
	}

	count := countHeaders(m.resp.Header)
	if count == 0 {
		return ""
	}

	return m.theme.Dim.Render(pluralHeaders(count)+" hidden · ") +
		m.theme.Hint.Render(m.keys.Headers.Help().Key) +
		m.theme.Dim.Render(" to show") + "\n"
}

// pluralHeaders names a header count for the folded line.
func pluralHeaders(count int) string {
	if count == 1 {
		return "1 header"
	}

	return strconv.Itoa(count) + " headers"
}

// countHeaders counts header lines, which is not the number of names: a header
// sent more than once renders one line per value.
func countHeaders(header http.Header) int {
	count := 0
	for _, values := range header {
		count += len(values)
	}

	return count
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
