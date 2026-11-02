// Package web serves the findings pages: server-rendered html/template with
// htmx for filtering (ADR 0001). Handlers are thin: they look up cached scan
// results and render them; all analysis lives in the advisor.
//
// An HX-Request gets only the findings fragment; any other request gets the
// full page, from the same template. Every request carries a Principal
// (AGENTS.md); with auth off it is the implicit viewer.
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/auth"
	"github.com/chocks/modhak/internal/config"
	"github.com/chocks/modhak/internal/scan"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Options configures a Server.
type Options struct {
	Targets []config.Target
	Cache   *scan.Cache
	// Now returns the current time (default time.Now).
	Now    func() time.Time
	Logger *slog.Logger
}

// Server renders the pages.
type Server struct {
	targets  []config.Target
	byName   map[string]config.Target
	cache    *scan.Cache
	now      func() time.Time
	log      *slog.Logger
	overview *template.Template
	findings *template.Template
}

// New builds a Server and parses its templates.
func New(o Options) (*Server, error) {
	s := &Server{targets: o.Targets, cache: o.Cache, now: o.Now, log: o.Logger, byName: map[string]config.Target{}}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	for _, t := range o.Targets {
		s.byName[t.Name] = t
	}
	funcs := template.FuncMap{
		"loc":      scan.Location,
		"describe": scan.Describe,
	}
	parse := func(page string) (*template.Template, error) {
		return template.New("").Funcs(funcs).ParseFS(templateFS, "templates/base.html", "templates/"+page)
	}
	var err error
	if s.overview, err = parse("overview.html"); err != nil {
		return nil, err
	}
	if s.findings, err = parse("findings.html"); err != nil {
		return nil, err
	}
	return s, nil
}

// Handler returns the HTTP handler with all routes.
func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(staticFS, "static")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleOverview)
	mux.HandleFunc("GET /t/{target}/findings", s.handleFindings)
	mux.HandleFunc("GET /t/{target}/findings.json", s.handleFindingsJSON)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	return s.middleware(mux)
}

// securityHeaders restrict the page to its own origin. htmx is configured
// (in base.html) not to inject inline styles or use eval, so no 'unsafe-*'.
var securityHeaders = map[string]string{
	"Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'",
	"X-Content-Type-Options":  "nosniff",
	"Referrer-Policy":         "no-referrer",
	"X-Frame-Options":         "DENY",
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		for k, v := range securityHeaders {
			w.Header().Set(k, v)
		}
		// Auth is off in M1: every request is the implicit principal.
		r = r.WithContext(auth.NewContext(r.Context(), auth.Implicit()))
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", s.now().Sub(start).Milliseconds())
	})
}

func principal(r *http.Request) auth.Principal {
	p, _ := auth.FromContext(r.Context())
	return p
}

// shortDuration drops the zero units Duration.String adds ("1m0s" -> "1m").
func shortDuration(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}

func ago(now, then time.Time) string {
	d := now.Sub(then)
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func (s *Server) render(w http.ResponseWriter, t *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("render", "template", name, "err", err)
	}
}

type overviewRow struct {
	Name, Addr, Server, Err, Ago string
	Critical, Warning, Info      int
}

