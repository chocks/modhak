// Package scan runs the one-shot pipeline behind `modhak scan`: capability
// detection, snapshot, advisor, per target. It keeps cmd/modhak free of
// business logic.
package scan

import (
	"context"
	"sync"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/capability"
	"github.com/chocks/modhak/internal/collector"
	"github.com/chocks/modhak/internal/config"
	"github.com/chocks/modhak/internal/target"
)

// TargetResult is the outcome for one target. On failure only Target, Addr
// and Error are set. It is also the JSON shape of `modhak scan --json`.
type TargetResult struct {
	Target       string             `json:"target"`
	Addr         string             `json:"addr"`
	Error        string             `json:"error,omitempty"`
	Capabilities *capability.Report `json:"capabilities,omitempty"`
	Findings     []advisor.Finding  `json:"findings"`
	Skipped      []advisor.Skipped  `json:"skipped,omitempty"`
}

// Failed reports whether the target could not be scanned.
func (r TargetResult) Failed() bool { return r.Error != "" }

// Run scans every target concurrently. A failure on one target is reported in
// its own result and never affects the others (ADR 0008). Results are in the
// order of targets. now is passed to the advisor (it never reads a clock).
func Run(ctx context.Context, targets []config.Target, cfg advisor.Config, now time.Time) []TargetResult {
	out := make([]TargetResult, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = scanOne(ctx, t, cfg, now)
		}()
	}
	wg.Wait()
	return out
}

func scanOne(ctx context.Context, t config.Target, cfg advisor.Config, now time.Time) TargetResult {
	res := TargetResult{Target: t.Name, Addr: t.Addr(), Findings: []advisor.Finding{}}
	fail := func(err error) TargetResult {
		res.Error = target.Explain(t, err).Error()
		return res
	}
	c, err := target.Open(t)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer c.Close()
	rep, err := capability.Detect(ctx, c)
	if err != nil {
		return fail(err)
	}
	sess, err := c.Session(ctx)
	if err != nil {
		return fail(err)
	}
	defer sess.Close()
	in, err := collector.Snapshot(ctx, sess, t.Name, rep, now)
	if err != nil {
		return fail(err)
	}
	r := advisor.Analyze(in, cfg)
	res.Capabilities = rep
	if r.Findings != nil {
		res.Findings = r.Findings
	}
	res.Skipped = r.Skipped
	return res
}
