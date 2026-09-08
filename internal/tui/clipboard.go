package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// copiedMsg reports what became of a copy to the local clipboard.
type copiedMsg struct {
	err error
}

// pastedMsg carries what the local clipboard held, or why it could not be read.
type pastedMsg struct {
	text string
	err  error
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

// pasteCmd reads the clipboard of the machine rfx runs on, shelling out the
// same way copyCmd does.
func pasteCmd() tea.Cmd {
	return func() tea.Msg {
		text, err := clipboard.ReadAll()
		if err != nil {
			return pastedMsg{err: fmt.Errorf("reading the local clipboard: %w", err)}
		}

		return pastedMsg{text: text}
	}
}

// handleCopied reports what became of a copy, in the terms the user can act
// on. Only the local clipboard answers back: OSC 52 is written blind down the
// terminal, and one that drops the sequence says nothing, so a copy that got no
// further than OSC 52 must not be announced as done. A local write says nothing
// about the user's own clipboard either when rfx is running over SSH, where the
// selection it lands in belongs to the far end.
func (m Model) handleCopied(msg copiedMsg) Model {
	if msg.err != nil {
		m.notice = "curl command sent as OSC 52 only — " + msg.err.Error()

		return m
	}

	if remoteSession() {
		m.notice = "curl command copied on this host — over SSH only OSC 52 can reach yours"

		return m
	}

	m.notice = "curl command copied to the clipboard"

	return m
}

// remoteSession reports whether rfx is running on the far end of an SSH
// connection, which OpenSSH marks with these two variables. It decides only how
// a copy is described, so guessing wrong costs nothing but the wording.
func remoteSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""
}
