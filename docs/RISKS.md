# Risks and traps

Version numbers and catalog names are intentionally sparse; see [FEATURES.md](FEATURES.md) for what is verified.

## MariaDB vs MySQL catalog divergence
The two flavors differ in catalogs, statements, and variables (for example `userstat` is MariaDB-specific, `data_locks` is MySQL 8.0; see FEATURES.md). Hard-coding one flavor's queries breaks the other.
Mitigation: the capability layer (ADR 0004) and the capability matrix; features ask before querying; CI against both flavors (future). Treat anything unverified as unavailable until tested.

## performance_schema often off on MariaDB
Digest, index I/O, and sys-schema features depend on it ([sys needs it](https://www.mariadb.com/docs/server/reference/sql-statements/administrative-sql-statements/system-tables/sys-schema)). Default state per flavor is UNVERIFIED.
Mitigation: detect via the capability layer and show "enable X to get Y". Enabling usually needs a restart (UNVERIFIED) and adds memory and CPU overhead whose size is not quantified here (TBD; measure before documenting numbers). modhak never changes server settings itself.

## Digest snapshot size on busy servers
The digest table can hold many rows, and the statement text is long. Overflow goes into a NULL schema/digest row ([source](https://dev.mysql.com/doc/refman/8.4/en/performance-schema-statement-digests.html)).
Mitigation: cap rows per snapshot (top N by total time plus a tail), store `digest_text` once per digest, normalize and truncate text, prune by retention. Report when the server's own table is overflowing, since that skews data.

## Counter resets on restart
Digest, index I/O, and status counters reset on restart, `TRUNCATE`, and (for index counters) possibly DDL ([source](https://mariadb.com/kb/en/3991/)).
Mitigation: a count lower than the previous snapshot starts a new baseline; record the reset in the store; never show negative rates; show the observation window.

## Unused-index verdicts need enough uptime
Zero reads over a short window prove nothing: monthly jobs, failover replicas, and recently deployed queries all hide.
Mitigation: require a minimum uptime/observation window before any "unused" verdict; always print "observed for N days"; never recommend dropping unique/PK indexes; run on a replica only with caution, since the replica may not serve the workload that uses the index. Drops stay recommendations (ADR 0007).

## Parser accuracy on MariaDB-specific syntax
Vitess/TiDB parsers target MySQL grammar. MariaDB-specific syntax (sequences, `RETURNING`, other extensions: list UNVERIFIED) may fail or parse wrongly. Digest text also has placeholders (`?`) and may be truncated.
Mitigation: parser behind an interface (ADR 0003); on parse failure emit "could not analyze", never guess; a regression corpus of MariaDB statements; EXPLAIN verification before any suggestion is shown as verified.

## Privilege model
Wrong or excessive grants either break features silently (partial `information_schema` results) or widen blast radius.
Mitigation: minimal GRANTs per feature, documented in the README and verified per FEATURES.md; the capability layer detects missing privileges and says which grant is missing; separate, optional user for kill-query.

## Other
- **Cloud/fork variants** (managed MySQL/MariaDB, Percona): version strings and privileges may differ. TBD.
- **EXPLAIN executes work**: `EXPLAIN ANALYZE` / `ANALYZE` run the statement. Explain stays SELECT-only and execution is opt-in.
- **Replica vs primary**: scans on a replica miss write-path index use on the primary and vice versa. State which target was scanned.
- **Monitoring overhead**: queries against `information_schema` can be slow on servers with very many tables. Add timeouts and per-feature intervals.
- **Credentials at rest**: the config file holds DB passwords. Support env var references; document file permissions. TBD.
