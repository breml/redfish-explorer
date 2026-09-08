package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/breml/redfish-explorer/internal/cache"
	"github.com/breml/redfish-explorer/internal/redfish"
	"github.com/breml/redfish-explorer/internal/redfishtest"
	"github.com/breml/redfish-explorer/internal/tui"
)

const (
	// termWidth and termHeight are a comfortable terminal for the layout tests.
	termWidth  = 120
	termHeight = 40
)

// newModel returns a model connected to a fixture service, sized and showing
// the given resource.
func newModel(t *testing.T, resource string) tui.Model {
	t.Helper()

	server := redfishtest.NewServer()
	t.Cleanup(server.Close)

	client, err := redfish.Connect(t.Context(), redfishtest.Config(server))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	t.Cleanup(client.Close)

	resp, err := client.Fetch(t.Context(), resource)
	if err != nil {
		t.Fatalf("Fetch(%s): %v", resource, err)
	}

	m := tui.New(client, cache.New(0))
	m = resize(m, termWidth, termHeight)
	return m.WithResponse(resource, resp, false)
}

// resize delivers a window size to a model.
func resize(m tui.Model, width int, height int) tui.Model {
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})

	return asModel(updated)
}

// asModel narrows what Update returns back to the concrete model.
func asModel(model tea.Model) tui.Model {
	m, ok := model.(tui.Model)
	if !ok {
		panic("tui: Update returned a different model type")
	}

	return m
}

// press delivers one key press to a model.
func press(m tui.Model, keys string) tui.Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: rune(keys[0]), Text: keys})

	return asModel(updated)
}

// pressCode delivers a named key, such as tab or pgdown.
func pressCode(m tui.Model, code rune) tui.Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: code})

	return asModel(updated)
}

// screen renders a model.
func screen(m tui.Model) string {
	return m.View().Content
}

// lineWith returns the first rendered line containing want.
func lineWith(t *testing.T, m tui.Model, want string) string {
	t.Helper()

	for line := range strings.SplitSeq(screen(m), "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}

	t.Fatalf("no line containing %q in:\n%s", want, screen(m))

	return ""
}

func TestViewUsesTheAltScreen(t *testing.T) {
	t.Parallel()

	if !newModel(t, redfish.RootPath).View().AltScreen {
		t.Error("View: want AltScreen set")
	}
}

func TestHeaderShowsPathAndBreadcrumb(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	lines := strings.Split(screen(m), "\n")
	if len(lines) < 2 {
		t.Fatalf("screen has %d lines, want at least 2", len(lines))
	}

	if !strings.Contains(lines[0], "/redfish/v1/Systems/1") {
		t.Errorf("first line = %q, want the current path", lines[0])
	}

	if !strings.Contains(lines[0], "RedfishVersion 1.18.0") {
		t.Errorf("first line = %q, want the Redfish version", lines[0])
	}

	if !strings.Contains(lines[1], "root > Systems > 1") {
		t.Errorf("second line = %q, want the breadcrumb", lines[1])
	}
}

func TestLinkPaneListsGroupsAndLinks(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	out := screen(m)

	for _, want := range []string{"Resource", "Links", "Actions", "Annotations", "Oem · Contoso"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen is missing group %q", want)
		}
	}

	for _, want := range []string{"Bios", "SecureBoot", "Chassis[0]", "#ComputerSystem.Reset"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen is missing link %q", want)
		}
	}
}

func TestLinkPaneStartsWithParentEntry(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// The first pane line after the border and the title is "..".
	if !strings.Contains(screen(m), "..") {
		t.Error("screen is missing the .. entry")
	}

	if !strings.Contains(lineWith(t, m, ".."), "▸") {
		t.Error(".. should carry the cursor on a fresh response")
	}
}

func TestLinkPaneMarksOEMAndActions(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	if !strings.Contains(lineWith(t, m, "Thermal"), "(oem)") {
		t.Error("an OEM link must carry the (oem) suffix")
	}

	if !strings.Contains(lineWith(t, m, "#ComputerSystem.Reset"), "⚡") {
		t.Error("an action must carry the ⚡ marker")
	}
}

func TestLinkPaneCountsLinks(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// Two Resource, two Links, one Action, one Annotation, three Oem.
	if !strings.Contains(screen(m), "Links (9)") {
		t.Errorf("want the pane title to count nine links, got:\n%s", lineWith(t, m, "Links ("))
	}
}

