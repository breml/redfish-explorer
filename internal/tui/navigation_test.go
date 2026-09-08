package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/breml/redfish-explorer/internal/cache"
	"github.com/breml/redfish-explorer/internal/redfish"
	"github.com/breml/redfish-explorer/internal/tui"
)

// enter, backspace and tab as the terminal reports them.
const (
	keyEnter     = '\r'
	keyBackspace = '\x7f'
)

// pathLine returns the first header line, which names the current resource.
func pathLine(m tui.Model) string {
	return strings.SplitN(screen(m), "\n", 2)[0]
}

// cursorLine returns the rendered row that carries the cursor.
func cursorLine(m tui.Model) string {
	for line := range strings.SplitSeq(screen(m), "\n") {
		if strings.Contains(line, "▸") {
			return line
		}
	}

	return ""
}

// moveTo walks the cursor down until the given label is highlighted.
func moveTo(t *testing.T, m tui.Model, label string) tui.Model {
	t.Helper()

	for range 60 {
		if strings.Contains(cursorLine(m), label) {
			return m
		}

		m = press(t, m, "j")
	}

	t.Fatalf("never reached %q, screen:\n%s", label, screen(m))

	return m
}

func TestInitLoadsTheStartingResource(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1") {
		t.Errorf("path line = %q, want the starting resource", pathLine(m))
	}

	if !strings.Contains(screen(m), "Bios") {
		t.Error("want the resource's links after Init")
	}
}

func TestFollowALink(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = moveTo(t, m, "Systems")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems") {
		t.Fatalf("path line = %q, want the Systems collection", pathLine(m))
	}

	// A collection lists its members by ID.
	if !strings.Contains(screen(m), "Members") {
		t.Errorf("want the Members group, screen:\n%s", screen(m))
	}
}

func TestDrillDownAndBackUp(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)

	m = moveTo(t, m, "Systems")
	m = pressCode(t, m, keyEnter)
	m = moveTo(t, m, "1")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1") {
		t.Fatalf("path line = %q, want to have drilled into the member", pathLine(m))
	}

	if !strings.Contains(screen(m), "root > Systems > 1") {
		t.Error("want the breadcrumb to track the location")
	}

	// The curl command tracks the location too.
	if !strings.Contains(screen(m), "/redfish/v1/Systems/1'") {
		t.Error("want the curl command to address the current resource")
	}

	m = pressCode(t, m, keyBackspace)

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems") {
		t.Fatalf("path line = %q, want backspace to go one level up", pathLine(m))
	}

	m = pressCode(t, m, keyBackspace)

	if !strings.Contains(pathLine(m), "/redfish/v1") {
		t.Fatalf("path line = %q, want to be back at the service root", pathLine(m))
	}
}

func TestParentEntryWalksUp(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// The cursor starts on "..".
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems") {
		t.Errorf("path line = %q, want .. to walk up", pathLine(m))
	}
}

func TestBackspaceStopsAtTheServiceRoot(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)

	for range 3 {
		m = pressCode(t, m, keyBackspace)
	}

	if !strings.Contains(pathLine(m), redfish.RootPath) {
		t.Errorf("path line = %q, want to stay at the service root", pathLine(m))
	}
}

func TestFollowingAnActionShowsANotice(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = moveTo(t, m, "#Contoso.SecureErase")

	before := pathLine(m)
	m = pressCode(t, m, keyEnter)

	if pathLine(m) != before {
		t.Error("an action target must not be fetched")
	}

	if !strings.Contains(screen(m), "not retrievable") {
		t.Errorf("want a notice about the POST target, screen:\n%s", screen(m))
	}
}

func TestFollowingAnActionOpensItsActionInfo(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = moveTo(t, m, "#ComputerSystem.Reset")
	m = pressCode(t, m, keyEnter)

	// The fixture has no ResetActionInfo resource, so this 404s — which is a
	// perfectly good outcome: the body says so.
	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1/ResetActionInfo") {
		t.Errorf("path line = %q, want the ActionInfo resource", pathLine(m))
	}
}

func TestErrorStatusRendersItsBody(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/DoesNotExist")

	if !strings.Contains(screen(m), "404 Not Found") {
		t.Errorf("want the 404 status, screen:\n%s", screen(m))
	}

	if !strings.Contains(screen(m), "ResourceMissingAtURI") {
		t.Error("want the Redfish error body")
	}

	// A 404 is a response, not a failure: the location still holds.
	if !strings.Contains(pathLine(m), "/redfish/v1/DoesNotExist") {
		t.Error("want the location to be the resource that 404d")
	}
}

func TestNonJSONErrorPageIsShownRaw(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Broken")

	if !strings.Contains(screen(m), "500 Internal Server Error") {
		t.Error("want the 500 status")
	}

	if !strings.Contains(screen(m), "not valid JSON") {
		t.Error("want the not-JSON notice")
	}

	if !strings.Contains(screen(m), "Internal Server Error</h1>") {
		t.Errorf("want the raw HTML body, screen:\n%s", screen(m))
	}
}

func TestReloadBypassesTheCache(t *testing.T) {
	t.Parallel()

	store := cache.New(time.Hour)
	m := newModelWithCache(t, "/redfish/v1/Systems/1", store)

	if store.Len() != 1 {
		t.Fatalf("cache holds %d entries after the first load, want 1", store.Len())
	}

	// Navigating away and back is answered from the cache.
	m = pressCode(t, m, keyEnter)
	m = moveTo(t, m, "1")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(screen(m), "cached") {
		t.Errorf("want the header to mark a cached response, screen:\n%s", screen(m))
	}

	m = press(t, m, "r")

	if strings.Contains(screen(m), "cached") {
		t.Error("reload must replace the cached entry with a fresh response")
	}
}

func TestStaleResponsesAreDiscarded(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// A late answer for a resource the user has already left must not be drawn
	// under the current location.
	updated, _ := m.Update(tui.StaleFetchedMsg("/redfish/v1/Chassis"))
	m = asModel(updated)

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1") {
		t.Errorf("path line = %q, want the stale answer ignored", pathLine(m))
	}
}

func TestPagingScrollsTheResponsePaneFromEitherPane(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	top := screen(m)

	m = pressCode(t, m, tea.KeyPgDown)

	if screen(m) == top {
		t.Error("page down should scroll the response pane while the link pane has focus")
	}

	m = pressCode(t, m, tea.KeyPgUp)

	if screen(m) != top {
		t.Error("page up should scroll back")
	}
}

func TestArrowsScrollTheResponsePaneWhenItHasFocus(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = pressCode(t, m, '\t')

	before := screen(m)
	m = press(t, m, "j")

	if screen(m) == before {
		t.Error("with the response pane focused, j should scroll it")
	}
}

func TestTheActionNoticeClearsWhenTheCursorMoves(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = moveTo(t, m, "#Contoso.SecureErase")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(screen(m), "not retrievable") {
		t.Fatalf("want the notice first, screen:\n%s", screen(m))
	}

	m = press(t, m, "k")

	if strings.Contains(screen(m), "not retrievable") {
		t.Errorf("the notice must not outlive the row it describes, screen:\n%s", screen(m))
	}
}
