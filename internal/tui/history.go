package tui

// maxHistory bounds the navigation trail. A session can visit a great many
// resources, and only the recent ones are worth stepping back through.
const maxHistory = 256

// noCursor asks for the cursor to start at the top of the link pane, rather
// than on a remembered row.
const noCursor = -1

// visit is a place the user has been, and where the cursor stood when they
// left it.
type visit struct {
	resource string
	cursor   int
}

// navKind says how a pending fetch reached its resource, which decides what it
// does to the navigation history when it lands.
type navKind int

const (
	// navForward is a step to a new place: the departure joins the history.
	navForward navKind = iota
	// navBack is a step back through the history: the visit it heads for
	// leaves the trail when the fetch lands, not when the key is pressed.
	navBack
	// navStay is a fetch that does not move: the initial load and a reload.
	navStay
)

// pushVisit records the place a forward navigation is leaving.
func (m Model) pushVisit(resource string, cursor int) Model {
	m.history = append(m.history, visit{resource: resource, cursor: cursor})

	if len(m.history) > maxHistory {
		m.history = m.history[len(m.history)-maxHistory:]
	}

	return m
}

// lastVisit returns the place a step back would return to, without taking it
// off the trail. It reports false when the history is empty, which is where a
// session began.
func (m Model) lastVisit() (previous visit, ok bool) {
	if len(m.history) == 0 {
		return visit{}, false
	}

	return m.history[len(m.history)-1], true
}

// dropVisit takes the most recent visit off the trail, once a step back has
// actually landed on it.
func (m Model) dropVisit() Model {
	if len(m.history) == 0 {
		return m
	}

	m.history = m.history[:len(m.history)-1]

	return m
}
