package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/breml/redfish-explorer/internal/redfish"
)

const (
	// parentLabel is the entry that walks one level up. The brief puts it first.
	parentLabel = ".."

	// oemSuffix marks an OEM entry, alongside its colour.
	oemSuffix = "(oem)"

	// actionMarker marks a POST-only action target.
	actionMarker = "⚡"

	// cursorMarker points at the selected row.
	cursorMarker = "▸ "

	// rowIndent aligns unselected rows with selected ones.
	rowIndent = "  "
)

// rowState says whether a row carries the cursor.
type rowState int

const (
	// rowNormal is an ordinary row.
	rowNormal rowState = iota
	// rowSelected is the row under the cursor.
	rowSelected
)

// stateOf reports how a row at index i should be drawn.
func (m Model) stateOf(i int) rowState {
	if i == m.cursor {
		return rowSelected
	}

	return rowNormal
}

// row is one line of the link pane. Group headers and the "no links" notice are
// rows too, so that scrolling and rendering deal with a single list, but the
// cursor skips them.
type row struct {
	// header marks a group heading.
	header bool
	// title is the heading text, when header is set.
	title string
	// oem marks a heading that introduces vendor extensions.
	oem bool
	// parent marks the ".." entry.
	parent bool
	// link is the link on this row, when it is neither a header nor "..".
	link redfish.Link
	// disabled marks a row the cursor may land on but that leads nowhere.
	disabled bool
}

// selectable reports whether the cursor may rest on this row.
func (r row) selectable() bool {
	return !r.header
}

// buildRows flattens the groups into the lines of the link pane.
func buildRows(groups []redfish.Group, atRoot bool) []row {
	rows := []row{{parent: true, disabled: atRoot}}

	for _, group := range groups {
		rows = append(rows, row{header: true, title: group.Title, oem: group.OEM})

		for _, link := range group.Links {
			rows = append(rows, row{link: link})
		}
	}

	return rows
}

// countLinks reports how many rows the cursor can rest on, excluding "..".
func countLinks(rows []row) int {
	count := 0

	for _, r := range rows {
		if r.selectable() && !r.parent {
			count++
		}
	}

	return count
}

// renderLinkPane draws the link pane body, scrolled so that the cursor is
// visible.
func (m Model) renderLinkPane(width int, height int) string {
	// The parent row is drawn whatever went wrong: the cursor is sitting on it,
	// and it is the only way back up.
	if m.linkErr != nil {
		return m.renderRow(m.rows[0], m.stateOf(0), width) + "\n" +
			m.theme.Dim.Render("no links: "+m.linkErr.Error())
	}

	if countLinks(m.rows) == 0 {
		return m.renderRow(m.rows[0], m.stateOf(0), width) + "\n" +
			m.theme.Dim.Render("no links in this resource")
	}

	start := m.linkScroll(height)

	lines := make([]string, 0, height)

	for i := start; i < len(m.rows) && len(lines) < height; i++ {
		lines = append(lines, m.renderRow(m.rows[i], m.stateOf(i), width))
	}

	return strings.Join(lines, "\n")
}

// linkScroll returns the first row to draw, keeping the cursor on screen.
func (m Model) linkScroll(height int) int {
	if height <= 0 || m.cursor < height {
		return 0
	}

	start := m.cursor - height + 1

	last := len(m.rows) - height
	if start > last {
		start = last
	}

	return max(start, 0)
}

// renderRow draws one line of the link pane.
func (m Model) renderRow(r row, state rowState, width int) string {
	if r.header {
		return m.renderGroupHeader(r, width)
	}

	label, style := m.rowLabel(r, state)

	prefix := rowIndent
	if state == rowSelected {
		prefix = cursorMarker
	}

	return prefix + style.Render(ansi.Truncate(label, max(width-lipgloss.Width(prefix), 0), "…"))
}

// rowLabel returns the text of a row and the style to draw it in.
func (m Model) rowLabel(r row, state rowState) (label string, style lipgloss.Style) {
	if r.parent {
		if r.disabled {
			return parentLabel, m.theme.Dim
		}

		return parentLabel, m.selectionStyle(m.theme.Item, state)
	}

	text := r.link.Label
	if r.link.Kind == redfish.KindAction {
		text += " " + actionMarker
	}

	if r.link.OEM {
		return text + " " + oemSuffix, m.selectionStyle(m.theme.OEM, state)
	}

	return text, m.selectionStyle(m.kindStyle(r.link.Kind), state)
}

// selectionStyle highlights a style when its row carries the cursor.
func (m Model) selectionStyle(base lipgloss.Style, state rowState) lipgloss.Style {
	if state != rowSelected {
		return base
	}

	return base.Background(m.theme.Selected.GetBackground()).Bold(true)
}

// kindStyle picks the colour for a link by what it points at.
func (m Model) kindStyle(kind redfish.Kind) lipgloss.Style {
	switch kind {
	case redfish.KindAction:
		return m.theme.Action

	case redfish.KindURI:
		return m.theme.URI

	case redfish.KindHeader:
		return m.theme.HeaderLink

	case redfish.KindResource:
		return m.theme.Item
	}

	return m.theme.Item
}

// renderGroupHeader draws a group heading, filling the line with a rule.
func (m Model) renderGroupHeader(r row, width int) string {
	style := m.theme.GroupHeader
	if r.oem {
		style = m.theme.OEM
	}

	title := " " + r.title + " "

	rule := max(width-lipgloss.Width(title)-2, 0)

	return style.Render(ansi.Truncate("──"+title+strings.Repeat("─", rule), width, "…"))
}

// linkPaneTitle names the pane and counts the links it holds.
func (m Model) linkPaneTitle() string {
	return "Links (" + strconv.Itoa(countLinks(m.rows)) + ")"
}
