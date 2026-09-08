package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/breml/redfish-explorer/internal/redfish"
	"github.com/breml/redfish-explorer/internal/tui"
)

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

func TestEditorAcceptsAPastedURLForTheConnectedHost(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	endpoint := m.Endpoint()

	m = openEditor(t, m)
	m = clearEditor(t, m, len(redfish.RootPath))
	m = typeText(t, m, endpoint+"/redfish/v1/Chassis")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Chassis") {
		t.Fatalf("header = %q, want the pasted URL reduced to its path", pathLine(m))
	}

	// The scheme and host are stripped, not carried into the location.
	if strings.Contains(pathLine(m), "https://") {
		t.Errorf("header = %q, want no scheme in the location", pathLine(m))
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
