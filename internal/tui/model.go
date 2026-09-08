package tui

import (
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

// Layout constants.
const (
	// headerHeight covers the path line, the breadcrumb and the rule below them.
	headerHeight = 3
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

	current   string
	resp      *redfish.Response
	groups    []redfish.Group
	rows      []row
	cursor    int
	fromCache bool

	body    viewport.Model
	editor  textinput.Model
	spinner spinner.Model
	help    help.Model

	// pending is the resource currently being fetched, if any.
	pending string
	loading bool

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
		pending: resource,
		loading: true,
		body:    viewport.New(),
		editor:  textinput.New(),
		spinner: spinner.New(),
		help:    help.New(),
	}

	m.rows = buildRows(nil, true)

	return m
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

	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd

	default:
		return m, nil
	}
}

// View renders the whole screen.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true

	return view
}

// WithResponse returns the model showing a response. It is a value method
// because Bubble Tea threads the model by value through Update.
func (m Model) WithResponse(resource string, resp *redfish.Response, fromCache bool) Model {
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
	m.cursor = m.firstSelectable()

	m.body.SetContent(m.renderBody())
	m.body.GotoTop()

	return m
}

// handleResize lays the panes out for a new terminal size.
func (m Model) handleResize(msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height

	m.body.SetWidth(max(m.bodyWidth()-frameWidth, 1))
	m.body.SetHeight(max(m.paneContentHeight()-1, 1))
	m.body.SetContent(m.renderBody())
	m.editor.SetWidth(max(m.width-frameWidth, 1))
	m.help.SetWidth(m.width)

	return m
}

// handleKey handles a key press in normal mode.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

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
		return m.goUp()

	case key.Matches(msg, m.keys.Reload):
		return m.startFetch(m.current, skipCache)

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

	return m.startFetch(r.link.Target, useCache)
}

// followAction opens an action's ActionInfo, which is the only part of an
// action a GET can reach. The target itself answers to POST and is listed so
// that vendor actions can be discovered at all.
func (m Model) followAction(link redfish.Link) (tea.Model, tea.Cmd) {
	if link.ActionInfo != "" {
		return m.startFetch(link.ActionInfo, useCache)
	}

	m.notice = "POST target — not retrievable; write support planned"

	return m, nil
}

// goUp walks one level towards the service root.
func (m Model) goUp() (tea.Model, tea.Cmd) {
	parent := redfish.Parent(m.current)
	if parent == m.current {
		return m, nil
	}

	return m.startFetch(parent, useCache)
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

// firstSelectable returns the index of the first row the cursor may rest on.
func (m Model) firstSelectable() int {
	for i, r := range m.rows {
		if r.selectable() {
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

	panes := lipgloss.JoinHorizontal(lipgloss.Top, m.renderLinks(), m.renderResponse())

	return strings.Join([]string{
		m.renderHeader(m.width),
		m.theme.Dim.Render(strings.Repeat("─", m.width)),
		panes,
		m.renderFooter(m.width),
	}, "\n")
}

// renderLinks frames the link pane.
func (m Model) renderLinks() string {
	inner := m.linkWidth() - frameWidth
	height := m.paneContentHeight()

	return m.pane(FocusLinks, m.linkWidth(),
		m.paneTitleLine(m.linkPaneTitle(), inner), m.renderLinkPane(inner, height-1))
}

// renderResponse frames the response pane.
func (m Model) renderResponse() string {
	inner := m.bodyWidth() - frameWidth

	return m.pane(FocusBody, m.bodyWidth(), m.paneTitleLine("Response", inner), m.body.View())
}

// pane frames a title and a body. Both dimensions passed to lipgloss are the
// outer ones — Style.Width and Style.Height include the border — and the
// content is cut to exactly the room inside it, so the two panes always agree
// and the screen never outgrows the terminal.
func (m Model) pane(focus Focus, outerWidth int, title string, body string) string {
	inner := outerWidth - frameWidth

	lines := append([]string{title}, strings.Split(body, "\n")...)
	lines = fit(lines, m.paneContentHeight())

	for i, line := range lines {
		lines[i] = ansi.Truncate(line, inner, "…")
	}

	return m.paneStyle(focus).
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
