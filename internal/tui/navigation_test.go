package tui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/breml/redfish-explorer/internal/cache"
	"github.com/breml/redfish-explorer/internal/redfish"
	"github.com/breml/redfish-explorer/internal/redfishtest"
	"github.com/breml/redfish-explorer/internal/tui"
)

// enter, backspace and tab as the terminal reports them.
const (
	keyEnter     = '\r'
	keyBackspace = '\x7f'
)

// parentEntry is the ".." row, as the link pane draws it.
const parentEntry = ".."

// pathLine returns the first header line, which names the current resource.
func pathLine(m tui.Model) string {
	return strings.SplitN(screen(m), "\n", 2)[0]
}

// currentPath returns just the resource from the first header line, which also
// carries the service metadata. Tests that distinguish a resource from its own
// prefix need the exact value, not a substring match.
func currentPath(t *testing.T, m tui.Model) string {
	t.Helper()

	fields := strings.Fields(pathLine(m))
	if len(fields) == 0 {
		t.Fatalf("no resource on the path line, screen:\n%s", screen(m))
	}

	return fields[0]
}

// cursorLine returns the link pane row that carries the cursor. The two panes
// are drawn side by side, so a whole screen line also holds whatever the
// response pane has at that height — text a caller searching for a link label
// must not match against.
func cursorLine(m tui.Model) string {
	for line := range strings.SplitSeq(screen(m), "\n") {
		if strings.Contains(line, "▸") {
			return linkPaneOf(line)
		}
	}

	return ""
}

// linkPaneOf cuts a screen line down to the link pane, discarding the response
// pane beside it. The two borders meeting is where one ends and the other
// begins.
func linkPaneOf(line string) string {
	pane, _, found := strings.Cut(line, "││")
	if !found {
		return line
	}

	return pane
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

// moveToParent walks the cursor up onto the ".." row, which a freshly loaded
// resource opens below.
func moveToParent(t *testing.T, m tui.Model) tui.Model {
	t.Helper()

	for range 60 {
		if strings.Contains(cursorLine(m), parentEntry) {
			return m
		}

		m = press(t, m, "k")
	}

	t.Fatalf("never reached %q, screen:\n%s", parentEntry, screen(m))

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

func TestDrillDownAndBackOut(t *testing.T) {
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

	if got := currentPath(t, m); got != "/redfish/v1/Systems" {
		t.Fatalf("path = %q, want backspace to retrace the last step", got)
	}

	m = pressCode(t, m, keyBackspace)

	if got := currentPath(t, m); got != redfish.RootPath {
		t.Fatalf("path = %q, want to be back at the service root", got)
	}
}

// Going back retraces where the user came from, which is not the same as
// walking up the path: a link can lead out of the current subtree.
func TestBackLeavesThePathTree(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = moveTo(t, m, "Chassis[0]")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Chassis/1") {
		t.Fatalf("path line = %q, want to have followed the link out of Systems", pathLine(m))
	}

	m = pressCode(t, m, keyBackspace)

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1") {
		t.Errorf("path line = %q, want back to return to where the link was followed", pathLine(m))
	}
}

func TestCursorLeftGoesBackToo(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = moveTo(t, m, "Chassis")
	m = pressCode(t, m, keyEnter)

	if !strings.Contains(pathLine(m), "/redfish/v1/Chassis") {
		t.Fatalf("path line = %q, want the Chassis collection", pathLine(m))
	}

	m = pressCode(t, m, tea.KeyLeft)

	if got := currentPath(t, m); got != redfish.RootPath {
		t.Errorf("path = %q, want cursor left to go back", got)
	}
}

func TestBackRestoresTheCursorPosition(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// A row well down the pane, below two group headings, so a reset to the
	// first selectable row would be unmistakable.
	m = moveTo(t, m, "ManagedBy[0]")
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Managers/BMC" {
		t.Fatalf("path = %q, want to have followed the link", got)
	}

	m = pressCode(t, m, keyBackspace)

	if got := currentPath(t, m); got != "/redfish/v1/Systems/1" {
		t.Fatalf("path = %q, want to be back", got)
	}

	if !strings.Contains(cursorLine(m), "ManagedBy[0]") {
		t.Errorf("cursor is on %q, want the row the link was followed from", cursorLine(m))
	}
}

// A forward step is a new place, so it starts on the first link of the resource
// it arrives at, and no row index is carried across: the service root has its
// own links where the Systems collection has its member.
func TestFollowingALinkStartsOnTheFirstLink(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = moveTo(t, m, "Systems")
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems" {
		t.Fatalf("path = %q, want the Systems collection", got)
	}

	// The collection has one member, so the cursor is already on it.
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems/1" {
		t.Errorf("path = %q, want enter to have followed the first link", got)
	}
}

func TestReloadKeepsTheCursorPosition(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = moveTo(t, m, "ManagedBy[0]")
	m = press(t, m, "r")

	if !strings.Contains(cursorLine(m), "ManagedBy[0]") {
		t.Errorf("cursor is on %q, want reload to keep the user's place", cursorLine(m))
	}
}

// Reload is not a step: it must not become something to go back through.
func TestReloadDoesNotEnterTheHistory(t *testing.T) {
	t.Parallel()

	m := newModel(t, redfish.RootPath)
	m = moveTo(t, m, "Systems")
	m = pressCode(t, m, keyEnter)
	m = press(t, m, "r")
	m = pressCode(t, m, keyBackspace)

	if got := currentPath(t, m); got != redfish.RootPath {
		t.Errorf("path = %q, want one back to undo the one step taken", got)
	}
}

