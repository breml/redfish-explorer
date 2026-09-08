package tui

import "charm.land/bubbles/v2/key"

// keyMap holds every binding. The brief fixes most of them; quit and help are
// the two conventional additions it does not list.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Back     key.Binding
	Tab      key.Binding
	Location key.Binding
	Reload   key.Binding
	Copy     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Cancel   key.Binding
	Help     key.Binding
	Quit     key.Binding
	// Paste is ctrl+v in the location editor. It is bound here rather than
	// left to the text input, because the input keeps a failed read in a field
	// of its own that nothing on the screen renders: a machine with no
	// clipboard tool would paste nothing and say nothing.
	Paste key.Binding
	// Interrupt is ctrl+c on its own. It is bound apart from Quit because the
	// location editor has to keep it while suspending every other binding: "q"
	// is text there, and Bubble Tea leaves ctrl+c to the program in raw mode.
	Interrupt key.Binding
}

// newKeyMap returns the default bindings.
func newKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "follow"),
		),
		Back: key.NewBinding(
			key.WithKeys("backspace", "left"),
			key.WithHelp("←/backspace", "back"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "panes"),
		),
		Location: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "location"),
		),
		Reload: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reload"),
		),
		Copy: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "scroll down"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Interrupt: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Paste: key.NewBinding(
			key.WithKeys("ctrl+v"),
			key.WithHelp("ctrl+v", "paste"),
		),
	}
}

// ShortHelp returns the bindings shown on the footer line.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Enter, k.Back, k.Location, k.Reload, k.Copy, k.Help, k.Quit}
}

// FullHelp returns every binding, grouped into columns for the help overlay.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Tab, k.PageUp, k.PageDown},
		{k.Location, k.Reload, k.Copy, k.Help, k.Quit},
	}
}
