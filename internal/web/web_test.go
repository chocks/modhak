package web

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/capability"
	"github.com/chocks/modhak/internal/config"
	"github.com/chocks/modhak/internal/scan"
)

var t0 = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func finding(kind string, sev advisor.Severity, idx string, ev advisor.Evidence, sql ...string) advisor.Finding {
	t := advisor.Target{Name: "prod", Schema: "shop", Table: "orders", Index: idx}
	return advisor.Finding{ID: kind + ":prod:shop:orders:" + idx, Kind: kind, Severity: sev, Target: t, Evidence: ev, SuggestedSQL: sql}
}

func results() map[string]scan.TargetResult {
	days := 40.0
	prod := scan.TargetResult{
		Target: "prod", Addr: "db:3306",
		Capabilities: &capability.Report{
			Flavor: capability.FlavorMariaDB, Version: "11.4.13",
			PerformanceSchema: capability.Capability{State: capability.Unavailable, Hint: "Enable performance_schema (set performance_schema=ON) to get digests."},
			IndexUsage:        capability.Capability{State: capability.Unavailable, Hint: "Enable performance_schema to get unused-index analysis."},
			Userstat:          capability.Capability{State: capability.Available},
			Sys:               capability.Capability{State: capability.Degraded, Hint: "needs performance_schema"},
			Replication:       capability.Capability{State: capability.Available},
		},
		Findings: []advisor.Finding{
			finding(advisor.KindAutoIncHeadroom, advisor.SeverityCritical, "",
				advisor.AutoIncEvidence{ColumnType: "int(11)", Next: 2100000000, Max: 2147483647, UsedFraction: 0.978, Remaining: 47483648, RoughDaysToOverflow: &days},
				"ALTER TABLE `shop`.`orders` MODIFY `id` BIGINT NOT NULL AUTO_INCREMENT"),
			finding(advisor.KindDuplicateIndex, advisor.SeverityWarning, "idx_dup",
				advisor.DuplicateEvidence{Kept: "idx_a", Duplicate: "idx_dup", Columns: []advisor.IndexColumn{{Name: "a"}}},
				"ALTER TABLE `shop`.`orders` DROP INDEX `idx_dup`"),
			// Hostile text must be escaped.
			finding(advisor.KindRedundantIndex, advisor.SeverityWarning, "<script>alert(1)</script>",
				advisor.RedundantEvidence{Redundant: "<script>alert(1)</script>", CoveredBy: "idx_ab", Columns: []advisor.IndexColumn{{Name: "a"}}, CoveringBy: []advisor.IndexColumn{{Name: "a"}, {Name: "b"}}},
				"ALTER TABLE `shop`.`orders` DROP INDEX `<script>alert(1)</script>`"),
		},
		Skipped: []advisor.Skipped{{Analysis: advisor.SkipUnusedIndexes, Reason: "Enable performance_schema to get unused-index analysis."}},
	}
	clean := scan.TargetResult{Target: "clean", Addr: "db2:3306", Findings: []advisor.Finding{},
		Capabilities: &capability.Report{Flavor: capability.FlavorMySQL, Version: "8.4.13"}}
	down := scan.TargetResult{Target: "down", Addr: "db3:3306", Findings: []advisor.Finding{},
		Error: `target "down" (db3:3306): cannot connect: <b>refused</b>`}
	return map[string]scan.TargetResult{"prod": prod, "clean": clean, "down": down}
}

