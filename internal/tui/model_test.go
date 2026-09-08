package tui_test

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// newModel returns a model connected to a fixture service, sized, and having
// loaded the given resource through the real fetch path.
func newModel(t *testing.T, resource string) tui.Model {
	t.Helper()

	return newModelWithCache(t, resource, cache.New(0))
}

// newModelWithCache is newModel with a caller-supplied cache.
func newModelWithCache(t *testing.T, resource string, store *cache.Cache) tui.Model {
	t.Helper()

	m, _ := newModelWithServer(t, resource, store)

	return m
}

// newModelWithServer is newModelWithCache, handing back the fixture service so
// that a test can take it away and exercise a failure to reach it.
func newModelWithServer(t *testing.T, resource string, store *cache.Cache) (tui.Model, *httptest.Server) {
	t.Helper()

	server := redfishtest.NewServer()
	t.Cleanup(server.Close)

	client, err := redfish.Connect(t.Context(), redfishtest.Config(server))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	t.Cleanup(client.Close)

	m := tui.New(client, store, resource)
	m = resize(m, termWidth, termHeight)

	return drive(t, m, m.Init()), server
}

// drive runs a command and feeds every message it produces back through Update,
// so a test exercises the same path the program does.
func drive(t *testing.T, m tui.Model, cmd tea.Cmd) tui.Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	msg := cmd()

	batch, ok := msg.(tea.BatchMsg)
	if ok {
		for _, c := range batch {
			m = drive(t, m, c)
		}

		return m
	}

	// The spinner reschedules itself forever, which a synchronous test cannot
	// follow. Its state does not affect anything under test.
	_, isTick := msg.(spinner.TickMsg)
	if msg == nil || isTick {
		return m
	}

	updated, next := m.Update(msg)

	return drive(t, asModel(updated), next)
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

// press delivers a printable key and settles whatever it starts.
func press(t *testing.T, m tui.Model, keys string) tui.Model {
	t.Helper()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: rune(keys[0]), Text: keys})

	return drive(t, asModel(updated), cmd)
}

// pressCode delivers a named key, such as tab, enter or backspace.
func pressCode(t *testing.T, m tui.Model, code rune) tui.Model {
	t.Helper()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: code})

	return drive(t, asModel(updated), cmd)
}

// screen renders a model as plain text. Styling is stripped because an ANSI
// escape such as "\x1b[1;38;2;..." contains digits and letters of its own, and
// a substring assertion would otherwise match the colour rather than the text.
func screen(m tui.Model) string {
	return ansi.Strip(styled(m))
}

// styled renders a model with its escape sequences intact, for the few tests
// that care about how something is drawn rather than what it says.
func styled(m tui.Model) string {
	return m.View().Content
}

// footerLine returns the last rendered line, which carries the notice and the
// key hints.
func footerLine(m tui.Model) string {
	lines := strings.Split(screen(m), "\n")

	return lines[len(lines)-1]
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
	if len(lines) < 3 {
		t.Fatalf("screen has %d lines, want at least 3", len(lines))
	}

	if !strings.Contains(lines[0], "/redfish/v1/Systems/1") {
		t.Errorf("first line = %q, want the current path", lines[0])
	}

	if !strings.Contains(lines[0], "RedfishVersion 1.18.0") {
		t.Errorf("first line = %q, want the Redfish version", lines[0])
	}

	if !strings.Contains(lines[1], "curl -s") {
		t.Errorf("second line = %q, want the curl command", lines[1])
	}

	if !strings.Contains(lines[2], "root > Systems > 1") {
		t.Errorf("third line = %q, want the breadcrumb", lines[2])
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

func TestBodyPaneShowsHeadersAndBody(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	out := screen(m)

	for _, want := range []string{"HTTP/1.1 200 OK", "Odata-Version: 4.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("response pane is missing %q", want)
		}
	}

	if !strings.Contains(out, `"@odata.id": "/redfish/v1/Systems/1"`) {
		t.Error("response pane is missing the pretty-printed body")
	}

	// The curl command moved to the header, so the pane opens on the status.
	if strings.Contains(lineWith(t, m, "Response ─"), "curl") {
		t.Error("the curl command should not be in the response pane")
	}
}

// The curl command sits on the second header line, between the path and the
// breadcrumb, on one line so that it can be copied in a single gesture.
func TestHeaderShowsTheCurlCommandOnOneLine(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	curl := strings.Split(screen(m), "\n")[1]

	for _, want := range []string{"curl -s -k", "-u 'admin:********'", "-H 'Accept: application/json'"} {
		if !strings.Contains(curl, want) {
			t.Errorf("curl line = %q, want it to contain %q", curl, want)
		}
	}

	if !strings.Contains(curl, "/redfish/v1/Systems/1'") {
		t.Errorf("curl line = %q, want it to address the current resource", curl)
	}

	if strings.Contains(curl, "User-Agent") {
		t.Errorf("curl line = %q, want no User-Agent header", curl)
	}
}

// Copying is the point of the single-line curl command: what goes out has to be
// exactly what the header shows.
func TestCopyPutsTheCurlCommandOnTheClipboard(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("y produced no command, want the clipboard to be set")
	}

	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("y produced %T, want a batch of both copy mechanisms", cmd())
	}

	if len(batch) != 2 {
		t.Errorf("batch has %d commands, want OSC 52 and the local clipboard", len(batch))
	}

	// SetClipboard carries the text in an unexported message whose underlying
	// type is a string, so the value is readable even if the type is not.
	want := strings.TrimSpace(strings.Split(screen(m), "\n")[1])

	var found bool

	for _, c := range batch {
		if fmt.Sprint(c()) == want {
			found = true
		}
	}

	if !found {
		t.Errorf("nothing in the batch copied %q", want)
	}

	if !strings.Contains(want, "/redfish/v1/Systems/1'") {
		t.Errorf("curl line = %q, want the current resource", want)
	}

	if strings.Contains(want, "User-Agent") {
		t.Errorf("curl line = %q, want no User-Agent header", want)
	}
}

