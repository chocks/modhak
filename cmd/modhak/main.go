// Command modhak is an advisor and dashboard for MariaDB and MySQL.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/config"
	"github.com/chocks/modhak/internal/scan"
	"github.com/chocks/modhak/internal/target"
	"github.com/chocks/modhak/internal/web"
)

const usage = `modhak: advisor and dashboard for MariaDB and MySQL

Usage:
  modhak <command> [flags]

Commands:
  scan           find index and AUTO_INCREMENT problems on each target
  serve          web UI with the same findings (auth is off; binds to loopback)
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
	case "serve":
		return serveCmd(ctx, args[1:], stdout, stderr)
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
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "modhak.yaml", "path to the config file")
	only := fs.String("target", "", "only this target (default: all)")
	asJSON := fs.Bool("json", false, "print JSON")
	af := addAdvisorFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	advCfg, err := af.config()
	if err != nil {
		fmt.Fprintln(stderr, "modhak:", err)
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

	results := scan.Run(ctx, targets, advCfg, time.Now())
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

// advisorFlags are the thresholds shared by scan and serve.
type advisorFlags struct {
	minUptime *time.Duration
	warn      *float64
	crit      *float64
}

func addAdvisorFlags(fs *flag.FlagSet) *advisorFlags {
	def := advisor.DefaultConfig()
	return &advisorFlags{
		minUptime: fs.Duration("min-uptime", def.MinUptime, "minimum server uptime before unused-index verdicts (0 = no minimum)"),
		warn:      fs.Float64("headroom-warn", def.HeadroomWarn, "AUTO_INCREMENT range fraction (0-1) that warns"),
		crit:      fs.Float64("headroom-critical", def.HeadroomCritical, "AUTO_INCREMENT range fraction (0-1) that is critical"),
	}
}

func (a *advisorFlags) config() (advisor.Config, error) {
	if *a.minUptime < 0 || *a.warn <= 0 || *a.warn > 1 || *a.crit <= 0 || *a.crit > 1 || *a.warn > *a.crit {
		return advisor.Config{}, errors.New("need min-uptime >= 0 and 0 < headroom-warn <= headroom-critical <= 1")
	}
	min := *a.minUptime
	// The advisor treats a zero MinUptime as "use the default", so an explicit
	// --min-uptime 0 (no minimum) is passed as the smallest positive duration.
	if min == 0 {
		min = time.Nanosecond
	}
	return advisor.Config{MinUptime: min, HeadroomWarn: *a.warn, HeadroomCritical: *a.crit}, nil
}

// serveCmd runs the web UI until ctx is cancelled. Auth is off in M1, so it
// binds to loopback by default and warns when asked to bind elsewhere.
func serveCmd(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "modhak.yaml", "path to the config file")
	listen := fs.String("listen", "127.0.0.1:8080", "address to listen on")
	ttl := fs.Duration("cache-ttl", time.Minute, "how long a scan result is reused before the target is scanned again")
	af := addAdvisorFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	advCfg, err := af.config()
	if err == nil && *ttl <= 0 {
		err = errors.New("cache-ttl must be positive")
	}
	if err != nil {
		fmt.Fprintln(stderr, "modhak:", err)
		return 2
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "modhak:", err)
		return 2
	}

	logger := slog.New(slog.NewTextHandler(stderr, nil))
	srv, err := web.New(web.Options{Targets: cfg.Targets, Cache: scan.NewCache(ctx, advCfg, *ttl), Logger: logger})
	if err != nil {
		fmt.Fprintln(stderr, "modhak:", err)
		return 1
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(stderr, "modhak:", err)
		return 1
	}
	if host, _, err := net.SplitHostPort(ln.Addr().String()); err == nil {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			logger.Warn("listening on a non-loopback address with auth off: anyone who can reach it can read findings and schema names")
		}
	}
	hs := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      3 * time.Minute, // a first view can wait for target scans
		IdleTimeout:       60 * time.Second,
	}
	fmt.Fprintf(stdout, "modhak listening on http://%s\n", ln.Addr())

	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	select {
	case err := <-errc:
		fmt.Fprintln(stderr, "modhak:", err)
		return 1
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := hs.Shutdown(shutdown); err != nil {
		fmt.Fprintln(stderr, "modhak: shutdown:", err)
		return 1
	}
	return 0
}
