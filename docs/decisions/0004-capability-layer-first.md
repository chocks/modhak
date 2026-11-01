# 0004: Capability layer first

Status: accepted

## Context
MariaDB and MySQL differ in catalogs, statements, and variables, and `performance_schema`, `userstat`, and the `sys` schema may each be absent or disabled. See the matrix in [FEATURES.md](../FEATURES.md).

## Decision
`internal/capability` is built first (M0). It detects flavor, version, and whether `performance_schema`, `userstat`, and the `sys` schema are available, and chooses per-flavor statements (such as the replication status statement). Every feature asks it before running any catalog query and degrades gracefully with a message of the form "enable X to get Y".

## Consequences
- No feature queries a catalog on assumption; an unverified capability counts as unavailable.
- Adding a flavor or version means updating the matrix and tests, not the features.
- Detection costs a few queries per target, cached and refreshed on reconnect or restart.
- Capability results are also data: shown in the UI and API so users see why a feature is off.
