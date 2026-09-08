package tui_test

import "charm.land/lipgloss/v2"

// lineWidth reports the printable width of a rendered line.
func lineWidth(line string) int {
	return lipgloss.Width(line)
}
