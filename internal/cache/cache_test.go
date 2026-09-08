package cache_test

import (
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/breml/redfish-explorer/internal/cache"
	"github.com/breml/redfish-explorer/internal/redfish"
)

const (
	// testTTL is the lifetime used by every test that cares about expiry.
	testTTL = 5 * time.Minute

	// testKey is a stand-in for the absolute URL of a resource.
	testKey = "https://10.0.0.5/redfish/v1/Systems/1"
)

// clock is a hand-advanced time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

// newClock returns a clock at a fixed, arbitrary instant.
func newClock() *clock {
	return &clock{now: time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC)}
}

// Now reports the current instant.
func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

// Advance moves the clock forward.
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// response builds a minimal response for a given status.
func response(status int) *redfish.Response {
	return &redfish.Response{
		URL:        testKey,
		Path:       "/redfish/v1/Systems/1",
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Body:       []byte(`{"@odata.id":"/redfish/v1/Systems/1"}`),
	}
}

// newCache returns a cache driven by a hand-advanced clock.
func newCache(t *testing.T, ttl time.Duration) (*cache.Cache, *clock) {
	t.Helper()

	c := cache.New(ttl)
	clk := newClock()
	c.SetClock(clk.Now)

	return c, clk
}

func TestGetReturnsAStoredResponse(t *testing.T) {
	t.Parallel()

	c, _ := newCache(t, testTTL)
	want := response(http.StatusOK)

	c.Set(testKey, want)

	got, ok := c.Get(testKey)
	if !ok {
		t.Fatal("Get: want a hit")
	}

	if got != want {
		t.Error("Get returned a different response than was stored")
	}

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestGetMissesAnUnknownKey(t *testing.T) {
	t.Parallel()

	c, _ := newCache(t, testTTL)

	_, ok := c.Get(testKey)
	if ok {
		t.Error("Get: want a miss for a key that was never stored")
	}
}

func TestEntriesExpire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		advance time.Duration
		wantHit bool
	}{
		{name: "well within the ttl", advance: time.Minute, wantHit: true},
		{name: "just within the ttl", advance: testTTL - time.Nanosecond, wantHit: true},
		{name: "exactly at the ttl", advance: testTTL, wantHit: false},
		{name: "past the ttl", advance: testTTL + time.Minute, wantHit: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c, clk := newCache(t, testTTL)
			c.Set(testKey, response(http.StatusOK))
			clk.Advance(test.advance)

			_, ok := c.Get(testKey)
			if ok != test.wantHit {
				t.Errorf("Get hit = %t, want %t after %s", ok, test.wantHit, test.advance)
			}
		})
	}
}

func TestGetDropsAnExpiredEntry(t *testing.T) {
	t.Parallel()

	c, clk := newCache(t, testTTL)
	c.Set(testKey, response(http.StatusOK))
	clk.Advance(testTTL)

	_, _ = c.Get(testKey)

	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0: an expired entry should be dropped in passing", c.Len())
	}
}

func TestZeroTTLDisablesTheCache(t *testing.T) {
	t.Parallel()

	for _, ttl := range []time.Duration{0, -time.Minute} {
		t.Run(ttl.String(), func(t *testing.T) {
			t.Parallel()

			c, _ := newCache(t, ttl)
			c.Set(testKey, response(http.StatusOK))

			_, ok := c.Get(testKey)
			if ok {
				t.Error("Get: want a miss when caching is disabled")
			}

			if c.Len() != 0 {
				t.Errorf("Len = %d, want 0 when caching is disabled", c.Len())
			}
		})
	}
}

func TestErrorResponsesAreCached(t *testing.T) {
	t.Parallel()

	// A stable 404 is a finding in itself; re-fetching it changes nothing.
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusNotImplemented} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()

			c, _ := newCache(t, testTTL)
			c.Set(testKey, response(status))

			got, ok := c.Get(testKey)
			if !ok {
				t.Fatalf("Get: want a hit for a %d response", status)
			}

			if got.StatusCode != status {
				t.Errorf("StatusCode = %d, want %d", got.StatusCode, status)
			}
		})
	}
}

func TestSetReplacesAnEarlierResponse(t *testing.T) {
	t.Parallel()

	c, _ := newCache(t, testTTL)
	c.Set(testKey, response(http.StatusNotFound))
	c.Set(testKey, response(http.StatusOK))

	got, ok := c.Get(testKey)
	if !ok {
		t.Fatal("Get: want a hit")
	}

	if got.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", got.StatusCode, http.StatusOK)
	}

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestSetIgnoresANilResponse(t *testing.T) {
	t.Parallel()

	c, _ := newCache(t, testTTL)
	c.Set(testKey, nil)

	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0", c.Len())
	}
}

func TestInvalidate(t *testing.T) {
	t.Parallel()

	c, _ := newCache(t, testTTL)
	c.Set(testKey, response(http.StatusOK))
	c.Invalidate(testKey)

	_, ok := c.Get(testKey)
	if ok {
		t.Error("Get: want a miss after Invalidate")
	}

	// Invalidating something absent must not panic.
	c.Invalidate("https://10.0.0.5/redfish/v1/Nope")
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	c, _ := newCache(t, testTTL)

	const workers = 8

	var wg sync.WaitGroup

	for worker := range workers {
		wg.Go(func() {
			key := testKey + "/" + strconv.Itoa(worker)

			for range 100 {
				c.Set(key, response(http.StatusOK))
				_, _ = c.Get(key)
				c.Invalidate(key)
				_ = c.Len()
			}
		})
	}

	wg.Wait()
}
