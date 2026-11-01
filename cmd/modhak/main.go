// Command modhak is an advisor and dashboard for MariaDB and MySQL.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/config"
	"github.com/chocks/modhak/internal/scan"
	"github.com/chocks/modhak/internal/target"
)

const usage = `modhak: advisor and dashboard for MariaDB and MySQL

Usage:
  modhak <command> [flags]

Commands:
  scan           find index and AUTO_INCREMENT problems on each target
  capabilities   detect flavor, version and features of each target
  version        print the version
`

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, "modhak", version)
		return 0
	case "scan":
		return scanCmd(ctx, args[1:], stdout, stderr)
	case "capabilities":
		return capabilities(ctx, args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "modhak: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// capabilities prints detected capabilities per target. Exit status is 1 if
// any target failed, but every target is still attempted and printed.
func capabilities(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "modhak.yaml", "path to the config file")
	only := fs.String("target", "", "only this target (default: all)")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "modhak:", err)
		return 2
	}
	targets := cfg.Targets
	if *only != "" {
		t, ok := cfg.Target(*only)
		if !ok {
			fmt.Fprintf(stderr, "modhak: no target named %q in %s\n", *only, *cfgPath)
			return 2
		}
		targets = []config.Target{t}
	}

	results := target.DetectCapabilities(ctx, targets)
	failed := false
	for _, r := range results {
		if r.Err != nil {
			failed = true
		}
	}
	if *asJSON {
		type entry struct {
			Target string `json:"target"`
			Addr   string `json:"addr"`
			Error  string `json:"error,omitempty"`
			Report any    `json:"capabilities,omitempty"`
		}
		var out []entry
		for _, r := range results {
			e := entry{Target: r.Target.Name, Addr: r.Target.Addr()}
			if r.Err != nil {
				e.Error = r.Err.Error()
			} else {
				e.Report = r.Report
			}
			out = append(out, e)
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(out)
	} else {
		for i, r := range results {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "== %s (%s)\n", r.Target.Name, r.Target.Addr())
			if r.Err != nil {
				fmt.Fprintf(stdout, "error: %v\n", r.Err)
				continue
			}
			fmt.Fprint(stdout, r.Report.Text())
		}
	}
	if failed {
		return 1
	}
	return 0
}

// scanCmd scans each target and prints findings. Exit status is 1 if any
// target could not be scanned (findings alone do not fail the run), 2 on
// usage or config errors.
func scanCmd(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	def := advisor.DefaultConfig()
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "modhak.yaml", "path to the config file")
	only := fs.String("target", "", "only this target (default: all)")
	asJSON := fs.Bool("json", false, "print JSON")
	minUptime := fs.Duration("min-uptime", def.MinUptime, "minimum server uptime before unused-index verdicts")
	warn := fs.Float64("headroom-warn", def.HeadroomWarn, "AUTO_INCREMENT range fraction (0-1) that warns")
	crit := fs.Float64("headroom-critical", def.HeadroomCritical, "AUTO_INCREMENT range fraction (0-1) that is critical")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *minUptime < 0 || *warn <= 0 || *warn > 1 || *crit <= 0 || *crit > 1 || *warn > *crit {
		fmt.Fprintln(stderr, "modhak: need min-uptime >= 0 and 0 < headroom-warn <= headroom-critical <= 1")
		return 2
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "modhak:", err)
		return 2
	}
	targets := cfg.Targets
	if *only != "" {
		t, ok := cfg.Target(*only)
		if !ok {
			fmt.Fprintf(stderr, "modhak: no target named %q in %s\n", *only, *cfgPath)
			return 2
		}
		targets = []config.Target{t}
	}

	// The advisor treats a zero MinUptime as "use the default", so an explicit
	// --min-uptime 0 (no minimum) is passed as the smallest positive duration.
	if *minUptime == 0 {
		*minUptime = time.Nanosecond
	}
	results := scan.Run(ctx, targets, advisor.Config{MinUptime: *minUptime, HeadroomWarn: *warn, HeadroomCritical: *crit}, time.Now())
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
	} else {
		fmt.Fprint(stdout, scan.Text(results))
	}
	for _, r := range results {
		if r.Failed() {
			return 1
		}
	}
	return 0
}
