package tui

// maxHistory bounds the navigation trail. A session can visit a great many
// resources, and only the recent ones are worth stepping back through.
const maxHistory = 256

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
	// navBack is a step back through the history, which goBack has already
	// unwound.
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

// popVisit removes the most recent visit and returns it. It reports false when
// the history is empty, which is where a session began.
func (m Model) popVisit() (model Model, previous visit, ok bool) {
	if len(m.history) == 0 {
		return m, visit{}, false
	}

	last := len(m.history) - 1
	previous = m.history[last]
	m.history = m.history[:last]

	return m, previous, true
}
