package tui_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/breml/redfish-explorer/internal/redfish"
	"github.com/breml/redfish-explorer/internal/tui"
)

// strayMsg is a message the model knows nothing about, standing in for
// whatever a bubble may send back while the editor is open.
type strayMsg struct{}

// keyEsc is escape as the terminal reports it.
const keyEsc = '\x1b'

// openEditor presses L to open the location editor.
func openEditor(t *testing.T, m tui.Model) tui.Model {
	t.Helper()

	return press(t, m, "L")
}

// typeText delivers each character of s to the model.
func typeText(t *testing.T, m tui.Model, s string) tui.Model {
	t.Helper()

	for _, r := range s {
		m = press(t, m, string(r))
	}

	return m
}

// paste delivers s the way a terminal delivers a paste: one message carrying
// the whole text, not a key press per character.
func paste(t *testing.T, m tui.Model, s string) tui.Model {
	t.Helper()

	updated, cmd := m.Update(tea.PasteMsg{Content: s})

	return drive(t, asModel(updated), cmd)
}

// clearEditor removes whatever the editor holds.
func clearEditor(t *testing.T, m tui.Model, length int) tui.Model {
	t.Helper()

	for range length {
		m = pressCode(t, m, keyBackspace)
	}

	return m
}

func TestEditorOpensPrefilledWithTheCurrentLocation(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, "/redfish/v1/Systems/1"))

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1") {
		t.Errorf("header = %q, want it prefilled with the current location", pathLine(m))
	}

	if !strings.Contains(screen(m), "enter load · esc cancel") {
		t.Errorf("want the editing hint, screen:\n%s", screen(m))
	}
}

func TestEditorPlacesTheTerminalCursor(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	if m.View().Cursor != nil {
		t.Error("the cursor must be hidden while navigating")
	}

	m = openEditor(t, m)

	cursor := m.View().Cursor
	if cursor == nil {
		t.Fatal("want a terminal cursor while editing")
	}

	// It opens at the end of the prefilled value, on the first header line.
	if cursor.Y != 0 {
		t.Errorf("cursor Y = %d, want the first header line", cursor.Y)
	}

	if cursor.X != len("/redfish/v1/Systems/1") {
		t.Errorf("cursor X = %d, want the end of the value", cursor.X)
	}
}

func TestEditorSuspendsOtherBindings(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, "/redfish/v1/Systems/1"))

	// r would reload and j would move the cursor; here they are just text.
	m = typeText(t, m, "rj")

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1rj") {
		t.Errorf("header = %q, want the keys typed into the editor", pathLine(m))
	}
}

func TestEditorCancelRestoresTheHeader(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	before := screen(m)

	m = openEditor(t, m)
	m = typeText(t, m, "/nonsense")
	m = pressCode(t, m, keyEsc)

	if screen(m) != before {
		t.Errorf("esc should restore the header untouched, got:\n%s", screen(m))
	}

	if m.View().Cursor != nil {
		t.Error("the cursor must be hidden again after esc")
	}
}

func TestEditorLoadsATypedPath(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = openEditor(t, m)
	m = clearEditor(t, m, len("/redfish/v1/Systems/1"))
	m = typeText(t, m, "/redfish/v1/Chassis/1")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Chassis/1") {
		t.Fatalf("header = %q, want the typed resource loaded", pathLine(m))
	}

	if !strings.Contains(screen(m), "Contoso Chassis") {
		t.Error("want the typed resource's body")
	}
}

func TestEditorAcceptsATypedURLForTheConnectedHost(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	endpoint := m.Endpoint()

	m = openEditor(t, m)
	m = clearEditor(t, m, len(redfish.RootPath))
	m = typeText(t, m, endpoint+"/redfish/v1/Chassis")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Chassis") {
		t.Fatalf("header = %q, want the URL reduced to its path", pathLine(m))
	}

	// The scheme and host are stripped, not carried into the location.
	if strings.Contains(pathLine(m), "https://") {
		t.Errorf("header = %q, want no scheme in the location", pathLine(m))
	}
}

// A terminal paste arrives as one message rather than as key presses, so it
// reaches the editor by a different route than typing does.
func TestEditorAcceptsATerminalPaste(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	endpoint := m.Endpoint()

	m = openEditor(t, m)
	m = clearEditor(t, m, len(redfish.RootPath))
	m = paste(t, m, endpoint+"/redfish/v1/Chassis")

	if !strings.Contains(pathLine(m), endpoint+"/redfish/v1/Chassis") {
		t.Fatalf("editor = %q, want the pasted URL in it", pathLine(m))
	}

	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Chassis") {
		t.Errorf("header = %q, want the pasted URL loaded", pathLine(m))
	}
}

// Pasting into a location already being edited inserts at the cursor rather
// than replacing what is there, so a path can be assembled from both.
func TestPasteInsertsIntoWhatIsAlreadyTyped(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))
	m = paste(t, m, "/Systems")

	if !strings.Contains(pathLine(m), redfish.RootPath+"/Systems") {
		t.Errorf("editor = %q, want the paste appended to the prefilled path", pathLine(m))
	}
}

