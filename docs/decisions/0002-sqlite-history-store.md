# 0002: SQLite history store via modernc.org/sqlite

Status: superseded by [0009](0009-postgres-history-store.md)

## Context
modhak keeps snapshots (digests, index usage, space, health) and an audit log. It must not require another server, and it must stay a CGO-free static binary.

## Decision
Store history in SQLite using `modernc.org/sqlite`, a pure-Go driver.

## Consequences
- No external dependency and trivial backup (one file); cross-compilation stays easy.
- Pure-Go SQLite is slower than the C library and has a larger binary; acceptable at snapshot volumes, to be measured in M2.
- Single-writer model: the collector serializes writes. Concurrency settings (WAL etc.) TBD.
- Retention and snapshot caps are required to bound file size (RISKS.md).
- Not suited to multi-node deployments; the "standalone collector agent" idea (Later) would need a different transport, not a shared file.
