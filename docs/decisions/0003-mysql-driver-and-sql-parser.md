# 0003: go-sql-driver/mysql; Vitess or TiDB parser behind an interface

Status: accepted (parser choice open)

## Context
modhak must talk to both MariaDB and MySQL, and for missing-index analysis it must parse normalized digest text.

## Decision
- Use `go-sql-driver/mysql` for target connections.
- Parse SQL through a modhak-owned interface (for example `Parse(text) (QueryShape, error)` returning tables, predicates, joins, ordering). The implementation uses the Vitess or TiDB parser; which one is decided before M5.

## Consequences
- Both candidate parsers target MySQL grammar; MariaDB-specific syntax may fail (RISKS.md). Failure must surface as "could not analyze".
- The interface keeps parser dependencies out of `internal/advisor` types and allows swapping or adding a fallback.
- These parsers are large dependencies; binary size impact is TBD.
- Digest text contains placeholders and may be truncated, so the interface has to tolerate partial input.