type overviewData struct {
	Principal auth.Principal
	TTL       string
	Rows      []overviewRow
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "1"
	results := s.cache.GetAll(s.targets, refresh)
	if refresh {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	now := s.now()
	data := overviewData{Principal: principal(r), TTL: shortDuration(s.cache.TTL)}
	for _, res := range results {
		row := overviewRow{Name: res.Target, Addr: res.Addr, Err: res.Error, Ago: ago(now, res.At)}
		if res.Capabilities != nil {
			row.Server = string(res.Capabilities.Flavor) + " " + res.Capabilities.Version
		}
		c := scan.Counts(res.TargetResult)
		row.Critical, row.Warning, row.Info = c[advisor.SeverityCritical], c[advisor.SeverityWarning], c[advisor.SeverityInfo]
		data.Rows = append(data.Rows, row)
	}
	s.render(w, s.overview, "base", data)
}

type option struct {
	Value, Label string
	Selected     bool
}

type capRow struct{ Name, State, Hint string }

type findingsData struct {
	Principal                    auth.Principal
	Target, Addr, Server, Err    string
	ScannedAt, Ago               string
	Path, RefreshURL, JSONURL    string
	Findings                     []advisor.Finding
	Total                        int
	Filtered                     bool
	Skipped                      []advisor.Skipped
	Caps                         []capRow
	CapsAttention                int
	KindOptions, SeverityOptions []option
}

var severities = []advisor.Severity{advisor.SeverityCritical, advisor.SeverityWarning, advisor.SeverityInfo}

// filters parses and validates the kind and severity query parameters.
func filters(q url.Values) (kind string, sev advisor.Severity, err error) {
	kind = q.Get("kind")
	if kind != "" {
		ok := false
		for _, k := range advisor.Kinds() {
			ok = ok || k == kind
		}
		if !ok {
			return "", "", fmt.Errorf("unknown kind %q", kind)
		}
	}
	sev = advisor.Severity(q.Get("severity"))
	if sev != "" {
		ok := false
		for _, s := range severities {
			ok = ok || s == sev
		}
		if !ok {
			return "", "", fmt.Errorf("unknown severity %q", sev)
		}
	}
	return kind, sev, nil
}

func query(kind string, sev advisor.Severity, extra ...string) string {
	v := url.Values{}
	if kind != "" {
		v.Set("kind", kind)
	}
	if sev != "" {
		v.Set("severity", string(sev))
	}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (config.Target, bool) {
	t, ok := s.byName[r.PathValue("target")]
	if !ok {
		http.NotFound(w, r)
	}
	return t, ok
}

func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	t, ok := s.lookup(w, r)
	if !ok {
		return
	}
	kind, sev, err := filters(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := "/t/" + url.PathEscape(t.Name) + "/findings"
	if r.URL.Query().Get("refresh") == "1" {
		// Rescan, then redirect so reloading the page does not rescan again.
		s.cache.Get(t, true)
		http.Redirect(w, r, path+query(kind, sev), http.StatusSeeOther)
		return
	}
	res, at := s.cache.Get(t, false)
	shown := scan.Filter(res, kind, sev)
	now := s.now()
	d := findingsData{
		Principal: principal(r), Target: t.Name, Addr: res.Addr, Err: res.Error,
		Path: path, RefreshURL: path + query(kind, sev, "refresh", "1"), JSONURL: path + ".json" + query(kind, sev),
		Findings: shown.Findings, Total: len(res.Findings), Filtered: kind != "" || sev != "",
		Skipped: res.Skipped, ScannedAt: at.UTC().Format("2006-01-02 15:04:05 UTC"), Ago: ago(now, at),
	}
	if rep := res.Capabilities; rep != nil {
		d.Server = string(rep.Flavor) + " " + rep.Version
		for _, c := range []struct {
			name string
			cap  capabilityView
		}{
			{"performance_schema", view(rep.PerformanceSchema.State.String(), rep.PerformanceSchema.Hint)},
			{"index usage (unused-index analysis)", view(rep.IndexUsage.State.String(), rep.IndexUsage.Hint)},
			{"userstat (MariaDB)", view(rep.Userstat.State.String(), rep.Userstat.Hint)},
			{"sys schema", view(rep.Sys.State.String(), rep.Sys.Hint)},
			{"replication status", view(rep.Replication.State.String(), rep.Replication.Hint)},
		} {
			d.Caps = append(d.Caps, capRow{Name: c.name, State: c.cap.state, Hint: c.cap.hint})
			if c.cap.state != "available" {
				d.CapsAttention++
			}
		}
	}
	d.KindOptions = []option{{Value: "", Label: "All kinds", Selected: kind == ""}}
	for _, k := range advisor.Kinds() {
		d.KindOptions = append(d.KindOptions, option{Value: k, Label: k, Selected: k == kind})
	}
	d.SeverityOptions = []option{{Value: "", Label: "All severities", Selected: sev == ""}}
	for _, v := range severities {
		d.SeverityOptions = append(d.SeverityOptions, option{Value: string(v), Label: string(v), Selected: v == sev})
	}

	w.Header().Add("Vary", "HX-Request")
	if r.Header.Get("HX-Request") == "true" {
		s.render(w, s.findings, "findings-list", d)
		return
	}
	s.render(w, s.findings, "base", d)
}

type capabilityView struct{ state, hint string }

func view(state, hint string) capabilityView { return capabilityView{state, hint} }

// handleFindingsJSON returns the same data as `modhak scan --json` for one
// target, with the same kind and severity filters as the page.
func (s *Server) handleFindingsJSON(w http.ResponseWriter, r *http.Request) {
	t, ok := s.lookup(w, r)
	if !ok {
		return
	}
	kind, sev, err := filters(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, _ := s.cache.Get(t, false)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(scan.Filter(res, kind, sev))
}
