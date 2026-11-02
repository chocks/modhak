package scan

import (
	"context"
	"sync"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/config"
)

// Cache keeps the latest scan of each target so page views do not hit the
// production database every time. Concurrent requests for the same target wait
// for one scan instead of starting several. Failures are cached too, so a
// down target is not hammered.
type Cache struct {
	// Scan scans one target. Use NewCache for the real implementation.
	Scan func(ctx context.Context, t config.Target) TargetResult
	// Now returns the current time; tests replace it.
	Now func() time.Time
	// TTL is how long a result is served. MinRefresh is how old a result must
	// be before an explicit refresh actually rescans (a floor on how often a
	// browser can trigger scans).
	TTL        time.Duration
	MinRefresh time.Duration
	// Timeout bounds one scan, independent of the request that started it.
	Timeout time.Duration

	base    context.Context
	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	mu     sync.Mutex
	result TargetResult
	at     time.Time
	has    bool
}

// NewCache returns a cache that runs real scans with cfg. ctx bounds all
// scans (cancel it on shutdown); scans do not stop when one HTTP client
// disconnects.
func NewCache(ctx context.Context, cfg advisor.Config, ttl time.Duration) *Cache {
	return &Cache{
		Scan: func(ctx context.Context, t config.Target) TargetResult {
			return scanOne(ctx, t, cfg, time.Now())
		},
		Now: time.Now, TTL: ttl, MinRefresh: 10 * time.Second, Timeout: 2 * time.Minute,
		base: ctx, entries: map[string]*entry{},
	}
}

func (c *Cache) entry(name string) *entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]*entry{}
	}
	e := c.entries[name]
	if e == nil {
		e = &entry{}
		c.entries[name] = e
	}
	return e
}

// Get returns the target's result and when it was scanned. It rescans if the
// cached result is older than TTL, or, when refresh is set, older than
// MinRefresh.
func (c *Cache) Get(t config.Target, refresh bool) (TargetResult, time.Time) {
	e := c.entry(t.Name)
	e.mu.Lock()
	defer e.mu.Unlock()
	now := c.Now()
	if e.has {
		age := now.Sub(e.at)
		if age < c.TTL && !(refresh && age >= c.MinRefresh) {
			return e.result, e.at
		}
	}
	base := c.base
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, c.Timeout)
	defer cancel()
	e.result, e.at, e.has = c.Scan(ctx, t), c.Now(), true
	return e.result, e.at
}

// Scanned is a result with its scan time.
type Scanned struct {
	TargetResult
	At time.Time `json:"-"`
}

// GetAll fetches every target concurrently, in the order of ts.
func (c *Cache) GetAll(ts []config.Target, refresh bool) []Scanned {
	out := make([]Scanned, len(ts))
	var wg sync.WaitGroup
	for i, t := range ts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, at := c.Get(t, refresh)
			out[i] = Scanned{TargetResult: r, At: at}
		}()
	}
	wg.Wait()
	return out
}
