package scan

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chocks/modhak/internal/config"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func newTestCache(scans *atomic.Int32, clk *clock, delay time.Duration) *Cache {
	return &Cache{
		Scan: func(_ context.Context, t config.Target) TargetResult {
			scans.Add(1)
			time.Sleep(delay)
			return TargetResult{Target: t.Name, Addr: t.Addr()}
		},
		Now: clk.now, TTL: time.Minute, MinRefresh: 10 * time.Second, Timeout: time.Second,
	}
}

func TestCacheTTLAndRefresh(t *testing.T) {
	var scans atomic.Int32
	clk := &clock{t: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}
	c := newTestCache(&scans, clk, 0)
	a := config.Target{Name: "a", Host: "h", Port: 3306}
	b := config.Target{Name: "b", Host: "h", Port: 3306}

	steps := []struct {
		name      string
		advance   time.Duration
		target    config.Target
		refresh   bool
		wantScans int32
	}{
		{"first get scans", 0, a, false, 1},
		{"second get is cached", 5 * time.Second, a, false, 1},
		{"refresh inside MinRefresh is ignored", 0, a, true, 1}, // age 5s < 10s
		{"refresh after MinRefresh rescans", 6 * time.Second, a, true, 2},
		{"another target is independent", 0, b, false, 3},
		{"get within TTL is cached", 30 * time.Second, a, false, 3},
		{"get after TTL rescans", 31 * time.Second, a, false, 4},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			clk.add(s.advance)
			c.Get(s.target, s.refresh)
			if got := scans.Load(); got != s.wantScans {
				t.Fatalf("scans = %d, want %d", got, s.wantScans)
			}
		})
	}
}

func TestCacheConcurrentRequestsShareOneScan(t *testing.T) {
	var scans atomic.Int32
	clk := &clock{t: time.Now()}
	c := newTestCache(&scans, clk, 50*time.Millisecond)
	tgt := config.Target{Name: "a", Host: "h", Port: 3306}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Get(tgt, false) }()
	}
	wg.Wait()
	if got := scans.Load(); got != 1 {
		t.Fatalf("scans = %d, want 1", got)
	}
}

func TestCacheGetAllKeepsOrderAndScansConcurrently(t *testing.T) {
	var scans atomic.Int32
	clk := &clock{t: time.Now()}
	c := newTestCache(&scans, clk, 100*time.Millisecond)
	ts := []config.Target{{Name: "z", Host: "h", Port: 1}, {Name: "a", Host: "h", Port: 2}, {Name: "m", Host: "h", Port: 3}}
	start := time.Now()
	got := c.GetAll(ts, false)
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Errorf("targets were scanned one after another (%s)", elapsed)
	}
	for i, want := range []string{"z", "a", "m"} {
		if got[i].Target != want {
			t.Errorf("result %d = %q, want %q", i, got[i].Target, want)
		}
	}
}
