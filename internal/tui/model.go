package tui

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/breml/redfish-explorer/internal/cache"
	"github.com/breml/redfish-explorer/internal/redfish"
)

// Focus says which pane takes the navigation keys.
type Focus int

const (
	// FocusLinks puts the cursor in the link pane.
	FocusLinks Focus = iota
	// FocusBody scrolls the response pane.
	FocusBody
)

// Mode says whether the header is being edited.
type Mode int

const (
	// ModeNormal is ordinary navigation.
	ModeNormal Mode = iota
	// ModeEdit is editing the location in the header.
	ModeEdit
)

// Layout constants.
const (
	// headerHeight covers the path line, the curl command, the breadcrumb and
	// the rule below them.
	headerHeight = 4
	// footerHeight covers the single footer line.
	footerHeight = 1
	// frameWidth and frameHeight are what a pane border costs.
	frameWidth  = 2
	frameHeight = 2
	// linkPaneShare is the fraction of the width the link pane takes.
	linkPaneShare = 3

	// minWidth and minHeight are the smallest terminal the layout survives.
	minWidth  = 60
	minHeight = 15
)

// Model is the explorer UI.
type Model struct {
	client  *redfish.Client
	store   *cache.Cache
	cfg     redfish.Config
	service redfish.ServiceInfo
	theme   Theme
	keys    keyMap

	width  int
	height int
	focus  Focus
	mode   Mode

	current   string
	resp      *redfish.Response
	groups    []redfish.Group
	rows      []row
	cursor    int
	fromCache bool

	// history is the trail of places a forward navigation has left behind.
	history []visit

	body    viewport.Model
	editor  textinput.Model
	spinner spinner.Model
	help    help.Model

	// pending is the resource currently being fetched, if any.
	pending string
	// pendingNav says what the pending fetch does to the history when it lands.
	pendingNav navKind
	// pendingLanding is where the pending fetch should leave the cursor.
	pendingLanding landing
	loading        bool

	// err is a failure to fetch: it replaces the response pane.
	err error
	// errResource is the resource that failed, so the pane can still show the
	// curl command for it and the user can retry by hand.
	errResource string
	// linkErr is a failure to read links out of a response that did arrive.
	// It explains an empty link pane and must never hide the response, since
	// showing exactly what the service sent is the point of the tool.
	linkErr error
	notice  string
	// editErr explains why the typed location was rejected, shown while the
	// editor stays open so it can be corrected rather than retyped.
	editErr string
	// showHelp covers the screen with every binding.
	showHelp bool
}

// New returns a model that will load resource from a connected service.
func New(client *redfish.Client, store *cache.Cache, resource string) Model {
	m := Model{
		client:  client,
		store:   store,
		cfg:     client.Config(),
		service: client.Service(),
		theme:   NewTheme(),
		keys:    newKeyMap(),
		current: resource,
		// Init fetches this straight away, so the guard in handleFetched has
		// to know about it from the start.
		pending:        resource,
		pendingNav:     navStay,
		pendingLanding: landFirst(),
		loading:        true,
		body:           viewport.New(),
		editor:         newEditor(),
		spinner:        spinner.New(),
		help:           help.New(),
	}

	m.rows = buildRows(nil, true)

	return m
}

// newEditor returns the location input used by the header.
func newEditor() textinput.Model {
	editor := textinput.New()
	editor.Prompt = ""

	// The real terminal cursor is placed by View, so that it lands in the
	// header where the user is typing.
	editor.SetVirtualCursor(false)

	return editor
}

// Init loads the service root.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchCmd(m.client, m.store, m.current, useCache))
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case fetchedMsg:
		return m.handleFetched(msg), nil

	case copiedMsg:
		return m.handleCopied(msg), nil

	case pastedMsg:
		return m.handlePasted(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd

	case tea.PasteMsg:
		return m.forwardToEditor(msg)

	default:
		// Nothing else on the screen takes input. The text input has a paste
		// of its own that would arrive here, but rfx binds ctrl+v itself
		// rather than let a failed read go unreported.
		return m, nil
	}
}

// View renders the whole screen.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.Cursor = m.cursorPosition()

	return view
}

