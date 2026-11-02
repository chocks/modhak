# Roadmap

M0 and M1 are done; M2 is next. Catalog and version claims are subject to the checklist in [FEATURES.md](FEATURES.md#unverified-checklist).

## M0: Capability detection and target connection manager

Status: **done.** Verified against MariaDB 10.6 and 11.4 and MySQL 8.0 and 8.4 (FEATURES.md V10).

Scope
- Config file loading with multiple targets (ADR 0008).
- Connection manager using the MySQL driver (ADR 0003), with timeouts and a read-only session.
- `internal/capability`: flavor, version, `performance_schema`, `userstat`, `sys` schema, replication statement choice.
- A way to print the detected capabilities for a target.

Exit criteria
- Against one MariaDB and one MySQL instance, detection output matches a manually checked result.
- A target with `performance_schema` off reports `Unavailable` with an "enable X to get Y" hint, not an error.
- Unreachable or unauthorized targets fail with a clear message and do not affect other targets.
- Table-driven tests cover version-string parsing, including fork strings.

## M1: Index hygiene and AUTO_INCREMENT headroom (first demoable value)

Status: **done.** `modhak scan` (text and JSON) and `modhak serve` (findings page and JSON route). Notes on how the exit criteria were met:
- Verified on docker compose servers (MariaDB 10.6/11.4, MySQL 8.0/8.4), **not yet on a real production replica**; run it against one before relying on it.
- The default minimum uptime for unused-index verdicts is 7 days (`--min-uptime`).
- Unused-index verdicts come from `performance_schema` only, and only while the table I/O instrument and `global_instrumentation` consumer are on. MariaDB `userstat` does not qualify: it lists only indexes that were read (FEATURES.md V12).
- The server's own query log during a scan shows only SELECT, SHOW and the session-local `SET` statements.
- The web UI has no auth (M4) and no history (M2): results are scanned on demand and cached briefly in memory.

Scope
- `internal/advisor`: duplicate, redundant, and unused index findings; AUTO_INCREMENT headroom finding.
- Findings page (server-rendered), JSON output, and CLI `modhak scan`.
- Unused-index verdicts include uptime and a caveat; no verdict below a configurable minimum uptime (default 7 days).
- Suggested DDL is text only (ADR 0007).

Goal: point at a production replica and get real findings in one run.

Exit criteria
- `modhak scan` on a real replica (MariaDB and MySQL) finishes in one command and prints findings in text and JSON.
- Every finding has kind, severity, target, evidence, and suggested SQL, per the contract in ARCHITECTURE.md.
- Advisor functions are pure and tested with table-driven tests, with no DB access.
- No statement beyond SELECT/SHOW is issued; verified by test or query log on a read-only user.
- Findings page renders the same findings as the JSON.
- With `userstat` and `performance_schema` both unavailable, unused-index analysis reports as unavailable rather than guessing.

## M2: Collector, store, delta logic, queries page

Scope
- `internal/collector` on an interval, writing to Postgres via `internal/store` (ADR 0009). docker compose gets a Postgres service.
- Digest snapshots with row cap and text normalization; `digest_text` deduplicated.
- Delta computation with counter-reset handling (a count drop is a new baseline).
- Queries page with top-N and time range; uPlot charts.
- Retention and pruning.

Exit criteria
- Restarting the target or truncating the digest table does not produce negative or inflated deltas (tested).
- Snapshot size on a busy server stays under the configured cap.
- Store survives collector restarts; schema migrations are versioned.
- Index usage history lets unused-index findings report an observation window longer than current uptime where counters allow it.

## M3: Live queries, space, health, replication pages

Scope
- Live queries (read-only view), connections vs `max_connections`, space and growth, health (buffer pool hit ratio, long transactions, history list length), replication lag.
- AUTO_INCREMENT days-to-overflow using history.

Exit criteria
- Each page degrades with an explicit message when its capability is missing.
- Replication page works on MariaDB and MySQL, using the statement chosen by the capability layer.
- Space page shows growth over at least two snapshots.

## M4: Auth middleware, API tokens, JSON API, MCP

Scope
- `internal/auth`: Principal, roles (viewer/operator), API tokens, optional OIDC (ADR 0006), CSRF on POSTs.
- Kill-query (opt-in, operator-only, audited).
- `internal/api` and `internal/mcp` as thin consumers of the advisor.

Exit criteria
- With auth off (default), behavior equals M3. With auth on, every route and tool checks the Principal.
- Kill is impossible unless enabled in config and the caller has the operator role; each kill appears in `audit_log`.
- An MCP client can list targets, list findings, and read evidence.
- Token storage stores hashes only (design detail TBD).

## M5: Suggested indexes, explain verification, schema lint

Scope
- Digest parsing behind the parser interface (ADR 0003); missing-index findings tied to digests.
- Explain page: SELECT-only, user-supplied parameters; before/after comparison.
- Schema lint: missing primary keys, mixed collations, oversized VARCHAR indexes.

Exit criteria
- Parser failures on a digest produce "could not analyze" and never a wrong suggestion; a MariaDB-specific syntax corpus is part of the tests.
- Non-SELECT input to the explain feature is rejected.
- Execution-based explain (`ANALYZE`) requires an explicit opt-in.
- Each suggestion links to the digest and shows plan evidence.

## Later

- Datadog events export.
- OIDC polish (group mapping UX, session handling).
- Standalone collector agent mode (collector near the DB, reports to a central modhak).
