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

	"github.com/chocks/modhak/internal/config"
	"github.com/chocks/modhak/internal/target"
)

const usage = `modhak: advisor and dashboard for MariaDB and MySQL

Usage:
  modhak <command> [flags]

Commands:
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
