# Architecture

Status: design. Code so far: M0 (config, target connections, capability detection). Catalog details are in [FEATURES.md](FEATURES.md); decisions are in [decisions/](decisions/).

## Data flow

```
 target DBs (read-only user)                       consumers
 ┌──────────────────────┐
 │ MariaDB / MySQL      │
 │  information_schema  │
 │  performance_schema  │
 │  userstat (MariaDB)  │
 └─────────┬────────────┘
           │ SELECT / SHOW only
           ▼
   ┌───────────────┐  asks first   ┌──────────────────┐
   │  capability   │◄──────────────│ collector, scan  │
   │ flavor,ver,   │               │ (per target)     │
   │ perf_schema,  │               └───────┬──────────┘
   │ userstat,sys  │                       │ snapshots
   └───────────────┘                       ▼
                                   ┌───────────────┐
                                   │store(Postgres)│  history, audit_log
                                   └───────┬───────┘
                                           │ inputs (plain structs)
                                           ▼
                                   ┌───────────────┐
                                   │   advisor     │  pure functions
                                   │ → []Finding   │
                                   └───────┬───────┘
                      ┌────────────────────┼─────────────────┐
                      ▼                    ▼                 ▼
                 internal/web         internal/api      internal/mcp
               (html/template,         (JSON)            (tools for
                htmx, uPlot)                              agents)
                      └──── every call carries a Principal (auth) ───┘
```

`modhak scan` (M1) is a shortcut: capability → one-shot collection → advisor → print, with no store.

## Package responsibilities

| Package | Responsibility | Must not |
|---|---|---|
| `cmd/modhak` | Entry point, flags, subcommands (`serve`, `scan`), wiring | Hold business logic |
| `internal/config` | Load and validate the YAML config (targets, timeouts, secrets references) | Open connections |
| `internal/target` | Per-target connection pool: read-only session, timeouts, error messages; runs capability detection per target in isolation | Hold analysis logic |
| `internal/capability` | Detect flavor, version, `performance_schema`, `userstat`, `sys`, replication statement; return `Available` / `Degraded` / `Unavailable(hint)` | Run feature queries |
| `internal/collector` | Per-target scheduled collection; reads catalogs after asking capability; computes deltas; writes to store | Judge or format findings |
| `internal/store` | Postgres access (ADR 0009), migrations, retention, audit log | Contain advisory logic |
| `internal/advisor` | Pure functions: inputs in, `[]Finding` out | Import `net/http`, `database/sql`, or the store; read clocks or env directly (pass time in) |
| `internal/web` | Routes, templates, htmx partials, embedded static | Re-implement analysis |
| `internal/auth` | `Principal`, roles, OIDC, API tokens, CSRF, audit helpers | Know about pages |
| `internal/api` | JSON endpoints over advisor and store | Add logic the web lacks |
| `internal/mcp` | MCP tools over the same data | Same |

Templates and static assets live under the web package and are embedded with `go:embed`.

## Findings layer contract

```go
// Sketch; illustrative, not final.
type Finding struct {
    Kind         string   // e.g. "duplicate_index", "unused_index", "redundant_index",
                          // "auto_increment_headroom", "missing_index"
    Severity     Severity // info | warning | critical
    Target       Target   // target name, schema, table, optionally index/column/digest
    Evidence     Evidence // structured, kind-specific; includes observation window where relevant
    SuggestedSQL []string // text only; never executed by modhak
    // Stable ID derived from Kind+Target for dedupe and links (TBD).
}
```

Rules
- Advisor functions are pure: `func(Input, Config, Principal?) []Finding`. Whether the Principal is an argument of the advisor or checked by the caller is an open question (see below). The decision requires every consumer to take a Principal.
- Evidence is structured data, not prose. UI text is built from it by the consumer.
- Findings from an unavailable capability are not produced; the consumer shows the capability's hint instead.
- Unused-index findings must carry uptime / observation window and the "observed for N days" caveat (ADR 0007).
- Output is deterministic for the same input (sorted), so tests and agents can diff results.

## Web layer

Server-rendered with `html/template`, htmx, and uPlot (ADR 0001).

Routes (planned; all GET unless noted)

| Route | Purpose | Milestone |
|---|---|---|
| `/` | Targets overview | M0/M1 |
| `/t/{target}/findings` | Findings list, filter by kind/severity | M1 |
| `/t/{target}/queries` | Digest deltas, charts | M2 |
| `/t/{target}/live` | Live queries; `POST .../kill` (opt-in) | M3/M4 |
| `/t/{target}/space` | Size and growth | M3 |
| `/t/{target}/health` | Connections, buffer pool, history length | M3 |
| `/t/{target}/replication` | Lag | M3 |
| `/t/{target}/explain` | Explain, SELECT-only (POST) | M5 |
| `/api/v1/...` | JSON API | M1 (CLI JSON), M4 |
| `/mcp` | MCP endpoint (transport TBD) | M4 |

