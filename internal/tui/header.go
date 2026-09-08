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

// renderHeader draws the three header lines: the current path with the service
// metadata, the curl command that fetches it, and the breadcrumb trail. While
// the location is being edited the first line becomes the editor and the third
// explains it; the count stays at three either way, so that opening the editor
// does not shift the panes below.
func (m Model) renderHeader(width int) string {
	if m.mode == ModeEdit {
		return m.editor.View() + "\n" +
			m.renderCurlLine(width) + "\n" +
			m.renderEditHint(width)
	}

	return m.renderPathLine(width) + "\n" +
		m.renderCurlLine(width) + "\n" +
		m.renderBreadcrumb(width)
}

// renderCurlLine draws the curl command for the current location. It sits
// between the path and the breadcrumb, on one line, so that it can be selected
// and copied in a single gesture.
func (m Model) renderCurlLine(width int) string {
	command := redfish.Curl(m.cfg, m.curlResource())

	return m.theme.Curl.Render(ansi.Truncate(command, width, "…"))
}

// curlResource is the resource the rendered curl command addresses. On a
// failure it is the one that failed, not the location the user is still
// standing on: the command is there to be retried.
func (m Model) curlResource() string {
	if m.err != nil && m.errResource != "" {
		return m.errResource
	}

	return m.current
}

// renderEditHint says what the editor accepts, or why the entry was refused.
func (m Model) renderEditHint(width int) string {
	if m.editErr != "" {
		return m.theme.Error.Render(ansi.Truncate(m.editErr, width, "…"))
	}

	return m.theme.Dim.Render(ansi.Truncate("enter load · esc cancel", width, "…"))
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

	if m.loading {
		parts = append(parts, m.spinner.View()+" loading")
	}

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