// WithResponse returns the model showing a response. It is a value method
// because Bubble Tea threads the model by value through Update.
func (m Model) WithResponse(resource string, resp *redfish.Response, fromCache bool) Model {
	// The location editor can resolve to the resource already on screen, which
	// is not a step and must not become one to go back through.
	if m.pendingNav == navForward && resource != m.current {
		m = m.pushVisit(m.current, m.cursor)
	}

	// A step back leaves the trail only once it has actually landed on the
	// place it was heading for. navStay is neither, and touches nothing.
	if m.pendingNav == navBack {
		m = m.dropVisit()
	}

	m.current = resource
	m.resp = resp
	m.fromCache = fromCache
	m.notice = ""
	m.err = nil
	m.errResource = ""

	groups, _, err := redfish.ExtractLinks(resp)
	m.groups = groups
	m.linkErr = err

	m.rows = buildRows(groups, resource == redfish.RootPath)
	m.cursor = m.restoreCursor()

	m.body.SetContent(m.renderBody())
	m.body.GotoTop()

	return m
}

// forwardToEditor hands the location editor something that changes what is
// typed, which also retires a refusal: the entry is being corrected, so the
// hint has done its work. Outside edit mode nothing else on the screen takes
// input, so the message is simply dropped.
func (m Model) forwardToEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mode != ModeEdit {
		return m, nil
	}

	var cmd tea.Cmd

	m.editor, cmd = m.editor.Update(msg)
	m.editErr = ""

	return m, cmd
}

// handlePasted puts what the clipboard held into the editor, or says why
// nothing arrived. It travels the same route as a terminal paste, so that both
// insert at the cursor and lose their newlines the same way.
func (m Model) handlePasted(msg pastedMsg) (tea.Model, tea.Cmd) {
	if m.mode != ModeEdit {
		return m, nil
	}

	if msg.err != nil {
		m.editErr = msg.err.Error()

		return m, nil
	}

	return m.forwardToEditor(tea.PasteMsg{Content: msg.text})
}

// cursorPosition puts the terminal cursor in the location editor while it is
// open, and hides it otherwise.
func (m Model) cursorPosition() *tea.Cursor {
	if m.mode != ModeEdit {
		return nil
	}

	// The editor occupies the first header line, so its own X offset is the
	// screen X and the row is zero.
	return m.editor.Cursor()
}

// handleResize lays the panes out for a new terminal size.
func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height

	m.body.SetWidth(max(m.bodyWidth()-frameWidth, 1))
	m.body.SetHeight(max(m.paneContentHeight()-1, 1))
	m.body.SetContent(m.renderBody())
	m.editor.SetWidth(max(m.width-frameWidth, 1))
	m.help.SetWidth(max(m.width-frameWidth, 1))

	return m
}

// handleKey routes a key press by mode.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode == ModeEdit {
		return m.handleEditKey(msg)
	}

	return m.handleNavigationKey(msg)
}

// handleEditKey handles a key press while the location is being edited. Every
// other binding is suspended: typing "r" into a path must not reload.
func (m Model) handleEditKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	// Every binding but the interrupt: leaving esc as the only way out of the
	// program would be a trap, and ctrl+c is not a signal in raw mode.
	case key.Matches(msg, m.keys.Interrupt):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Cancel):
		return m.stopEditing(), nil

	case key.Matches(msg, m.keys.Enter):
		return m.submitEditedLocation()

	// The text input has a ctrl+v of its own; rfx reads the clipboard itself
	// so that a machine without the tools to do it says so.
	case key.Matches(msg, m.keys.Paste):
		return m, pasteCmd()

	default:
		return m.forwardToEditor(msg)
	}
}

// startEditing opens the location editor on the current resource.
func (m Model) startEditing() (tea.Model, tea.Cmd) {
	m.mode = ModeEdit
	m.editErr = ""
	m.editor.SetValue(m.current)
	m.editor.CursorEnd()

	return m, m.editor.Focus()
}

// stopEditing closes the editor and restores the header.
func (m Model) stopEditing() Model {
	m.mode = ModeNormal
	m.editErr = ""
	m.editor.Blur()

	return m
}

