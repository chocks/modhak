package target

import (
	"context"
	"sync"

	"github.com/chocks/modhak/internal/capability"
	"github.com/chocks/modhak/internal/config"
)

// Result is the outcome of inspecting one target. Exactly one of Report and
// Err is set.
type Result struct {
	Target config.Target
	Report *capability.Report
	Err    error
}

// DetectCapabilities detects capabilities for each target concurrently. A
// failure on one target is reported in its Result and never affects the
// others (ADR 0008). Results are in the order of ts.
func DetectCapabilities(ctx context.Context, ts []config.Target) []Result {
	out := make([]Result, len(ts))
	var wg sync.WaitGroup
	for i, t := range ts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = detectOne(ctx, t)
		}()
	}
	wg.Wait()
	return out
}

func detectOne(ctx context.Context, t config.Target) Result {
	c, err := Open(t)
	if err != nil {
		return Result{Target: t, Err: err}
	}
	defer c.Close()
	rep, err := capability.Detect(ctx, c)
	if err != nil {
		return Result{Target: t, Err: Explain(t, err)}
	}
	return Result{Target: t, Report: rep}
}
