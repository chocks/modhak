# modhak

An index and schema advisor with a dashboard for MariaDB (first-class) and MySQL (supported). Inspired by [PgHero](https://github.com/ankane/pghero).

> **Status: early development.** Capability detection (`modhak capabilities`) and index/AUTO_INCREMENT scanning (`modhak scan`) work against MariaDB and MySQL. The web UI, history store, JSON API and MCP are *planned*; sections marked planned are not available yet.

*Name:* a modak is a steamed dumpling ("modak" is the common spelling; "modhak" is this project's spelling). It is a sibling to my existing tool, dumpling.

## Why

MariaDB and MySQL lack a simple, free, self-hosted tool that tells you what to fix: duplicate and unused indexes, missing indexes, AUTO_INCREMENT headroom. modhak collects snapshots from a target database, keeps history in Postgres (planned), and produces actionable findings (with evidence and suggested SQL) in a web UI, a JSON API, and MCP so AI agents can act on them.

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
| Duplicate / redundant / unused index findings with DDL | implemented (M1): `modhak scan`, web findings page |
| AUTO_INCREMENT headroom | implemented (M1): `modhak scan`, web findings page |
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
modhak serve --config configs/modhak.local.yaml [--listen 127.0.0.1:8080] [--cache-ttl 1m]   # or: make serve
```

`scan` prints duplicate, redundant and unused index findings and AUTO_INCREMENT headroom for each target, with evidence and suggested SQL that modhak never runs. It exits 1 if any target could not be scanned and 0 otherwise, even when it finds problems. Unused-index verdicts need `performance_schema` (userstat alone is not enough) and a server uptime of at least `--min-uptime` (default 7 days; `0` turns the minimum off); otherwise `scan` says why it skipped them.

`serve` runs a small web UI over the same findings: a targets overview at `/`, and `/t/NAME/findings` per target with kind and severity filters (htmx; it also works without JavaScript). `/t/NAME/findings.json` returns the same JSON as `scan --json` for that target. Pages show each target's capabilities with the "enable X to get Y" hints, the evidence and caveats for each finding, and the suggested SQL, which modhak never runs.

Things to know about `serve` in this milestone:
- **There is no authentication yet** (M4). It binds to `127.0.0.1` by default and warns if you bind elsewhere. Do not expose it: anyone who can reach it can read findings and schema names. Every request is treated as an implicit read-only `viewer`.
- **There is no history store yet** (M2). Each target is scanned on demand and the result is cached in memory for `--cache-ttl` (default 1 minute) so page views do not hit your database every time. "Rescan" forces a new scan, at most once every 10 seconds per target.
- The UI and its htmx script are embedded in the binary; it loads nothing from the internet, and the content security policy allows only its own origin.

## Minimal GRANTs

Use a dedicated read-only user, ideally on a replica.

**Verified** (capability detection and `modhak scan`, on MariaDB 10.6 and 11.4 and MySQL 8.0 and 8.4):

```sql
CREATE USER 'modhak'@'%' IDENTIFIED BY '...';
GRANT SELECT, PROCESS ON *.* TO 'modhak'@'%';
```

Narrower grants (for example `SELECT` on `performance_schema` and on your schemas only) are **UNVERIFIED**: see the per-feature table and checklist in [docs/FEATURES.md](docs/FEATURES.md#4-minimal-grants-per-feature). Grants for later features are also unverified: replication status (MariaDB: `REPLICATION SLAVE ADMIN` 10.5.2+ or `REPLICA MONITOR` 10.5.9+, per docs; MySQL: unverified), EXPLAIN (M5) and opt-in kill-query.

modhak never needs write privileges and never runs DDL. It sends only `SELECT` and `SHOW`, plus two session-local `SET` statements (a read-only transaction, and on MySQL turning off the `information_schema` statistics cache so `AUTO_INCREMENT` is current).

## Supported versions

| Server | Version | Status |
|---|---|---|
| MariaDB | tested: 10.6, 11.4 | first-class target |
| MySQL | tested: 8.0, 8.4 | supported |

"Tested" means the integration tests and `modhak scan` were run against those versions (docker compose and CI cover 11.4 and 8.4; 10.6 and 8.0 are in the `matrix` profile). The lowest supported versions are TBD until the verification checklist in [docs/FEATURES.md](docs/FEATURES.md#unverified-checklist) is finished. Forks and managed services (Percona, RDS, Aurora, Cloud SQL) are untested.

## Configuration

Today only `targets` is read. The other keys below are the planned shape (not final, not implemented).

```yaml
listen: 127.0.0.1:8080      # planned
store:                      # planned: Postgres history store (ADR 0009); `scan` needs none
  url_env: MODHAK_STORE_URL
auth:                       # planned
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
features:                   # planned
  kill_query: false         # opt-in; needs operator role and a privileged user
```

An example that works today is [configs/modhak.example.yaml](configs/modhak.example.yaml).

## Local development

Needs Go (1.27 is pinned in `go.mod`; an older local `go` fetches it automatically) and Docker with Compose.

```sh
make up                      # MariaDB 11.4 (:33061) and MySQL 8.4 (:33062), read-only `modhak` user
make check                   # gofmt, vet, tests
make build                   # bin/modhak
docker compose --profile bare up -d --wait     # servers with performance_schema off (:33063, :33064)
docker compose --profile matrix up -d --wait   # MariaDB 10.6 (:33065), MySQL 8.0 (:33066)
make run ARGS="scan --config configs/modhak.local.yaml"    # or: capabilities
MODHAK_TEST_ADDR=127.0.0.1:33061 go test ./...             # also run the live-server tests (use :33062 for MySQL)
make down                    # stop everything; `make reset` also drops data
```

CI (`.github/workflows/ci.yml`) runs gofmt, vet, tests and build, then the live-server tests against the compose MariaDB and MySQL. The `app` profile (`docker compose --profile app up --build`) runs modhak itself in a container. Postgres for the history store (ADR 0009) joins the compose file when M2 starts.

`make up` copies `configs/modhak.example.yaml` to the git-ignored `configs/modhak.local.yaml`. Dev credentials (`modhak`/`modhak`, root `root`) are for local containers only.

## Roadmap

M0 capability detection and connections (done); M1 index hygiene and AUTO_INCREMENT headroom with `modhak scan` (CLI done, findings page next); M2 collector, Postgres store, queries page; M3 live queries, space, health, replication; M4 auth, API tokens, JSON API, MCP; M5 suggested indexes, explain verification, schema lint. Later: Datadog events export, OIDC polish, standalone collector agent. Scope and exit criteria: [docs/ROADMAP.md](docs/ROADMAP.md).

## Docs

[Architecture](docs/ARCHITECTURE.md) · [Features](docs/FEATURES.md) · [Roadmap](docs/ROADMAP.md) · [Risks](docs/RISKS.md) · [Decisions](docs/decisions/) · [AGENTS.md](AGENTS.md)

## License

MIT, see [LICENSE](LICENSE). (Placeholder until confirmed final.)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