// submitEditedLocation loads what was typed, or explains why it cannot.
func (m Model) submitEditedLocation() (tea.Model, tea.Cmd) {
	resource, err := m.editedResource()
	if err != nil {
		// Keep the editor open so the entry can be corrected in place.
		m.editErr = err.Error()

		return m, nil
	}

	// A path that turns out not to exist is a normal outcome, not an error:
	// probing for undocumented endpoints is what this is for.
	return m.stopEditing().startFetch(resource, useCache, navForward, landFirst())
}

// editedResource turns what was typed into a resource on the connected
// endpoint. A full URL pasted from a browser is accepted; one naming a
// different host is refused, because silently querying another machine than
// the one in the header would be a real hazard.
func (m Model) editedResource() (string, error) {
	typed := strings.TrimSpace(m.editor.Value())
	if typed == "" {
		return "", errors.New("enter a resource path, for example " + redfish.RootPath + "/Systems")
	}

	target, err := m.client.Resolve(typed)
	if err != nil {
		return "", err
	}

	return strings.TrimPrefix(target, m.cfg.Endpoint), nil
}

// handleNavigationKey handles a key press in normal mode.
func (m Model) handleNavigationKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp

		return m, nil

	// Any other key closes the overlay rather than acting behind it.
	case m.showHelp:
		m.showHelp = false

		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.focus = m.otherFocus()

		return m, nil

	// Paging always scrolls the response pane, whichever pane has focus.
	case key.Matches(msg, m.keys.PageUp):
		m.body.PageUp()

		return m, nil

	case key.Matches(msg, m.keys.PageDown):
		m.body.PageDown()

		return m, nil

	case key.Matches(msg, m.keys.Up):
		return m.moveUp(), nil

	case key.Matches(msg, m.keys.Down):
		return m.moveDown(), nil

	case key.Matches(msg, m.keys.Enter):
		return m.follow()

	case key.Matches(msg, m.keys.Back):
		return m.goBack()

	case key.Matches(msg, m.keys.Reload):
		return m.startFetch(m.current, skipCache, navStay, landRow(m.cursor))

	case key.Matches(msg, m.keys.Copy):
		return m.copyCurl()

	case key.Matches(msg, m.keys.Location):
		return m.startEditing()

	default:
		return m, nil
	}
}

// follow acts on the highlighted row.
func (m Model) follow() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok {
		return m, nil
	}

	if r.parent {
		return m.goUp()
	}

	if r.link.Kind == redfish.KindAction {
		return m.followAction(r.link)
	}

	return m.startFetch(r.link.Target, useCache, navForward, landFirst())
}

// followAction opens an action's ActionInfo, which is the only part of an
// action a GET can reach. The target itself answers to POST and is listed so
// that vendor actions can be discovered at all.
func (m Model) followAction(link redfish.Link) (tea.Model, tea.Cmd) {
	if link.ActionInfo != "" {
		return m.startFetch(link.ActionInfo, useCache, navForward, landFirst())
	}

	m.notice = "POST target — not retrievable; write support planned"

	return m, nil
}

// copyCurl puts the curl command for the current location on the clipboard.
//
// Both mechanisms are used, because neither covers every case. OSC 52 travels
// down an SSH connection, which is how a BMC is usually reached, but a great
// many terminals refuse to act on it — VTE-based ones never have, and tmux and
// xterm need it turned on. The local clipboard tools always work, but only on
// the machine rfx itself runs on. What lands is the same either way, and the
// footer reports which of the two could actually be confirmed.
func (m Model) copyCurl() (tea.Model, tea.Cmd) {
	// What is copied is exactly what the header shows, password masking
	// included: --show-password governs both.
	command := redfish.Curl(m.cfg, m.curlResource())

	return m, tea.Batch(tea.SetClipboard(command), copyCmd(command))
}

// goUp walks one level towards the service root.
func (m Model) goUp() (tea.Model, tea.Cmd) {
	parent := redfish.Parent(m.current)
	if parent == m.current {
		return m, nil
	}

	return m.startFetch(parent, useCache, navForward, landChild(m.current))
}