// A copy that got no further than OSC 52 cannot be confirmed, and must not be
// announced as done: a terminal that drops the sequence says nothing back.
func TestCopyReportsOnlyWhatItCanConfirm(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	confirmed, _ := m.Update(tui.CopyResultMsg(nil))
	if !strings.Contains(screen(asModel(confirmed)), "copied to the clipboard") {
		t.Errorf("want the copy confirmed, footer:\n%s", footerLine(asModel(confirmed)))
	}

	blind, _ := m.Update(tui.CopyResultMsg(errors.New("no clipboard utilities available")))
	if !strings.Contains(screen(asModel(blind)), "OSC 52") {
		t.Errorf("want an unconfirmed copy to say so, footer:\n%s", footerLine(asModel(blind)))
	}
}

func TestCursorMovesAndSkipsGroupHeaders(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// Down from ".." lands on the first link, not on the "Resource" heading.
	m = press(t, m, "j")

	if !strings.Contains(lineWith(t, m, "Bios"), "▸") {
		t.Errorf("cursor should be on Bios, screen:\n%s", screen(m))
	}

	m = press(t, m, "k")

	if !strings.Contains(lineWith(t, m, ".."), "▸") {
		t.Error("cursor should be back on ..")
	}
}

func TestCursorStopsAtTheEnds(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	m = press(t, m, "k")
	if !strings.Contains(lineWith(t, m, ".."), "▸") {
		t.Error("cursor should stay on the first row")
	}

	for range 40 {
		m = press(t, m, "j")
	}

	if !strings.Contains(lineWith(t, m, "SmartStorageUri"), "▸") {
		t.Errorf("cursor should stop on the last link, screen:\n%s", screen(m))
	}
}

