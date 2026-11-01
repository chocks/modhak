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

Early development. Rows marked planned are not implemented; the milestone is in parentheses. Details and sources: [docs/FEATURES.md](docs/FEATURES.md).

| Area | Status |
|---|---|
| Capability detection (flavor, version, performance_schema, userstat, sys) | implemented (M0): `modhak capabilities` |
| Duplicate / redundant / unused index findings with DDL | CLI done (M1): `modhak scan`; findings page next |
| AUTO_INCREMENT headroom | CLI done (M1): `modhak scan` |
| `modhak scan` CLI with JSON output | implemented (M1) |
| Query digests with history and deltas | planned (M2) |
| Live queries, space growth, health, replication | planned (M3) |
| Auth (OIDC, API tokens), JSON API, MCP, opt-in kill | planned (M4) |
| Suggested indexes, EXPLAIN verification, schema lint | planned (M5) |

## Quick start

Implemented so far:

```sh
modhak capabilities --config configs/modhak.local.yaml [--target NAME] [--json]
modhak scan --config configs/modhak.local.yaml [--target NAME] [--json] [--min-uptime 168h]
```

`scan` prints duplicate, redundant and unused index findings and AUTO_INCREMENT headroom for each target, with evidence and suggested SQL that modhak never runs. It exits 1 if any target could not be scanned and 0 otherwise, even when it finds problems. Unused-index verdicts need `performance_schema` (userstat alone is not enough) and a server uptime of at least `--min-uptime` (default 7 days; `0` turns the minimum off); otherwise `scan` says why it skipped them.

Planned:

```sh
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
    host: db1.internal
    port: 3306              # default
    user: modhak
    password_env: MODHAK_PASSWORD   # or `password:`; set only one
    tls: preferred          # false | true | skip-verify | preferred
  - name: staging
    host: staging-db
    user: modhak
    password_env: MODHAK_PASSWORD
features:
  kill_query: false         # opt-in; needs operator role and a privileged user
```

## Local development

Needs Go (1.27 is pinned in `go.mod`; an older local `go` fetches it automatically) and Docker with Compose.

```sh
make up                      # MariaDB 11.4 (:33061) and MySQL 8.4 (:33062), read-only `modhak` user
make check                   # gofmt, vet, tests
make build                   # bin/modhak
docker compose --profile bare up -d --wait     # servers with performance_schema off (:33063, :33064)
docker compose --profile matrix up -d --wait   # MariaDB 10.6 (:33065), MySQL 8.0 (:33066)
make run ARGS="capabilities --config configs/modhak.local.yaml"
MODHAK_TEST_ADDR=127.0.0.1:33061 go test ./internal/target -v   # live-server test, skipped otherwise
make down                    # stop everything; `make reset` also drops data
```

`make up` copies `configs/modhak.example.yaml` to the git-ignored `configs/modhak.local.yaml`. Dev credentials (`modhak`/`modhak`, root `root`) are for local containers only.

## Roadmap

M0 capability detection and connections; M1 index hygiene and AUTO_INCREMENT headroom with `modhak scan`; M2 collector, store, queries page; M3 live queries, space, health, replication; M4 auth, API tokens, JSON API, MCP; M5 suggested indexes, explain verification, schema lint. Later: Datadog events export, OIDC polish, standalone collector agent. Scope and exit criteria: [docs/ROADMAP.md](docs/ROADMAP.md).

## Docs

[Architecture](docs/ARCHITECTURE.md) · [Features](docs/FEATURES.md) · [Roadmap](docs/ROADMAP.md) · [Risks](docs/RISKS.md) · [Decisions](docs/decisions/) · [AGENTS.md](AGENTS.md)

## License

MIT, see [LICENSE](LICENSE). (Placeholder until confirmed final.)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
