# 0007: Safety defaults

Status: accepted

## Context
modhak points at production databases. A monitoring tool that can change data or schema is a liability, and index-drop advice can be wrong (for example, with too little observation time).

## Decision
- The DB user is read-only by default; the README documents minimal GRANTs.
- Kill-query is opt-in via config and requires the `operator` role; each kill is written to the audit log.
- modhak never auto-executes DDL. Index drops are recommendations that carry evidence and an "observed for N days" caveat.
- Explain accepts a single SELECT only; execution-based explain is opt-in.

## Consequences
- Users apply suggested SQL themselves; modhak does not close the loop. Before/after verification (M5) is done by re-explaining, not by applying.
- Some features need extra grants; they are documented per feature and stay off without them.
- Findings must state their limits (uptime, replica vs primary) rather than imply certainty.