func TestFooterShowsTheSelectedTarget(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = press(t, m, "j")

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
	before := styled(m)

	m = pressCode(t, m, '\t')

	if styled(m) == before {
		t.Error("tab should change which pane is highlighted")
	}

	m = pressCode(t, m, '\t')

	if styled(m) != before {
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
		out := styled(m)

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

func TestHelpPanel(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	before := screen(m)

	m = press(t, m, "?")
	help := screen(m)

	if !strings.Contains(help, "Help ─") {
		t.Fatalf("want the help panel, got:\n%s", help)
	}

	for _, want := range []string{"location", "reload", "copy", "back", "(oem)"} {
		if !strings.Contains(help, want) {
			t.Errorf("help panel is missing %q", want)
		}
	}

	m = press(t, m, "?")

	if screen(m) != before {
		t.Error("? should toggle the panel off again")
	}
}

// The panel takes the place of the two panes only. Everything framing them
// stays put, so the user does not lose their bearings while reading it.
func TestHelpPanelKeepsTheHeaderAndFooter(t *testing.T) {
	t.Parallel()

	m := press(t, newModel(t, "/redfish/v1/Systems/1"), "?")
	lines := strings.Split(screen(m), "\n")

	if !strings.Contains(lines[0], "/redfish/v1/Systems/1") {
		t.Errorf("first line = %q, want the location kept", lines[0])
	}

	if !strings.Contains(lines[1], "curl -s") {
		t.Errorf("second line = %q, want the curl command kept", lines[1])
	}

	if !strings.Contains(lines[2], "root > Systems > 1") {
		t.Errorf("third line = %q, want the breadcrumb kept", lines[2])
	}

	if !strings.Contains(footerLine(m), "q quit") {
		t.Errorf("footer = %q, want the key hints kept", footerLine(m))
	}

	// The panes themselves are gone: one panel spans the width.
	for _, gone := range []string{"Links (", "Response ─"} {
		if strings.Contains(screen(m), gone) {
			t.Errorf("want the %q pane replaced by the help panel", gone)
		}
	}
}

func TestAnyKeyClosesTheHelpPanel(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	before := screen(m)

	m = press(t, m, "?")
	m = press(t, m, "j")

	if screen(m) != before {
		t.Error("a key should close the overlay without acting behind it")
	}
}

func TestEmptyBodyIsCalledOut(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = m.WithResponse("/redfish/v1/Empty", &redfish.Response{Body: nil}, false)

	if !strings.Contains(screen(m), "empty response body") {
		t.Errorf("want an empty-body notice, got:\n%s", screen(m))
	}
}

func TestCachedResponsesAreMarked(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	resp := &redfish.Response{Body: []byte(`{}`), FetchedAt: time.Now().Add(-12 * time.Second)}
	m = m.WithResponse("/redfish/v1/Cached", resp, true)

	if !strings.Contains(screen(m), "cached 12s ago") {
		t.Errorf("want the cached age in the header, got:\n%s", pathLine(m))
	}
}

func TestBreadcrumbElidesFromTheLeft(t *testing.T) {
	t.Parallel()

	deep := "/redfish/v1/Systems/1/Storage/Controller/Volumes/VeryLongVolumeIdentifier/Metrics"
	m := newModel(t, redfish.RootPath)
	m = m.WithResponse(deep, &redfish.Response{Body: []byte(`{}`)}, false)
	m = resize(m, 61, 20)

	lines := strings.Split(screen(m), "\n")
	breadcrumb := lines[2]

	// The tail is the informative end, so the head is what gives way.
	if !strings.Contains(breadcrumb, "Metrics") {
		t.Errorf("breadcrumb = %q, want the tail kept", breadcrumb)
	}

	if !strings.Contains(breadcrumb, "…") {
		t.Errorf("breadcrumb = %q, want it elided", breadcrumb)
	}
}

func TestHelpPanelStaysInsideTheTerminal(t *testing.T) {
	t.Parallel()

	m := press(t, newModel(t, "/redfish/v1/Systems/1"), "?")

	sizes := []struct {
		width  int
		height int
	}{
		{width: 200, height: 60},
		{width: 80, height: 24},
		{width: 60, height: 15},
		{width: 120, height: 20},
	}

	for _, size := range sizes {
		m = resize(m, size.width, size.height)
		out := styled(m)

		if !strings.Contains(screen(m), "Help ─") {
			t.Fatalf("at %dx%d the panel is not showing:\n%s", size.width, size.height, screen(m))
		}

		lines := strings.Split(out, "\n")
		if len(lines) > size.height {
			t.Errorf("at %dx%d the help screen is %d lines tall", size.width, size.height, len(lines))
		}

		for i, line := range lines {
			if width := lineWidth(line); width > size.width {
				t.Errorf("at %dx%d line %d is %d columns wide", size.width, size.height, i, width)
			}
		}

		// The notes wrap rather than being cut off, so their tail survives even
		// on the narrowest supported terminal.
		if !strings.Contains(screen(m), "ActionInfo") {
			t.Errorf("at %dx%d the panel lost the end of its notes:\n%s", size.width, size.height, screen(m))
		}
	}
}

func TestLinkPaneKeepsTheParentEntryOnANonJSONBody(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfishtest.BrokenPath)

	if !strings.Contains(screen(m), "no links:") {
		t.Fatalf("want the not-JSON link notice, screen:\n%s", screen(m))
	}

	// The cursor is on "..", so it has to be drawn: it is the only way back up.
	if !strings.Contains(cursorLine(m), "..") {
		t.Errorf("want the .. entry under the cursor, screen:\n%s", screen(m))
	}
}
