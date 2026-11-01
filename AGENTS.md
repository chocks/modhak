# AGENTS.md

Guidance for coding agents working in this repository.

## Project overview
modhak is a planned advisor and dashboard for MariaDB (first-class) and MySQL. It snapshots a target database, stores history in SQLite, and produces findings (with evidence and suggested SQL) for a web UI, JSON API, and MCP. Current state: **documentation only**; no Go code yet. Read `docs/ARCHITECTURE.md`, `docs/FEATURES.md`, and `docs/decisions/` before changing anything.

## Layout (planned)
```
cmd/modhak/            entry point
internal/capability/   flavor/version/feature detection
internal/collector/    scheduled snapshots, deltas
internal/store/        SQLite history, audit log
internal/advisor/      pure findings functions
internal/web/          handlers, templates, static (go:embed)
internal/auth/         Principal, roles, OIDC, tokens, CSRF
internal/api/          JSON API
internal/mcp/          MCP tools
docs/                  architecture, features, roadmap, risks, decisions (ADRs)
```

## Conventions
- **Advisor is pure.** `internal/advisor` takes plain structs and returns `[]Finding`. No HTTP, no `database/sql`, no store access, no clock or env reads (pass time in).
- **Capability checks before catalog queries.** Any code that reads `information_schema`, `performance_schema`, userstat tables, or runs `SHOW ... STATUS` must first ask `internal/capability`, and handle `Degraded` / `Unavailable` with an "enable X to get Y" message.
- **Never execute DDL.** modhak only suggests SQL. No `ALTER`, `DROP`, `CREATE` against targets, ever. Only `SELECT`, `SHOW`, and (opt-in, audited) `KILL QUERY`. Explain accepts a single SELECT only.
- **Web, API, MCP are thin.** No analysis logic in handlers; all take a `Principal`.
- **htmx rule:** `HX-Request` gets a partial, otherwise a full page, from the same template.
- **No Node toolchain, no CGO.** SQLite is `modernc.org/sqlite`.
- **Don't invent catalog facts.** Every table, column, variable, statement, and version in code or docs must be in `docs/FEATURES.md` as verified, or marked UNVERIFIED there. If you verify something, update the table and tick the checklist.
- **Tests:** table-driven. Advisor tests need no database. Delta logic needs reset cases (count drops mean a new baseline). Future: integration tests against both MariaDB and MySQL via docker compose; until then, mark tests that assume one flavor.
- **Decisions:** a change that contradicts an ADR needs a new ADR that supersedes it, not a silent edit.
- Keep docs concise; mark uncertainty as TBD or Open question.

## Do not (this phase)
Do not add Go code beyond the current milestone's scope until a milestone asks for it. CI lives in `.github/workflows/ci.yml`.