func newServer(t *testing.T) (*Server, *atomic.Int32, *http.Client, string, func()) {
	t.Helper()
	var scans atomic.Int32
	res := results()
	cache := &scan.Cache{
		Scan: func(_ context.Context, tg config.Target) scan.TargetResult {
			scans.Add(1)
			return res[tg.Name]
		},
		Now: func() time.Time { return t0 }, TTL: time.Minute, MinRefresh: 10 * time.Second, Timeout: time.Second,
	}
	targets := []config.Target{{Name: "prod", Host: "db", Port: 3306}, {Name: "clean", Host: "db2", Port: 3306}, {Name: "down", Host: "db3", Port: 3306}}
	s, err := New(Options{Targets: targets, Cache: cache, Now: func() time.Time { return t0.Add(3 * time.Minute) },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s, &scans, client, ts.URL, ts.Close
}

func get(t *testing.T, c *http.Client, url string, headers ...string) (int, http.Header, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

var idRE = regexp.MustCompile(`data-finding-id="([^"]*)"`)

func pageIDs(html string) []string {
	var out []string
	for _, m := range idRE.FindAllStringSubmatch(html, -1) {
		// html/template escapes attribute values; undo the common entities.
		out = append(out, strings.NewReplacer("&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'", "&amp;", "&").Replace(m[1]))
	}
	return out
}

func jsonIDs(t *testing.T, body string) []string {
	t.Helper()
	var r struct{ Findings []struct{ ID string } }
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("%v\n%s", err, body)
	}
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.ID)
	}
	return out
}

func TestOverview(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	code, _, body := get(t, c, base+"/")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{
		`href="/t/prod/findings"`, `href="/t/clean/findings"`, `href="/t/down/findings"`,
		"mariadb 11.4.13", "mysql 8.4.13", "db:3306", "3m ago",
		"anonymous (viewer)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	// prod: 1 critical, 2 warning, 0 info
	if !regexp.MustCompile(`(?s)prod</a>.*?<td class="num">1</td><td class="num">2</td><td class="num">0</td>`).MatchString(body) {
		t.Errorf("severity counts for prod not shown:\n%s", body)
	}
	if !strings.Contains(body, "cannot connect") || strings.Contains(body, "<b>refused</b>") {
		t.Errorf("error text must be shown and escaped")
	}
}

func TestFindingsFullPageAndPartialShareTemplate(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	_, hdr, full := get(t, c, base+"/t/prod/findings")
	_, hdr2, part := get(t, c, base+"/t/prod/findings", "HX-Request", "true")

	if !strings.Contains(full, "<!doctype html>") || !strings.Contains(full, "<html") || !strings.Contains(full, `src="/static/htmx.min.js"`) {
		t.Error("full page should be a complete document")
	}
	if regexp.MustCompile(`<(html|body|head)[ >]`).MatchString(part) {
		t.Errorf("HX-Request must get a fragment, got:\n%s", part)
	}
	if !strings.HasPrefix(strings.TrimSpace(part), `<section id="findings"`) {
		t.Errorf("fragment should be the #findings section, got:\n%s", part)
	}
	if !strings.Contains(full, part[strings.Index(part, `<section`):strings.Index(part, `</section>`)]) {
		t.Error("the full page must include the same fragment the partial returns")
	}
	if !reflectEqual(pageIDs(full), pageIDs(part)) || len(pageIDs(full)) != 3 {
		t.Errorf("full %v vs partial %v", pageIDs(full), pageIDs(part))
	}
	for _, h := range []http.Header{hdr, hdr2} {
		if !strings.Contains(h.Get("Vary"), "HX-Request") {
			t.Errorf("Vary should include HX-Request: %q", h.Get("Vary"))
		}
	}
	for _, want := range []string{
		`class="badge sev-critical">critical<`, "auto_increment_headroom", "shop.orders",
		"Suggested SQL (modhak does not run it)", "MODIFY `id` BIGINT NOT NULL AUTO_INCREMENT;",
		"rough estimate: about 40 days left",
		"Skipped unused_index_analysis:",
		`hx-get="/t/prod/findings"`, `hx-target="#findings"`, `hx-push-url="true"`, "<noscript>",
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full page missing %q", want)
		}
	}
}

func reflectEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFindingsEscapesHostileText(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	_, _, body := get(t, c, base+"/t/prod/findings")
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatalf("unescaped script in page:\n%s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("hostile index name should appear escaped")
	}
}