// goBack returns to the place the last forward navigation left. With nothing
// to go back to it does nothing, as goUp does at the service root.
//
// The visit stays on the trail until the fetch lands, so that a step back which
// never arrives — an unreachable service, a late answer for somewhere the user
// has since left — does not consume the place it was heading for.
func (m Model) goBack() (tea.Model, tea.Cmd) {
	previous, ok := m.lastVisit()
	if !ok {
		return m, nil
	}

	return m.startFetch(previous.resource, useCache, navBack, landRow(previous.cursor))
}

// moveUp moves the cursor or scrolls the response pane, by which pane has focus.
func (m Model) moveUp() Model {
	if m.focus == FocusBody {
		m.body.ScrollUp(1)

		return m
	}

	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].selectable() {
			m.cursor = i
			// A notice describes the row it was raised on, not this one.
			m.notice = ""

			break
		}
	}

	return m
}

// moveDown moves the cursor or scrolls the response pane.
func (m Model) moveDown() Model {
	if m.focus == FocusBody {
		m.body.ScrollDown(1)

		return m
	}

	for i := m.cursor + 1; i < len(m.rows); i++ {
		if m.rows[i].selectable() {
			m.cursor = i
			m.notice = ""

			break
		}
	}

	return m
}

// otherFocus returns the pane that does not currently have focus.
func (m Model) otherFocus() Focus {
	if m.focus == FocusLinks {
		return FocusBody
	}

	return FocusLinks
}

// restoreCursor places the cursor for a response that has just landed. A
// remembered row is only honoured when the new row set still has one there to
// rest on: a resource can have changed between two visits, and the row a stale
// index points at would be the wrong one.
func (m Model) restoreCursor() int {
	remembered := m.pendingLanding.cursor
	if remembered >= 0 && remembered < len(m.rows) && m.rows[remembered].selectable() {
		return remembered
	}

	if i, ok := m.rowForTarget(m.pendingLanding.target); ok {
		return i
	}

	return m.firstLinkRow()
}

// rowForTarget returns the row that leads to target. Trailing slashes are
// ignored on both sides, since a service writes its own links and the path
// walked up from was trimmed by redfish.Parent.
func (m Model) rowForTarget(target string) (index int, ok bool) {
	if target == "" {
		return 0, false
	}

	target = strings.TrimRight(target, "/")

	for i, r := range m.rows {
		if r.header || r.parent {
			continue
		}

		if strings.TrimRight(r.link.Target, "/") == target {
			return i, true
		}
	}

	return 0, false
}

// firstLinkRow returns the row the cursor rests on when nothing better is
// known: the first link, so that a resource is one keypress from being
// explored. A resource with no links leaves the cursor on "..", which is then
// the only row there is.
func (m Model) firstLinkRow() int {
	for i, r := range m.rows {
		if r.selectable() && !r.parent {
			return i
		}
	}

	return 0
}

// selectedRow returns the row under the cursor.
func (m Model) selectedRow() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}

	return m.rows[m.cursor], true
}

// linkWidth is the width of the link pane, a third of the screen.
func (m Model) linkWidth() int {
	return m.width / linkPaneShare
}

// bodyWidth is the width of the response pane, the rest of the screen.
func (m Model) bodyWidth() int {
	return m.width - m.linkWidth()
}

// paneHeight is the height both panes share, borders included.
func (m Model) paneHeight() int {
	return m.height - headerHeight - footerHeight
}

// paneContentHeight is the height inside a pane border. Both panes spend the
// first of those lines on their title.
func (m Model) paneContentHeight() int {
	return m.paneHeight() - frameHeight
}

// render composes the screen.
func (m Model) render() string {
	if m.width < minWidth || m.height < minHeight {
		return "terminal too small: rfx needs at least " +
			itoa(minWidth) + "x" + itoa(minHeight)
	}

	middle := lipgloss.JoinHorizontal(lipgloss.Top, m.renderLinks(), m.renderResponse())
	if m.showHelp {
		middle = m.renderHelpPane()
	}

	return strings.Join([]string{
		m.renderHeader(m.width),
		m.theme.Dim.Render(strings.Repeat("─", m.width)),
		middle,
		m.renderFooter(m.width),
	}, "\n")
}