func TestEditorRefusesAnotherHost(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))
	m = clearEditor(t, m, len(redfish.RootPath))
	m = typeText(t, m, "https://192.0.2.1/redfish/v1")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(screen(m), "not on the connected endpoint") {
		t.Errorf("want an inline refusal, screen:\n%s", screen(m))
	}

	// The editor stays open so the entry can be corrected in place.
	if m.View().Cursor == nil {
		t.Error("the editor should stay open after a refusal")
	}
}

func TestEditorRejectsUnusableEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		typed string
	}{
		{name: "empty", typed: ""},
		{name: "whitespace only", typed: "   "},
		{name: "relative path", typed: "Systems/1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			m := openEditor(t, newModel(t, redfish.RootPath))
			m = clearEditor(t, m, len(redfish.RootPath))
			m = typeText(t, m, test.typed)
			m = pressCode(t, m, keyEnter)

			if m.View().Cursor == nil {
				t.Fatalf("the editor should stay open for %q", test.typed)
			}

			if !strings.Contains(pathLine(m), test.typed) {
				t.Errorf("header = %q, want what was typed kept for correction", pathLine(m))
			}
		})
	}
}

func TestEditorTypoLoadsThe404(t *testing.T) {
	t.Parallel()

	// Probing for an undocumented endpoint is a first-class use, so a 404 is a
	// result to read, not an error to complain about.
	m := newModel(t, redfish.RootPath)
	m = openEditor(t, m)
	m = clearEditor(t, m, len(redfish.RootPath))
	m = typeText(t, m, "/redfish/v1/Undocumented")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Undocumented") {
		t.Fatalf("header = %q, want the probed path loaded", pathLine(m))
	}

	if !strings.Contains(screen(m), "404 Not Found") {
		t.Error("want the 404 rendered like any other response")
	}

	if m.View().Cursor != nil {
		t.Error("the editor should have closed")
	}
}

func TestEditorTrimsSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))
	m = clearEditor(t, m, len(redfish.RootPath))
	m = typeText(t, m, "  /redfish/v1/Chassis  ")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Chassis") {
		t.Errorf("header = %q, want the trimmed path loaded", pathLine(m))
	}
}

func TestEditorHintClearsOnFurtherTyping(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))
	m = clearEditor(t, m, len(redfish.RootPath))
	m = typeText(t, m, "nonsense")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(screen(m), "absolute") {
		t.Fatalf("want a refusal first, screen:\n%s", screen(m))
	}

	m = typeText(t, m, "x")

	if strings.Contains(screen(m), "absolute") {
		t.Error("the refusal should clear once the entry is being corrected")
	}
}

// ctrl+v reads the clipboard rather than leaving it to the text input, whose
// own paste keeps its failure in a field nothing renders.
func TestCtrlVReadsTheClipboard(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+v produced no command, want the clipboard to be read")
	}

	if _, ok := cmd().(tea.KeyPressMsg); ok {
		t.Error("ctrl+v should not have been forwarded to the editor as a key")
	}
}

func TestAClipboardReadInsertsWhatItFound(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))

	updated, _ := m.Update(tui.PasteResultMsg("/Systems", nil))
	m = asModel(updated)

	if !strings.Contains(pathLine(m), redfish.RootPath+"/Systems") {
		t.Errorf("editor = %q, want the clipboard text inserted at the cursor", pathLine(m))
	}
}

// A machine with no clipboard tool has to say so: pasting nothing and
// reporting nothing is the one outcome the user cannot make sense of.
func TestAFailedClipboardReadSaysSo(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))

	updated, _ := m.Update(tui.PasteResultMsg("", errors.New("no clipboard utilities available")))
	m = asModel(updated)

	if !strings.Contains(screen(m), "no clipboard utilities available") {
		t.Errorf("want the reason the paste failed, screen:\n%s", screen(m))
	}

	if m.View().Cursor == nil {
		t.Error("the editor should stay open after a failed paste")
	}
}

// A refusal is retired by correcting the entry, and by nothing else: a message
// that changes no text must not take the only explanation off the screen.
func TestARefusalSurvivesAMessageThatChangesNothing(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))
	m = clearEditor(t, m, len(redfish.RootPath))
	m = typeText(t, m, "nonsense")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(screen(m), "absolute") {
		t.Fatalf("want a refusal first, screen:\n%s", screen(m))
	}

	updated, _ := m.Update(strayMsg{})
	m = asModel(updated)

	if !strings.Contains(screen(m), "absolute") {
		t.Errorf("the refusal must outlive an unrelated message, screen:\n%s", screen(m))
	}
}

// Loading the location already on screen is not a step, so it must not become
// one to go back through.
func TestSubmittingTheUnchangedLocationIsNotAStep(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = moveTo(t, m, "Systems")
	m = pressCode(t, m, keyEnter)

	m = openEditor(t, m)
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems" {
		t.Fatalf("path = %q, want to be where the editor was opened", got)
	}

	m = pressCode(t, m, keyBackspace)

	if got := currentPath(t, m); got != redfish.RootPath {
		t.Errorf("path = %q, want one back to leave for the service root", got)
	}
}

func TestEditorKeepsTheInterrupt(t *testing.T) {
	t.Parallel()

	m := openEditor(t, newModel(t, redfish.RootPath))

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c must not be swallowed by the editor")
	}

	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c returned %T, want a quit", cmd())
	}
}