func TestFindingsFilters(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"no filter", "", []string{"auto_increment_headroom:prod:shop:orders:", "duplicate_index:prod:shop:orders:idx_dup", "redundant_index:prod:shop:orders:<script>alert(1)</script>"}},
		{"by kind", "?kind=duplicate_index", []string{"duplicate_index:prod:shop:orders:idx_dup"}},
		{"by severity", "?severity=critical", []string{"auto_increment_headroom:prod:shop:orders:"}},
		{"both", "?kind=duplicate_index&severity=critical", nil},
		{"empty values mean all", "?kind=&severity=", []string{"auto_increment_headroom:prod:shop:orders:", "duplicate_index:prod:shop:orders:idx_dup", "redundant_index:prod:shop:orders:<script>alert(1)</script>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, page := get(t, c, base+"/t/prod/findings"+tt.query)
			_, _, js := get(t, c, base+"/t/prod/findings.json"+tt.query)
			p, j := pageIDs(page), jsonIDs(t, js)
			sort.Strings(p)
			sort.Strings(j)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if !reflectEqual(p, want) {
				t.Errorf("page ids = %v, want %v", p, want)
			}
			// The page and the JSON must show the same findings.
			if !reflectEqual(p, j) {
				t.Errorf("page %v != json %v", p, j)
			}
		})
	}
	t.Run("filtered page says so", func(t *testing.T) {
		_, _, page := get(t, c, base+"/t/prod/findings?severity=critical")
		if !strings.Contains(page, "1 finding (filtered from 3)") {
			t.Errorf("count line missing:\n%s", page)
		}
		_, _, none := get(t, c, base+"/t/prod/findings?kind=duplicate_index&severity=critical")
		if !strings.Contains(none, "No findings match these filters.") {
			t.Errorf("empty filtered message missing")
		}
	})
	t.Run("selected options", func(t *testing.T) {
		_, _, page := get(t, c, base+"/t/prod/findings?kind=unused_index&severity=info")
		if !strings.Contains(page, `<option value="unused_index" selected>`) || !strings.Contains(page, `<option value="info" selected>`) {
			t.Errorf("filter selection not reflected")
		}
	})
	for _, q := range []string{"?kind=drop_table", "?severity=fatal"} {
		t.Run("rejects "+q, func(t *testing.T) {
			for _, path := range []string{"/t/prod/findings", "/t/prod/findings.json"} {
				if code, _, _ := get(t, c, base+path+q); code != http.StatusBadRequest {
					t.Errorf("%s%s = %d, want 400", path, q, code)
				}
			}
		})
	}
}

func TestJSONMatchesScanJSON(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	code, hdr, body := get(t, c, base+"/t/prod/findings.json")
	if code != 200 || !strings.HasPrefix(hdr.Get("Content-Type"), "application/json") {
		t.Fatalf("status %d type %q", code, hdr.Get("Content-Type"))
	}
	want, _ := json.MarshalIndent(results()["prod"], "", "  ")
	var a, b any
	json.Unmarshal([]byte(body), &a)
	json.Unmarshal(want, &b)
	ga, _ := json.Marshal(a)
	gb, _ := json.Marshal(b)
	if string(ga) != string(gb) {
		t.Errorf("web JSON differs from the scan JSON shape:\n%s\n%s", ga, gb)
	}
}

