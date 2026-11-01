# modhak

An index and schema advisor with a dashboard for MariaDB (first-class) and MySQL (supported). Inspired by [PgHero](https://github.com/ankane/pghero).

> **Status: planning.** No code exists yet. Everything below describes the intended design. Sections marked *planned* or *not yet implemented* are not available.

*Name:* a modak is a steamed dumpling ("modak" is the common spelling; "modhak" is this project's spelling). It is a sibling to my existing tool, dumpling.

## Why

MariaDB and MySQL lack a simple, free, self-hosted tool that tells you what to fix: duplicate and unused indexes, missing indexes, AUTO_INCREMENT headroom. modhak collects snapshots from a target database, keeps history in SQLite, and produces actionable findings (with evidence and suggested SQL) in a web UI, a JSON API, and MCP so AI agents can act on them.

## Positioning vs Datadog DBM

modhak is not a DBM replacement. DBM monitors; modhak advises.

- DBM: continuous monitoring, alerting, fleet-wide dashboards, hosted.
- modhak: duplicate/unused/redundant indexes with ready-to-run DDL, missing-index suggestions tied to specific query digests, AUTO_INCREMENT headroom, schema lint, before/after EXPLAIN verification. Free, self-hosted, works against any reachable MariaDB/MySQL with a read-only user. Every finding is structured data with evidence.

If you need alerting, long retention, or APM correlation, use a monitoring product. modhak can sit beside it.

## Features

Nothing is implemented yet. All of the following is **planned**; the milestone is in parentheses. Details and sources: [docs/FEATURES.md](docs/FEATURES.md).

| Area | Status |
|---|---|
| Capability detection (flavor, version, performance_schema, userstat, sys) | planned (M0) |
| Duplicate / redundant / unused index findings with DDL | planned (M1) |
| AUTO_INCREMENT headroom | planned (M1) |
| `modhak scan` CLI with JSON output | planned (M1) |
| Query digests with history and deltas | planned (M2) |
| Live queries, space growth, health, replication | planned (M3) |
| Auth (OIDC, API tokens), JSON API, MCP, opt-in kill | planned (M4) |
| Suggested indexes, EXPLAIN verification, schema lint | planned (M5) |

## Quick start

**Not yet implemented.** Intended shape:

```sh
modhak scan --config modhak.yaml          # one-shot findings, text or JSON
modhak serve --config modhak.yaml         # web UI + collector
```

## Minimal GRANTs

**Proposed and UNVERIFIED** except where noted; see the per-feature table and checklist in [docs/FEATURES.md](docs/FEATURES.md#4-minimal-grants-per-feature). Use a dedicated read-only user, ideally on a replica.

```sql
-- Proposal, to be tested against real MariaDB and MySQL instances.
CREATE USER 'modhak'@'%' IDENTIFIED BY '...';
GRANT SELECT ON performance_schema.* TO 'modhak'@'%';   -- digests, index I/O
GRANT PROCESS ON *.* TO 'modhak'@'%';                   -- see all threads
-- Replication status (MariaDB, per docs): REPLICATION SLAVE ADMIN (10.5.2+) or REPLICA MONITOR (10.5.9+).
-- Replication status (MySQL): UNVERIFIED.
-- Index/column metadata for your schemas: SELECT on those schemas (UNVERIFIED if needed).
-- EXPLAIN (M5): SELECT on the tables being explained.
-- Kill-query (opt-in, separate user recommended): privilege UNVERIFIED.
```

modhak never needs write privileges and never runs DDL.

## Supported versions

| Server | Version | Status |
|---|---|---|
| MariaDB | TBD | first-class target |
| MySQL | TBD | supported |

The lowest supported versions will be set after the verification checklist in [docs/FEATURES.md](docs/FEATURES.md#unverified-checklist) is run. Facts known so far: MariaDB `sys` schema starts at 10.6.0; `SHOW REPLICA STATUS` exists in MariaDB from 10.5.1.

## Configuration

Planned shape (not final, not implemented):

```yaml
listen: 127.0.0.1:8080
store:
  path: ./modhak.db
auth:
  enabled: false            # off by default
targets:
  - name: prod-replica-1
    dsn: modhak:${MODHAK_PASSWORD}@tcp(db1.internal:3306)/
    flavor: auto            # auto | mariadb | mysql
  - name: staging
    dsn: modhak:${MODHAK_PASSWORD}@tcp(staging-db:3306)/
features:
  kill_query: false         # opt-in; needs operator role and a privileged user
```

## Roadmap

M0 capability detection and connections; M1 index hygiene and AUTO_INCREMENT headroom with `modhak scan`; M2 collector, store, queries page; M3 live queries, space, health, replication; M4 auth, API tokens, JSON API, MCP; M5 suggested indexes, explain verification, schema lint. Later: Datadog events export, OIDC polish, standalone collector agent. Scope and exit criteria: [docs/ROADMAP.md](docs/ROADMAP.md).

## Docs

[Architecture](docs/ARCHITECTURE.md) · [Features](docs/FEATURES.md) · [Roadmap](docs/ROADMAP.md) · [Risks](docs/RISKS.md) · [Decisions](docs/decisions/) · [AGENTS.md](AGENTS.md)

## License

MIT, see [LICENSE](LICENSE). (Placeholder until confirmed final.)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
