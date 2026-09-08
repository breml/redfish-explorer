// Package tui draws the Redfish explorer: a header naming the current
// location, a pane listing the links found there, and a pane showing the
// request and the response that produced them.
package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Theme holds every style the UI draws with. It is built once and carried on
// the model, because package-level styles are not allowed and a theme is
// easier to vary in tests.
type Theme struct {
	// Header line styles.
	Path       lipgloss.Style
	Breadcrumb lipgloss.Style
	Meta       lipgloss.Style

	// Pane frames.
	PaneFocused lipgloss.Style
	PaneBlurred lipgloss.Style
	PaneTitle   lipgloss.Style

	// Link pane entries.
	Item        lipgloss.Style
	Selected    lipgloss.Style
	GroupHeader lipgloss.Style
	OEM         lipgloss.Style
	OEMSelected lipgloss.Style
	Action      lipgloss.Style
	URI         lipgloss.Style
	HeaderLink  lipgloss.Style

	// Response pane.
	Curl   lipgloss.Style
	Status lipgloss.Style
	Header lipgloss.Style

	// Pretty-printed JSON.
	JSONKey        lipgloss.Style
	JSONAnnotation lipgloss.Style
	JSONString     lipgloss.Style
	JSONLink       lipgloss.Style
	JSONNumber     lipgloss.Style
	JSONBool       lipgloss.Style
	JSONNull       lipgloss.Style
	JSONPunct      lipgloss.Style

	// Shared.
	Dim   lipgloss.Style
	Error lipgloss.Style
	Hint  lipgloss.Style
}

// adaptive pairs a light and a dark colour.
func adaptive(light string, dark string) color.Color {
	return compat.AdaptiveColor{Light: lipgloss.Color(light), Dark: lipgloss.Color(dark)}
}

// NewTheme returns the default theme. OEM entries are deliberately the loudest
// thing on the screen: finding vendor extensions is what rfx is for.
func NewTheme() Theme {
	var (
		text       = adaptive("#1a1a1a", "#e4e4e4")
		muted      = adaptive("#6c6c6c", "#8a8a8a")
		accent     = adaptive("#0057b7", "#7aa2f7")
		oem        = adaptive("#b3005e", "#ff79c6")
		action     = adaptive("#8a6d00", "#e5c07b")
		uri        = adaptive("#00707a", "#56b6c2")
		danger     = adaptive("#b3261e", "#ff6b6b")
		annotation = adaptive("#6b3fa0", "#bb9af7")
		literal    = adaptive("#2a7d2a", "#9ece6a")
		number     = adaptive("#a35200", "#ff9e64")
		selection  = adaptive("#d7e3ff", "#2b3357")
	)

	selected := lipgloss.NewStyle().Background(selection).Bold(true)

	return Theme{
		Path:       lipgloss.NewStyle().Foreground(text).Bold(true),
		Breadcrumb: lipgloss.NewStyle().Foreground(accent),
		Meta:       lipgloss.NewStyle().Foreground(muted),

		PaneFocused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent),
		PaneBlurred: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted),
		PaneTitle:   lipgloss.NewStyle().Foreground(muted).Bold(true),

		Item:        lipgloss.NewStyle().Foreground(text),
		Selected:    selected.Foreground(text),
		GroupHeader: lipgloss.NewStyle().Foreground(muted).Bold(true),
		OEM:         lipgloss.NewStyle().Foreground(oem).Bold(true),
		OEMSelected: selected.Foreground(oem),
		Action:      lipgloss.NewStyle().Foreground(action),
		URI:         lipgloss.NewStyle().Foreground(uri),
		HeaderLink:  lipgloss.NewStyle().Foreground(muted),

		Curl:   lipgloss.NewStyle().Foreground(accent),
		Status: lipgloss.NewStyle().Foreground(text).Bold(true),
		Header: lipgloss.NewStyle().Foreground(muted),

		JSONKey:        lipgloss.NewStyle().Foreground(text).Bold(true),
		JSONAnnotation: lipgloss.NewStyle().Foreground(annotation).Bold(true),
		JSONString:     lipgloss.NewStyle().Foreground(literal),
		JSONLink:       lipgloss.NewStyle().Foreground(accent).Underline(true),
		JSONNumber:     lipgloss.NewStyle().Foreground(number),
		JSONBool:       lipgloss.NewStyle().Foreground(number),
		JSONNull:       lipgloss.NewStyle().Foreground(muted),
		JSONPunct:      lipgloss.NewStyle().Foreground(muted),

		Dim:   lipgloss.NewStyle().Foreground(muted),
		Error: lipgloss.NewStyle().Foreground(danger).Bold(true),
		Hint:  lipgloss.NewStyle().Foreground(muted),
	}
}