func TestBodyPaneShowsCurlHeadersAndBody(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	out := screen(m)

	for _, want := range []string{"curl -s -k", "-u 'admin:********'", "HTTP/1.1 200 OK", "Odata-Version: 4.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("response pane is missing %q", want)
		}
	}

	if !strings.Contains(out, `"@odata.id": "/redfish/v1/Systems/1"`) {
		t.Error("response pane is missing the pretty-printed body")
	}
}

func TestCursorMovesAndSkipsGroupHeaders(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// Down from ".." lands on the first link, not on the "Resource" heading.
	m = press(m, "j")

	if !strings.Contains(lineWith(t, m, "Bios"), "▸") {
		t.Errorf("cursor should be on Bios, screen:\n%s", screen(m))
	}

	m = press(m, "k")

	if !strings.Contains(lineWith(t, m, ".."), "▸") {
		t.Error("cursor should be back on ..")
	}
}

func TestCursorStopsAtTheEnds(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	m = press(m, "k")
	if !strings.Contains(lineWith(t, m, ".."), "▸") {
		t.Error("cursor should stay on the first row")
	}

	for range 40 {
		m = press(m, "j")
	}

	if !strings.Contains(lineWith(t, m, "SmartStorageUri"), "▸") {
		t.Errorf("cursor should stop on the last link, screen:\n%s", screen(m))
	}
}

func TestFooterShowsTheSelectedTarget(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = press(m, "j")

	lines := strings.Split(screen(m), "\n")
	footer := lines[len(lines)-1]

	if !strings.Contains(footer, "/redfish/v1/Systems/1/Bios") {
		t.Errorf("footer = %q, want the selected target", footer)
	}

	if !strings.Contains(footer, "tab") {
		t.Errorf("footer = %q, want the key hints", footer)
	}
}

func TestParentEntryIsDimmedAtTheRoot(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)

	lines := strings.Split(screen(m), "\n")
	if !strings.Contains(lines[len(lines)-1], "already at the service root") {
		t.Errorf("footer = %q, want it to say the root has no parent", lines[len(lines)-1])
	}
}

func TestTabMovesFocus(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	before := screen(m)

	m = pressCode(m, '\t')

	if screen(m) == before {
		t.Error("tab should change which pane is highlighted")
	}

	m = pressCode(m, '\t')

	if screen(m) != before {
		t.Error("tab twice should return to the first pane")
	}
}

func TestLayoutSurvivesResizing(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	sizes := []struct {
		width  int
		height int
	}{
		{width: 200, height: 60},
		{width: 80, height: 24},
		{width: 61, height: 16},
		{width: 120, height: 40},
	}

	for _, size := range sizes {
		m = resize(m, size.width, size.height)
		out := screen(m)

		for i, line := range strings.Split(out, "\n") {
			if width := lineWidth(line); width > size.width {
				t.Errorf("at %dx%d line %d is %d columns wide", size.width, size.height, i, width)
			}
		}

		if height := strings.Count(out, "\n") + 1; height > size.height {
			t.Errorf("at %dx%d the screen is %d lines tall", size.width, size.height, height)
		}
	}
}

func TestTooSmallTerminal(t *testing.T) {
	t.Parallel()

	m := resize(newModel(t, redfish.RootPath), 40, 10)

	if !strings.Contains(screen(m), "terminal too small") {
		t.Errorf("want a too-small notice, got:\n%s", screen(m))
	}
}

func TestResourceWithoutLinks(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = m.WithResponse("/redfish/v1/Empty", &redfish.Response{Body: []byte(`{}`)}, false)

	if !strings.Contains(screen(m), "..") {
		t.Error("the .. entry must remain when a resource has no links")
	}

	if !strings.Contains(screen(m), "Links (0)") {
		t.Error("want the link count to be zero")
	}
}

func TestNonJSONBody(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = m.WithResponse("/redfish/v1/Broken", &redfish.Response{Body: []byte("<html>nope</html>")}, false)

	if !strings.Contains(screen(m), "not valid JSON") {
		t.Errorf("want a not-JSON notice, got:\n%s", screen(m))
	}

	if !strings.Contains(screen(m), "<html>nope</html>") {
		t.Error("the raw body must still be shown")
	}
}
