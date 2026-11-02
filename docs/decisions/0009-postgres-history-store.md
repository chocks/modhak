# 0009: PostgreSQL history store

Status: accepted. Supersedes [0002](0002-sqlite-history-store.md).

## Context
modhak keeps snapshots (digests, index usage, space, health) and an audit log. ADR 0002 chose an embedded SQLite file so modhak would need no other server. The store has not been written yet (it arrives in M2), so switching now costs only documentation. The project owner decided to use PostgreSQL for modhak's own database instead.

This is unrelated to the targets: modhak still analyzes MariaDB (first-class) and MySQL only.

## Decision
- Store history and the audit log in PostgreSQL.
- Connect with `jackc/pgx/v5`, a pure-Go driver, so the "no CGO" rule still holds. (Proposed; confirm when M2 starts.)
- Schema changes go through versioned migrations in `internal/store`. The migration tool is TBD.
- `modhak scan` stays stateless and needs no Postgres. `modhak serve` and the collector need a reachable Postgres, configured under `store:` in the config file. How the connection URL and its password are supplied is TBD (same open question as target secrets, ARCHITECTURE.md #11).
- Times are stored as `timestamptz` (UTC).

## Consequences
- modhak is no longer a single self-contained binary for the server mode: running it needs a Postgres instance. The README and local development setup must say so, and docker compose gains a Postgres service in M2.
- Several modhak processes, or a future standalone collector agent (Later), can share one store, which the single-file SQLite model could not do. Writers are no longer serialized by file locking; the collector's write path is designed for concurrent use.
- Backup, access control, and retention for the history are the operator's Postgres practices. Retention and per-snapshot caps are still required to bound size (RISKS.md).
- The audit log lives in this database. Whether it needs stronger protection than the history tables (separate role, append-only grants) is TBD for M4.
- Larger tables may call for partitioning by time; measure in M2 before adding it.
- Tests that touch the store need a Postgres instance; integration tests run against docker compose, like the target-database tests.