// newModelOnAFlakyService returns a model on a fixture service that can be
// taken away and given back, which closing a test server cannot do. While it is
// down it drops the connection rather than answering: an unreachable BMC is a
// transport failure, where a status would be a response rfx would render.
func newModelOnAFlakyService(t *testing.T, resource string) (tui.Model, *atomic.Bool) {
	t.Helper()

	fixture := redfishtest.NewServer()
	t.Cleanup(fixture.Close)

	down := new(atomic.Bool)

	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !down.Load() {
			fixture.Config.Handler.ServeHTTP(w, r)

			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the test server does not support hijacking, so it cannot go away")

			return
		}

		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)

			return
		}

		conn.Close()
	}))
	t.Cleanup(service.Close)

	client, err := redfish.Connect(t.Context(), redfishtest.Config(service))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	t.Cleanup(client.Close)

	m := tui.New(client, cache.New(0), resource)
	m = resize(m, termWidth, termHeight)

	return drive(t, m, m.Init()), down
}

// A step back that never lands must not consume the place it was heading for:
// the trail has to be intact once the service answers again.
func TestAFailedBackKeepsTheTrail(t *testing.T) {
	t.Parallel()

	m, down := newModelOnAFlakyService(t, redfish.RootPath)

	m = moveTo(t, m, "Systems")
	m = pressCode(t, m, keyEnter)
	m = moveTo(t, m, "1")
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems/1" {
		t.Fatalf("path = %q, want to have drilled into the member", got)
	}

	down.Store(true)

	// Two failed attempts, which under a trail unwound before the fetch would
	// have emptied it.
	for range 2 {
		m = pressCode(t, m, keyBackspace)

		if got := currentPath(t, m); got != "/redfish/v1/Systems/1" {
			t.Fatalf("path = %q, want a failed back to leave the location alone", got)
		}
	}

	if !strings.Contains(screen(m), "fetching /redfish/v1/Systems") {
		t.Fatalf("want the failure reported, screen:\n%s", screen(m))
	}

	down.Store(false)

	m = pressCode(t, m, keyBackspace)

	if got := currentPath(t, m); got != "/redfish/v1/Systems" {
		t.Fatalf("path = %q, want back to retrace the last step once the service is up", got)
	}

	m = pressCode(t, m, keyBackspace)

	if got := currentPath(t, m); got != redfish.RootPath {
		t.Errorf("path = %q, want the whole trail to have survived", got)
	}
}

func TestParentEntryWalksUp(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// The cursor starts on the first link, so step up onto "..".
	m = moveToParent(t, m)
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems" {
		t.Errorf("path = %q, want .. to walk up", got)
	}

	// Walking up is a step like any other, so back retraces it.
	m = pressCode(t, m, keyBackspace)

	if !strings.Contains(pathLine(m), "/redfish/v1/Systems/1") {
		t.Errorf("path line = %q, want back to undo the walk up", pathLine(m))
	}
}

func TestBackspaceStopsWhereTheSessionStarted(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")

	// Nothing has been navigated to yet, so there is nowhere to go back to.
	for range 3 {
		m = pressCode(t, m, keyBackspace)
	}

	if got := currentPath(t, m); got != "/redfish/v1/Systems/1" {
		t.Errorf("path = %q, want to stay at the starting resource", got)
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
	m = moveToParent(t, m)
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

// A resource that cannot be reached at all keeps the user where they are, and
// the header shows the command for the resource that failed rather than for the
// one still on screen, so that the request can be retried by hand.
func TestAnUnreachableServiceKeepsTheLocationAndShowsItsCurl(t *testing.T) {
	t.Parallel()

	m, server := newModelWithServer(t, "/redfish/v1/Systems/1", cache.New(0))

	server.Close()

	m = moveTo(t, m, "Bios")
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems/1" {
		t.Errorf("path = %q, want a failed step to leave the location alone", got)
	}

	if !strings.Contains(screen(m), "fetching /redfish/v1/Systems/1/Bios") {
		t.Errorf("want the failure reported, screen:\n%s", screen(m))
	}

	curl := strings.Split(screen(m), "\n")[1]
	if !strings.Contains(curl, "/redfish/v1/Systems/1/Bios'") {
		t.Errorf("curl line = %q, want it to address the resource that failed", curl)
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

// Walking up keeps the user oriented: the parent opens on the row that leads
// back to the resource just left, not on its first link.
func TestWalkingUpSelectsTheResourceLeft(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems/1")
	m = moveToParent(t, m)
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems" {
		t.Fatalf("path = %q, want .. to walk up", got)
	}

	if !strings.Contains(cursorLine(m), "1") {
		t.Errorf("cursor is on %q, want the member walked up from", cursorLine(m))
	}

	// Following what is under the cursor returns to where the walk up began.
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != "/redfish/v1/Systems/1" {
		t.Errorf("path = %q, want the cursor to have been on the child", got)
	}
}

func TestWalkingUpToTheRootSelectsTheChild(t *testing.T) {
	t.Parallel()

	m := newModel(t, "/redfish/v1/Systems")
	m = moveToParent(t, m)
	m = pressCode(t, m, keyEnter)

	if got := currentPath(t, m); got != redfish.RootPath {
		t.Fatalf("path = %q, want the service root", got)
	}

	// "Chassis" sorts first at the root, so the first link is not the answer.
	if !strings.Contains(cursorLine(m), "Systems") {
		t.Errorf("cursor is on %q, want the Systems link walked up from", cursorLine(m))
	}
}