// renderHelpPane draws every binding in one panel across the width of both
// panes. It takes their place rather than the whole screen, so that the
// location, the curl command and the key hints stay where they were.
func (m Model) renderHelpPane() string {
	inner := m.width - frameWidth

	// The title takes the first of the pane's content lines.
	return m.pane(m.theme.PaneFocused, m.width,
		m.paneTitleLine("Help", inner),
		strings.Join(m.helpLines(inner, m.paneContentHeight()-1), "\n"))
}

// helpLines is the content of the panel, one terminal line per entry. The key
// columns arrive as several lines in one string and a note can be longer than a
// narrow terminal, so both are broken up here: pane counts lines, and would
// otherwise pad a block that already overflows.
//
// How to leave the panel is said in the footer, where the key hints live
// anyway, so that the panel spends every line it has on content. The blank
// between the columns and the notes goes the same way when height is short: the
// notes are content, the spacer is only comfort.
func (m Model) helpLines(width int, height int) []string {
	lines := strings.Split(m.help.FullHelpView(m.keys.FullHelp()), "\n")

	var notes []string

	for _, note := range helpNotes() {
		for wrapped := range strings.SplitSeq(ansi.Wrap(note, width, ""), "\n") {
			notes = append(notes, m.theme.Dim.Render(wrapped))
		}
	}

	if len(lines)+len(notes) < height {
		lines = append(lines, "")
	}

	return append(lines, notes...)
}

// helpNotes explains the link pane markers, below the key columns. They are
// kept short on purpose: at the smallest supported terminal the panel has only
// just enough room for the key columns and these two lines, and help that
// silently loses its tail is worse than help that is terse.
func helpNotes() []string {
	return []string{
		"(oem) = vendor extension, " + actionMarker + " = POST-only action target.",
		"Following an action opens its ActionInfo.",
	}
}

// renderLinks frames the link pane.
func (m Model) renderLinks() string {
	inner := m.linkWidth() - frameWidth
	height := m.paneContentHeight()

	return m.pane(m.paneStyle(FocusLinks), m.linkWidth(),
		m.paneTitleLine(m.linkPaneTitle(), inner), m.renderLinkPane(inner, height-1))
}

// renderResponse frames the response pane.
func (m Model) renderResponse() string {
	inner := m.bodyWidth() - frameWidth

	return m.pane(m.paneStyle(FocusBody), m.bodyWidth(),
		m.paneTitleLine("Response", inner), m.body.View())
}

// pane frames a title and a body. Both dimensions passed to lipgloss are the
// outer ones — Style.Width and Style.Height include the border — and the
// content is cut to exactly the room inside it, so the panes always agree and
// the screen never outgrows the terminal.
//
// The frame arrives as a style rather than as a Focus, because the help panel
// is framed too and has no place in a two-valued focus.
func (m Model) pane(frame lipgloss.Style, outerWidth int, title string, body string) string {
	inner := outerWidth - frameWidth

	lines := append([]string{title}, strings.Split(body, "\n")...)
	lines = fit(lines, m.paneContentHeight())

	for i, line := range lines {
		lines[i] = ansi.Truncate(line, inner, "…")
	}

	return frame.
		Width(outerWidth).
		Height(m.paneHeight()).
		Render(strings.Join(lines, "\n"))
}

// fit truncates or pads a block of lines to exactly n lines.
func fit(lines []string, n int) []string {
	if n < 0 {
		n = 0
	}

	if len(lines) > n {
		return lines[:n]
	}

	for len(lines) < n {
		lines = append(lines, "")
	}

	return lines
}

// paneTitleLine draws a pane's title above its contents.
func (m Model) paneTitleLine(title string, width int) string {
	rule := max(width-lipgloss.Width(title)-1, 0)

	return m.theme.PaneTitle.Render(title) + m.theme.Dim.Render(" "+strings.Repeat("─", rule))
}

// paneStyle returns the frame for a pane, highlighted when it has focus.
func (m Model) paneStyle(pane Focus) lipgloss.Style {
	if m.focus == pane {
		return m.theme.PaneFocused
	}

	return m.theme.PaneBlurred
}

// itoa renders a small non-negative number without pulling in strconv at the
// call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}
