# Features and capability matrix

Status: planning. Nothing here is implemented.

## How to read the verification marks

Every catalog table, variable, or statement below carries a source link or the marker **UNVERIFIED**.

- **Linked source** means a search result for that official MariaDB/MySQL documentation page confirmed the claim. In the session that wrote this file, direct fetches of `dev.mysql.com` and `mariadb.com` were blocked by the network policy, so the full pages were **not** read end to end. A link marks a claim the search snippet supported, not a claim checked against a running server.
- **UNVERIFIED** means the claim is not confirmed. Do not rely on it. Each one is repeated in the [checklist at the end](#unverified-checklist) so it can be tested against a real MariaDB (and MySQL) instance.
- Version numbers appear only where a source gave them. Otherwise the text says "TBD".

## 1. Verified facts used by this document

| # | Claim | Status | Source |
|---|---|---|---|
| V1 | MariaDB `INFORMATION_SCHEMA.INDEX_STATISTICS` belongs to User Statistics, which is off by default and enabled with the `userstat` system variable (`SET GLOBAL userstat=1` or option file). | Confirmed by search snippets | [MariaDB: User Statistics](https://mariadb.com/kb/en/mariadb/user-statistics/), [INDEX_STATISTICS](https://mariadb.com/docs/server/reference/system-tables/information-schema/information-schema-tables/information-schema-index_statistics-table) |
| V2 | `INDEX_STATISTICS` columns: `TABLE_SCHEMA`, `TABLE_NAME`, `INDEX_NAME`, `ROWS_READ`. An older KB page also lists a `QUERIES` column marked "from MariaDB 11.5"; the newer docs page does not list it. Treat `QUERIES` as version-dependent. | Confirmed, with a conflict between pages | [KB page](https://mariadb.com/kb/en/4057/), [docs page](https://mariadb.com/docs/server/reference/system-tables/information-schema/information-schema-tables/information-schema-index_statistics-table) |
| V3 | MariaDB has a `sys` schema from 10.6.0. It requires Performance Schema to be enabled. | Confirmed by search snippets | [MariaDB: Sys Schema](https://www.mariadb.com/docs/server/reference/sql-statements/administrative-sql-statements/system-tables/sys-schema) |
| V4 | MariaDB: `SHOW REPLICA STATUS` is an alias for `SHOW SLAVE STATUS` from 10.5.1. Required privilege: `REPLICATION SLAVE ADMIN` accepted from 10.5.2; `REPLICA MONITOR` from 10.5.9. | Confirmed by search snippets | [MariaDB: SHOW REPLICA STATUS](https://mariadb.com/kb/en/show-replica-status/) |
| V5 | MySQL 8.4 rejects `SHOW SLAVE STATUS` (syntax error) and accepts `SHOW REPLICA STATUS`. The release where MySQL introduced `REPLICA` syntax is not confirmed here. | 8.4 confirmed via a third-party blog; introduction version UNVERIFIED | [lefred: Since when in MariaDB do we use REPLICA?](https://lefred.be/content/mariadb-curiosity-since-when-in-mariadb-we-use-replica) |
| V6 | MySQL `performance_schema.events_statements_summary_by_digest`: key is (`SCHEMA_NAME`, `DIGEST`); has `DIGEST_TEXT`, `COUNT_STAR`, `SUM_TIMER_WAIT`, `FIRST_SEEN`, `LAST_SEEN`. Fixed capacity set by `performance_schema_digests_size`; overflow statements are aggregated into a row where `SCHEMA_NAME` and `DIGEST` are NULL. | Confirmed for MySQL by search snippets | [MySQL 8.4: statement summary tables](https://dev.mysql.com/doc/refman/8.4/en/performance-schema-statement-summary-tables.html), [8.4: statement digests](https://dev.mysql.com/doc/refman/8.4/en/performance-schema-statement-digests.html) |
| V7 | `performance_schema.table_io_waits_summary_by_index_usage` exists in MySQL and in MariaDB. Grouped by (`OBJECT_TYPE`, `OBJECT_SCHEMA`, `OBJECT_NAME`, `INDEX_NAME`). `INDEX_NAME` is NULL when no index was used (inserts counted here) and `PRIMARY` for the primary key. `COUNT_STAR` is the event count. `TRUNCATE` resets counters; DDL that changes a table's indexes may reset per-index statistics. | Confirmed by search snippets | [MariaDB KB](https://mariadb.com/kb/en/3991/), [MySQL: table wait summary tables](https://dev.mysql.com/doc/refman/en/performance-schema-table-wait-summary-tables.html) |
| V8 | MySQL 8.0 has `performance_schema.data_locks` and `data_lock_waits`. `INFORMATION_SCHEMA.INNODB_LOCKS` and `INNODB_LOCK_WAITS` were removed in 8.0.1. `data_locks` needs `SELECT` on the table (the old table needed `PROCESS`). | Confirmed by search snippets | [MySQL 8.0: data_locks](https://dev.mysql.com/doc/refman/8.0/en/performance-schema-data-locks-table.html) |
| V9 | MySQL `EXPLAIN ANALYZE` always uses TREE output. An updated JSON explain format (selected by `explain_json_format_version`) is described as introduced in 8.3 and able to show `EXPLAIN ANALYZE` output. | Confirmed by search snippets of a MySQL blog and 9.7 manual; exact 8.3 claim needs a re-read | [MySQL: EXPLAIN](https://dev.mysql.com/doc/en/explain.html), [MySQL blog: new JSON format](https://dev.mysql.com/blog-archive/new-json-format-for-explain/) |

## 2. Capability matrix (what the capability layer detects)

| Capability | MariaDB | MySQL | Detection query / probe | Source |
|---|---|---|---|---|
| Flavor and version | `SELECT VERSION()` contains "MariaDB" (assumed) | `SELECT VERSION()` | `VERSION()`, `@@version_comment` | UNVERIFIED |
| `performance_schema` on | Often off by default (see RISKS.md). Probe `@@performance_schema`. | On by default (assumed) | `SELECT @@performance_schema` | UNVERIFIED |
| Statement digests table | Table existence and column set must be probed | Yes ([V6](#1-verified-facts-used-by-this-document)) | `information_schema.tables` + `columns` for `events_statements_summary_by_digest` | MariaDB column list UNVERIFIED |
| Index I/O table | Yes, [V7](#1-verified-facts-used-by-this-document) | Yes, [V7](#1-verified-facts-used-by-this-document) | Table exists and has rows | [V7 sources](#1-verified-facts-used-by-this-document) |
| `userstat` | Yes, variable `userstat`, default off ([V1](#1-verified-facts-used-by-this-document)) | No (assumed) | `SELECT @@userstat` | [V1 sources](#1-verified-facts-used-by-this-document) |
| `sys` schema | 10.6.0+ and needs perf schema ([V3](#1-verified-facts-used-by-this-document)) | 5.7+ (UNVERIFIED) | `information_schema.schemata WHERE schema_name='sys'` | MySQL version UNVERIFIED |
| Replication status statement | `SHOW SLAVE STATUS` always; `SHOW REPLICA STATUS` from 10.5.1 ([V4](#1-verified-facts-used-by-this-document)) | `SHOW REPLICA STATUS` on 8.4 ([V5](#1-verified-facts-used-by-this-document)); `SHOW SLAVE STATUS` on older, cutoff UNVERIFIED | Choose by flavor and version; fall back on syntax error | See V4, V5 |
| `EXPLAIN FORMAT=JSON` | TBD | TBD | Try it; classify error | UNVERIFIED for both flavors and versions |
| `EXPLAIN ANALYZE` | TBD. A third-party guide calls it outdated syntax on MariaDB. | TREE output; first version UNVERIFIED | Try it; classify error | [V9](#1-verified-facts-used-by-this-document) |
| `ANALYZE FORMAT=JSON` | Expected on MariaDB, version UNVERIFIED | Not expected | Try it | UNVERIFIED |
| InnoDB lock introspection | `information_schema.INNODB_LOCKS` presence in recent versions UNVERIFIED | `performance_schema.data_locks` (8.0+) | Probe tables | MySQL: [V8](#1-verified-facts-used-by-this-document); MariaDB UNVERIFIED |

Rules: a feature declares which capabilities it needs; the layer returns `Available`, `Degraded(reason)`, or `Unavailable(enable_hint)`; features never run a catalog query without asking first.

## 3. Feature map

Columns: Feature | modhak equivalent | source catalogs/queries | MariaDB vs MySQL notes | milestone. Source markers apply to each catalog named.

| Feature | modhak equivalent | Source catalogs / queries | MariaDB vs MySQL notes | Milestone |
|---|---|---|---|---|
| Queries / slow queries | Queries page: per-digest totals, deltas between snapshots, top by total time | `performance_schema.events_statements_summary_by_digest` ([V6](#1-verified-facts-used-by-this-document)). Columns `SUM_ROWS_EXAMINED`, `SUM_NO_INDEX_USED`, `SUM_ROWS_SENT`, `QUERY_SAMPLE_TEXT`: UNVERIFIED. Snapshot, then diff. If a digest's `COUNT_STAR` drops versus the previous snapshot, treat it as a new baseline (counter reset, restart, `TRUNCATE`, or digest eviction); never emit a negative delta. | MariaDB column set UNVERIFIED. Overflow row has NULL schema and digest ([V6](#1-verified-facts-used-by-this-document)): store or skip it explicitly. | M2 |
| Live queries + kill | Live page; `KILL QUERY` button | `information_schema.PROCESSLIST` and/or `performance_schema.threads` (both UNVERIFIED); `KILL QUERY <id>` (UNVERIFIED) | Opt-in via config and `operator` role; every kill goes to `audit_log` (ADR 0006, 0007). Needed privilege UNVERIFIED. | M3 (kill: M4, needs auth) |
| Space and growth | Space page: per-table and per-schema size over time | `information_schema.tables` (`DATA_LENGTH`, `INDEX_LENGTH`, `TABLE_ROWS`) snapshots into `space_snapshots`. Column names UNVERIFIED. | MySQL 8.0 may cache `information_schema.tables` statistics (`information_schema_stats_expiry`): UNVERIFIED, test. `TABLE_ROWS` is an estimate for InnoDB. | M3 |
| Unused indexes | Finding `unused_index` with DDL and observation window | MariaDB: `information_schema.INDEX_STATISTICS` when `userstat=ON` ([V1, V2](#1-verified-facts-used-by-this-document)). Either flavor: `performance_schema.table_io_waits_summary_by_index_usage` ([V7](#1-verified-facts-used-by-this-document)). Join with `information_schema.statistics` to find indexes with zero reads. Show uptime (`SHOW GLOBAL STATUS LIKE 'Uptime'`, UNVERIFIED) as the observation window. | `userstat` MariaDB-only (assumed). Counters reset on restart and may reset on DDL ([V7](#1-verified-facts-used-by-this-document)); the verdict must say so. Unique and primary indexes are never recommended for drop. | M1 |
| Duplicate / redundant indexes | Findings `duplicate_index`, `redundant_index` with `ALTER TABLE ... DROP INDEX` | `information_schema.statistics` (columns `TABLE_SCHEMA`, `TABLE_NAME`, `INDEX_NAME`, `SEQ_IN_INDEX`, `COLUMN_NAME`, `NON_UNIQUE`, `INDEX_TYPE`; all UNVERIFIED as exact names). Prefix comparison in Go. | Behavior of `sub_part`, descending index parts (MySQL 8.0), ignored indexes (MariaDB 10.6 `IGNORED` / MySQL 8.0 `INVISIBLE`) all UNVERIFIED. | M1 |
| Suggested indexes | Finding `missing_index` tied to a digest | Digest text from `events_statements_summary_by_digest` or `digest_text`; parse (ADR 0003); check against `information_schema.statistics` and cardinality (`CARDINALITY` column, UNVERIFIED); verify with EXPLAIN | Parser accuracy on MariaDB syntax is a risk (RISKS.md). Verification statement per flavor: see matrix. | M5 |
| Explain | Explain page: SELECT-only, user supplies parameters; before/after comparison | `EXPLAIN FORMAT=JSON`, `EXPLAIN ANALYZE` (MySQL), `ANALYZE FORMAT=JSON` (MariaDB): all version support UNVERIFIED except [V9](#1-verified-facts-used-by-this-document) | Reject anything that is not a single `SELECT`. `EXPLAIN ANALYZE` / `ANALYZE` execute the statement, so they need extra care and an explicit opt-in. | M5 |
| Connections | Connections vs `max_connections`, grouped by user/host | `SHOW GLOBAL VARIABLES LIKE 'max_connections'`; `information_schema.PROCESSLIST` or `performance_schema.threads` (UNVERIFIED) | `PROCESS` privilege needed to see other users' threads (UNVERIFIED). | M3 |
| Replication lag | Replication page | `SHOW REPLICA STATUS` / `SHOW SLAVE STATUS` per flavor and version ([V4, V5](#1-verified-facts-used-by-this-document)); lag field `Seconds_Behind_Master` / `Seconds_Behind_Source` (UNVERIFIED) | MariaDB multi-source uses `SHOW ALL SLAVES STATUS` (UNVERIFIED). Privilege: MariaDB per [V4](#1-verified-facts-used-by-this-document); MySQL UNVERIFIED. | M3 |
| Cache hit rate | Health page: InnoDB buffer pool hit ratio | `SHOW GLOBAL STATUS` counters `Innodb_buffer_pool_reads`, `Innodb_buffer_pool_read_requests` (UNVERIFIED) | Ratio from deltas between snapshots, not lifetime totals. | M3 |
| Multiple databases | Target list | Config file (ADR 0008) | n/a | M0 |
| Vacuum / transaction wraparound | Long transactions and InnoDB history list length | `information_schema.INNODB_TRX` (`trx_started`), history list length from `SHOW ENGINE INNODB STATUS` or `information_schema.INNODB_METRICS` (`trx_rseg_history_len`); all UNVERIFIED | Metric name and enablement per flavor UNVERIFIED. | M3 |
| Sequence overflow | Finding `auto_increment_headroom` with days-to-overflow | `information_schema.columns` (`COLUMN_TYPE`, `EXTRA` containing `auto_increment`) and `information_schema.tables.AUTO_INCREMENT`; type max values below. All UNVERIFIED as documented. | See AUTO_INCREMENT note after the table. | M1 |
| Invalid indexes | Finding for ignored-but-present and unused-but-present indexes | `information_schema.statistics` (`IS_VISIBLE` in MySQL 8.0, `IGNORED` in MariaDB 10.6): both UNVERIFIED | Maps loosely; Postgres-style "invalid" indexes have no direct InnoDB analog. | M1 (best effort) |
| JSON API and MCP | Same findings as structured data | `internal/advisor` output (ARCHITECTURE.md) | n/a | M1 (CLI JSON), M4 (API, MCP) |
| Before/after EXPLAIN verification | Compare plans before and after a suggested index, in a non-prod setting | See Explain | Applying the DDL is the user's job; modhak never executes DDL (ADR 0007). | M5 |
| Schema lint | Missing PKs, mixed collations, oversized VARCHAR indexes | `information_schema.tables`, `columns`, `statistics` (`COLLATION_NAME`, `CHARACTER_MAXIMUM_LENGTH`; UNVERIFIED) | Index key length limits per flavor and row format: UNVERIFIED. | M5 |

### AUTO_INCREMENT headroom notes

- Max values by integer type are standard (TINYINT 127 / 255 unsigned, SMALLINT 32767 / 65535, MEDIUMINT 8388607 / 16777215, INT 2147483647 / 4294967295, BIGINT 9223372036854775807 / 18446744073709551615). **UNVERIFIED** against the MariaDB and MySQL "integer types" pages; confirm before shipping.
- Current value: `information_schema.tables.AUTO_INCREMENT` (UNVERIFIED). On MySQL 8.0 this can be served from a cache; confirm `information_schema_stats_expiry` behavior, or read it another way.
- Days-to-overflow needs a rate. M1 has one point in time, so M1 reports headroom percentage and uses `CREATE_TIME` / uptime only as a rough bound, labeled as such. Rate from history arrives with `space_snapshots` in M2/M3.
- Tables with a composite key where AUTO_INCREMENT is not the first column of the key (MyISAM) are out of scope until verified.

## 4. Minimal GRANTs per feature

All entries are **proposals**. Only the replication privilege names in V4 have a source. Verify each against a real instance with a user holding only that grant.

| Feature | Proposed grant | Status |
|---|---|---|
| Connect, `VERSION()`, `SHOW VARIABLES`, `SHOW STATUS` | none beyond login | UNVERIFIED |
| `information_schema.statistics/tables/columns` | visible only for objects the user has some privilege on, so grant `SELECT` on target schemas (or accept partial results) | UNVERIFIED |
| `performance_schema.*` digest, index I/O, threads | `SELECT ON performance_schema.*` | UNVERIFIED |
| `userstat` tables (MariaDB) | privilege to read `INDEX_STATISTICS` / `TABLE_STATISTICS` | UNVERIFIED |
| See other users' threads | `PROCESS` | UNVERIFIED |
| Replication status | MariaDB: `REPLICATION SLAVE ADMIN` (10.5.2+), `REPLICA MONITOR` (10.5.9+) ([V4](#1-verified-facts-used-by-this-document)). MySQL: UNVERIFIED (`REPLICATION CLIENT` expected) | MariaDB confirmed; MySQL UNVERIFIED |
| InnoDB locks (MySQL 8.0 `data_locks`) | `SELECT` on the table ([V8](#1-verified-facts-used-by-this-document)); `PROCESS` was for the removed table | Confirmed |
| `EXPLAIN` of a user SELECT | `SELECT` on the referenced tables | UNVERIFIED |
| `KILL QUERY` (opt-in, separate user recommended) | MariaDB / MySQL privilege names UNVERIFIED | UNVERIFIED |

## UNVERIFIED checklist

Test each against a real MariaDB instance (note the version), and a MySQL instance where marked. Tick when confirmed and update the table above.

- [ ] MariaDB: `events_statements_summary_by_digest` exists, its full column list, and whether `QUERY_SAMPLE_TEXT`, `SUM_ROWS_EXAMINED`, `SUM_NO_INDEX_USED`, `SUM_ROWS_SENT` exist.
- [ ] MySQL: `SUM_ROWS_EXAMINED`, `SUM_NO_INDEX_USED`, `SUM_ROWS_SENT`, `QUERY_SAMPLE_TEXT` exist in the target versions.
- [ ] `performance_schema` default value on MariaDB and MySQL; `@@performance_schema` probe works; which consumers and instruments are needed for digests and index I/O.
- [ ] `table_io_waits_summary_by_index_usage` column names beyond those in V7 (`COUNT_READ`, `COUNT_WRITE`, ...), and which instrument enables it by default on each flavor.
- [ ] MariaDB `INDEX_STATISTICS`: `QUERIES` column present or not, and from which version; privilege needed to read it; `FLUSH INDEX_STATISTICS` behavior.
- [ ] Whether `userstat` exists on MySQL (assumed: no).
- [ ] `sys` schema on MySQL: first version (5.7.7 assumed); which sys views modhak could use; MariaDB sys schema contents on 10.6+ vs 11.x.
- [ ] MySQL version where `SHOW REPLICA STATUS` was introduced (8.0.22 assumed) and where `SHOW SLAVE STATUS` was removed (8.4 rejects it per V5).
- [ ] Replication lag column names per flavor (`Seconds_Behind_Master` vs `Seconds_Behind_Source`), and MariaDB `SHOW ALL SLAVES STATUS` / `SHOW ALL REPLICAS STATUS`.
- [ ] MySQL privilege for replication status (`REPLICATION CLIENT` assumed).
- [ ] `EXPLAIN FORMAT=JSON` support and output shape on MariaDB and MySQL, by version.
- [ ] `EXPLAIN ANALYZE`: MySQL first version (8.0.18 assumed) and whether MariaDB accepts it (and what it maps to).
- [ ] `ANALYZE FORMAT=JSON` on MariaDB: first version (10.1 assumed), and that it executes the statement.
- [ ] MySQL JSON explain v2 (`explain_json_format_version`) introduced in 8.3, and whether it works with `EXPLAIN ANALYZE`.
- [ ] `information_schema.tables.AUTO_INCREMENT` accuracy, and the effect of `information_schema_stats_expiry` on MySQL 8.0.
- [ ] `information_schema.columns` fields (`COLUMN_TYPE`, `EXTRA`) for detecting AUTO_INCREMENT and signedness.
- [ ] Integer max values per type (standard values listed above).
- [ ] `information_schema.statistics` column names and `SUB_PART`, `CARDINALITY`, `IS_VISIBLE` (MySQL) / `IGNORED` (MariaDB) availability by version.
- [ ] Descending index parts and functional indexes (MySQL 8.0) in `statistics`, and how the duplicate detector must treat them.
- [ ] `performance_schema.threads` vs `information_schema.PROCESSLIST` columns, and MariaDB's perf-schema threads coverage.
- [ ] `KILL QUERY` privilege requirements on both flavors (own threads vs others').
- [ ] InnoDB history list length source per flavor (`INNODB_METRICS` name, `SHOW ENGINE INNODB STATUS`) and whether the counter is enabled by default.
- [ ] MariaDB lock introspection tables in current versions (`information_schema.INNODB_LOCKS`, `INNODB_LOCK_WAITS`, or metadata locks via `METADATA_LOCK_INFO`).
- [ ] `Uptime` status variable use as the observation window, and which operations reset index counters (restart, `TRUNCATE`, DDL, `FLUSH`).
- [ ] Buffer pool counters `Innodb_buffer_pool_reads` / `Innodb_buffer_pool_read_requests` on both flavors.
- [ ] `max_connections` and per-user connection views on both flavors.
- [ ] Every row in the "Minimal GRANTs" table, with a user holding only that grant.
- [ ] `SELECT VERSION()` / `@@version_comment` flavor detection on MariaDB, MySQL, and common forks (Percona, Aurora, RDS, Cloud SQL): TBD whether forks need special handling.
