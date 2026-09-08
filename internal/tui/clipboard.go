package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// copiedMsg reports what became of a copy to the local clipboard.
type copiedMsg struct {
	err error
}

// copyCmd writes text to the clipboard of the machine rfx runs on. It is a
// command rather than a plain call because on Unix it shells out to xclip,
// xsel or wl-copy, which has no business happening on the update path.
func copyCmd(text string) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(text)
		if err != nil {
			return copiedMsg{err: fmt.Errorf("writing to the local clipboard: %w", err)}
		}

		return copiedMsg{err: nil}
	}
}

// handleCopied reports what became of a copy. Only the local clipboard can be
// confirmed: OSC 52 is written blind down the terminal, and one that drops the
// sequence says nothing back, so a copy that got no further than OSC 52 must
// not be announced as done.
func (m Model) handleCopied(msg copiedMsg) Model {
	if msg.err != nil {
		m.notice = "curl command sent as OSC 52 — if nothing pastes, the terminal is refusing it"

		return m
	}

	m.notice = "curl command copied to the clipboard"

	return m
}