func TestCapabilitiesShowEnableHints(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	_, _, body := get(t, c, base+"/t/prod/findings")
	for _, want := range []string{
		"Capabilities (3 need attention)", "Enable performance_schema to get unused-index analysis.",
		`cap-unavailable`, `cap-degraded`, `cap-available`, "needs performance_schema",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestErrorTargetAndEmptyTarget(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	_, _, down := get(t, c, base+"/t/down/findings")
	if !strings.Contains(down, `role="alert"`) || !strings.Contains(down, "cannot connect") || strings.Contains(down, "<b>refused</b>") {
		t.Errorf("error target page wrong:\n%s", down)
	}
	if strings.Contains(down, `class="filters"`) {
		t.Error("no filter form for a target that failed")
	}
	_, _, part := get(t, c, base+"/t/down/findings", "HX-Request", "true")
	if !strings.Contains(part, "cannot connect") || strings.Contains(part, "<html") {
		t.Errorf("error partial wrong:\n%s", part)
	}
	_, _, clean := get(t, c, base+"/t/clean/findings")
	if !strings.Contains(clean, "0 findings") || !strings.Contains(clean, "No findings.") {
		t.Errorf("clean target page wrong:\n%s", clean)
	}
}

func TestRoutingAndMethods(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	for _, tt := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/t/nope/findings", 404},
		{"GET", "/t/nope/findings.json", 404},
		{"GET", "/nope", 404},
		{"GET", "/healthz", 200},
		{"POST", "/", 405},
		{"POST", "/t/prod/findings", 405},
		{"DELETE", "/t/prod/findings.json", 405},
	} {
		req, _ := http.NewRequest(tt.method, base+tt.path, nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.path, resp.StatusCode, tt.want)
		}
	}
}

func TestSecurityHeadersAndStatic(t *testing.T) {
	_, _, c, base, done := newServer(t)
	defer done()
	for _, path := range []string{"/", "/t/prod/findings", "/t/prod/findings.json", "/healthz", "/static/modhak.css"} {
		_, h, _ := get(t, c, base+path)
		csp := h.Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "unsafe") {
			t.Errorf("%s CSP = %q", path, csp)
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s missing security headers: %v", path, h)
		}
	}
	code, h, js := get(t, c, base+"/static/htmx.min.js")
	if code != 200 || !strings.Contains(h.Get("Content-Type"), "javascript") || len(js) < 10000 {
		t.Errorf("htmx not served: %d %q %d bytes", code, h.Get("Content-Type"), len(js))
	}
	_, _, page := get(t, c, base+"/t/prod/findings")
	for _, bad := range []string{"<style", " style=", "onclick=", "<script>", "http://", "https://"} {
		if strings.Contains(page, bad) {
			t.Errorf("page contains %q, which the CSP would block or which loads external content", bad)
		}
	}
	if !strings.Contains(page, `"allowEval":false`) || !strings.Contains(page, `"includeIndicatorStyles":false`) {
		t.Error("htmx must be configured without eval or inline styles")
	}
}

func TestRefreshRedirectsAndRespectsMinInterval(t *testing.T) {
	_, scans, c, base, done := newServer(t)
	defer done()
	get(t, c, base+"/t/prod/findings")
	if scans.Load() != 1 {
		t.Fatalf("scans = %d", scans.Load())
	}
	// Fake clock is frozen, so the result is 0s old: refresh is below MinRefresh.
	req, _ := http.NewRequest("GET", base+"/t/prod/findings?kind=duplicate_index&refresh=1", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/t/prod/findings?kind=duplicate_index" {
		t.Errorf("refresh redirect = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if scans.Load() != 1 {
		t.Errorf("refresh inside MinRefresh must not rescan, scans = %d", scans.Load())
	}
	// Overview refresh also redirects.
	req, _ = http.NewRequest("GET", base+"/?refresh=1", nil)
	resp, _ = c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Errorf("overview refresh = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestNewRejectsBrokenTemplatesNever(t *testing.T) {
	// Templates are embedded and parsed at startup; New must succeed.
	if _, err := New(Options{Cache: &scan.Cache{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Fatal(err)
	}
}

func TestShortDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Minute: "1m", 5 * time.Minute: "5m", time.Hour: "1h", 2 * time.Hour: "2h",
		90 * time.Second: "1m30s", 30 * time.Second: "30s", 90 * time.Minute: "90m",
	} {
		if got := shortDuration(d); got != want {
			t.Errorf("shortDuration(%s) = %q, want %q", d, got, want)
		}
	}
}
