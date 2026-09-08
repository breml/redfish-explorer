package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/breml/redfish-explorer/internal/cache"
	"github.com/breml/redfish-explorer/internal/redfish"
)

// cachePolicy says whether a fetch may be answered from the cache.
type cachePolicy int

const (
	// useCache answers from the cache when a fresh entry is held.
	useCache cachePolicy = iota
	// skipCache forces a request and replaces the cached entry.
	skipCache
)

// fetchedMsg reports the outcome of one fetch.
type fetchedMsg struct {
	// resource is the path that was asked for. It is compared against the
	// pending path so that a late answer cannot land under a new location.
	resource string
	response *redfish.Response
	cached   bool
	err      error
}

// fetchCmd fetches a resource, consulting the cache unless told otherwise.
func fetchCmd(client *redfish.Client, store *cache.Cache, resource string, policy cachePolicy) tea.Cmd {
	return func() tea.Msg {
		// The cache is keyed by absolute URL, not by path, so that a key stays
		// correct if rfx ever talks to more than one endpoint.
		key, err := client.Resolve(resource)
		if err != nil {
			return fetchedMsg{resource: resource, err: err}
		}

		if policy == useCache {
			cached, ok := store.Get(key)
			if ok {
				return fetchedMsg{resource: resource, response: cached, cached: true}
			}
		}

		response, err := client.Fetch(context.Background(), resource)
		if err != nil {
			return fetchedMsg{resource: resource, err: err}
		}

		// Error statuses are cached like any other answer; a transport failure
		// never gets here, and would not be worth remembering if it did.
		store.Set(key, response)

		return fetchedMsg{resource: resource, response: response}
	}
}

// handleFetched folds the outcome of a fetch into the model.
func (m Model) handleFetched(msg fetchedMsg) Model {
	// A late answer for a resource the user has already navigated away from
	// must not be drawn under the new location.
	if msg.resource != m.pending {
		return m
	}

	m.pending = ""
	m.loading = false

	if msg.err != nil {
		return m.withFetchError(msg.resource, msg.err)
	}

	return m.WithResponse(msg.resource, msg.response, msg.cached)
}

// withFetchError shows a failure to reach a resource while keeping the current
// location and its links, so that a failed step never loses the user's place.
func (m Model) withFetchError(resource string, err error) Model {
	m.err = fmt.Errorf("fetching %s: %w", resource, err)
	m.errResource = resource
	m.notice = ""

	m.body.SetContent(m.renderBody())
	m.body.GotoTop()

	return m
}

// startFetch begins loading a resource, landing the cursor on the given row
// once it arrives. The navKind and the cursor travel with the request rather
// than being applied here, so that a fetch which never lands leaves both the
// history and the user's place untouched.
func (m Model) startFetch(resource string, policy cachePolicy, nav navKind, cursor int) (Model, tea.Cmd) {
	m.pending = resource
	m.pendingNav = nav
	m.pendingCursor = cursor
	m.loading = true
	m.notice = ""

	return m, fetchCmd(m.client, m.store, resource, policy)
}
