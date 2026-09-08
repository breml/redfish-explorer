package tui

// StaleFetchedMsg builds a fetch result for a resource that is not the one the
// model is waiting for, so that the guard against late answers can be tested.
func StaleFetchedMsg(resource string) any {
	return fetchedMsg{resource: resource, response: nil}
}

// CopyResultMsg builds the message a finished copy reports, so that both
// outcomes can be exercised without depending on whether the machine running
// the tests has a clipboard tool installed.
func CopyResultMsg(err error) any {
	return copiedMsg{err: err}
}

// Endpoint returns the connected endpoint. It exists for tests only.
func (m Model) Endpoint() string {
	return m.cfg.Endpoint
}
