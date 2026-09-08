package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/breml/redfish-explorer/internal/redfish"
)

const (
	// breadcrumbSeparator joins the trail shown on the second header line.
	breadcrumbSeparator = " > "

	// minHintWidth is the narrowest the key hints are worth drawing.
	minHintWidth = 12
)

// renderHeader draws the two header lines: the current path with the service
// metadata, and the breadcrumb trail.
func (m Model) renderHeader(width int) string {
	return m.renderPathLine(width) + "\n" + m.renderBreadcrumb(width)
}

// renderPathLine draws the current path on the left and what the service said
// about the response on the right.
func (m Model) renderPathLine(width int) string {
	left := m.theme.Path.Render(m.current)
	right := m.theme.Meta.Render(m.metadata())

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, width, "…")
	}

	return left + strings.Repeat(" ", gap) + right
}

// metadata summarises the service and the current response.
func (m Model) metadata() string {
	var parts []string

	service := m.service
	if service.RedfishVersion != "" {
		parts = append(parts, "RedfishVersion "+service.RedfishVersion)
	}

	if service.Vendor != "" {
		parts = append(parts, service.Vendor)
	}

	if m.resp != nil {
		parts = append(parts, m.resp.Status, m.resp.Duration.Round(time.Millisecond).String())
	}

	if m.fromCache && m.resp != nil {
		parts = append(parts, "cached "+age(m.resp.FetchedAt)+" ago")
	}

	return strings.Join(parts, " · ")
}

// age renders how long ago an instant was, for the cached marker.
func age(at time.Time) string {
	return time.Since(at).Round(time.Second).String()
}

// renderBreadcrumb draws the trail, eliding from the left when it does not fit:
// the tail is the informative end.
func (m Model) renderBreadcrumb(width int) string {
	trail := strings.Join(redfish.Breadcrumb(m.current), breadcrumbSeparator)

	if lipgloss.Width(trail) > width {
		trail = ansi.TruncateLeft(trail, lipgloss.Width(trail)-width+1, "…")
	}

	return m.theme.Breadcrumb.Render(trail)
}

// renderFooter draws the target of the highlighted link on the left and the key
// hints on the right. Where they do not both fit the hints give way: the target
// is what the user is deciding on, the hints they already know.
func (m Model) renderFooter(width int) string {
	left := m.theme.Dim.Render(ansi.Truncate(m.footerLeft(), width, "…"))
	hints := m.help.ShortHelpView(m.keys.ShortHelp())

	room := width - lipgloss.Width(left) - 1
	if room < minHintWidth {
		return left
	}

	right := m.theme.Hint.Render(ansi.Truncate(hints, room, "…"))

	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)

	return left + strings.Repeat(" ", gap) + right
}

// footerLeft describes what following the highlighted row would do.
func (m Model) footerLeft() string {
	if m.notice != "" {
		return m.notice
	}

	r, ok := m.selectedRow()
	if !ok {
		return ""
	}

	if r.parent {
		if r.disabled {
			return "already at the service root"
		}

		return redfish.Parent(m.current)
	}

	if r.link.JSONPath == "" {
		return r.link.Target
	}

	return r.link.Target + "  " + r.link.JSONPath
}
