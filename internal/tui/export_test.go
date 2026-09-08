package tui

// StaleFetchedMsg builds a fetch result for a resource that is not the one the
// model is waiting for, so that the guard against late answers can be tested.
func StaleFetchedMsg(resource string) any {
	return fetchedMsg{resource: resource, response: nil}
}

// Endpoint returns the connected endpoint. It exists for tests only.
func (m Model) Endpoint() string {
	return m.cfg.Endpoint
}