Partial vs full page rule
- A request with the `HX-Request: true` header gets a partial (the fragment for the target element). Any other request gets the full page. Both come from the same template; the full page includes the fragment.
- A fragment is never an independent page: direct URL access must work with a full render.
- Mutating actions are POST with CSRF (ADR 0006); the response is a partial plus an `HX-Trigger` where needed.
- Navigation uses normal links (optionally `hx-boost`); filters use `hx-get` on the same URL.

Charts
- uPlot, embedded; data embedded in the page as JSON in a `<script type="application/json">` block or fetched from a JSON endpoint on the same route family. No chart state on the server. No Node build.

## Store schema sketch

PostgreSQL (ADR 0009); illustrative columns, refine in M2. Times are `timestamptz` (UTC). All history tables carry `target_id`.

| Table | Core columns | Notes |
|---|---|---|
| `targets` (optional) | `id`, `name` | May be implicit from config; TBD |
| `digest_text` | `target_id`, `schema_name`, `digest`, `text`, `first_seen_at` | One row per digest; normalized and truncated |
| `digest_snapshots` | `id`, `target_id`, `taken_at`, `schema_name`, `digest`, `count_star`, `sum_timer_wait`, plus examined/sent/no-index counters if verified, `reset_epoch` | Capped per snapshot; deltas computed against previous row |
| `index_usage` | `target_id`, `taken_at`, `schema_name`, `table_name`, `index_name`, `read_count` or `rows_read`, `source` (`userstat` / `perf_schema`) | Source recorded because semantics differ |
| `space_snapshots` | `target_id`, `taken_at`, `schema_name`, `table_name`, `data_length`, `index_length`, `table_rows`, `auto_increment` | Basis for growth and days-to-overflow |
| `health_snapshots` | `target_id`, `taken_at`, `threads_connected`, `max_connections`, buffer pool counters, `history_list_length`, `replication_lag_seconds` | Exact fields follow what is verified |
| `audit_log` | `id`, `at`, `principal`, `role`, `action`, `target_id`, `detail`, `outcome`, `remote_addr` | Append-only; kill and auth events |
| `server_state` | `target_id`, `taken_at`, `uptime`, `flavor`, `version` | Needed to detect restarts; may merge into `health_snapshots` |

Also (TBD): `schema_migrations`, `api_tokens` (hashed), and `capability_snapshots`.

## Auth design

- Default off: requests get an implicit Principal; its role is an open question (see 4 below). Kill stays disabled unless configured (ADR 0007).
- Later (M4): OIDC via go-oidc and oauth2 with PKCE; group-to-role mapping to `viewer` / `operator`; API tokens for agents and MCP with a role and an optional target scope; CSRF on all POSTs; audit log for destructive actions (kill).
- Every web, API, and MCP entry builds a `Principal` and passes it down. Authorization checks live in a small number of choke points (middleware and a `Can(principal, action, target)` helper), not scattered in handlers.
- Session cookie details, token hashing scheme, and token scoping are TBD.

## Open questions

1. Is the `Principal` an argument to advisor functions, or is authorization enforced only at the consumer? Decision 5 says all consumers take one; the advisor signature is undecided. Leaning: consumers filter by target access; advisor stays pure.
2. Which parser: Vitess or TiDB? Evaluate MariaDB-syntax coverage, binary size, and API stability before M5.
3. MCP transport (stdio, streamable HTTP, both) and which SDK. TBD.
4. Default behavior when auth is off: which role does the implicit Principal have, and is kill allowed if enabled in config?
5. Minimum uptime default for unused-index verdicts. M1 uses 7 days, configurable (`advisor.Config.MinUptime`); revisit with real data.
6. Supported version matrix (lowest MariaDB and MySQL versions). TBD until the FEATURES.md checklist is run.
7. Whether `modhak scan` should be able to write to the store, or stay stateless.
8. Is the `targets` table needed or does config alone identify targets (renames break history)?
9. Digest snapshot cap and retention defaults.
10. How to treat forks and managed services (Percona, RDS, Aurora, Cloud SQL).
11. Secrets handling in config (env expansion, file references).
12. License: MIT is present in LICENSE; confirm it is the intended final license.
13. Whether scanning a replica is the recommended default, given replicas can see different index usage than the primary.
