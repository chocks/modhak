# 0005: One findings layer (internal/advisor)

Status: accepted

## Context
The web UI, JSON API, MCP server, and CLI all need the same advice. Duplicating logic across them would let results diverge.

## Decision
`internal/advisor` holds pure functions that produce `Finding{Kind, Severity, Target, Evidence, SuggestedSQL}`. Web, API, and MCP are thin consumers. All entry points take a `Principal` for authorization.

## Consequences
- Advisor code has no HTTP, no DB handles, and no clock reads, so it is unit-testable with table-driven tests.
- Evidence must be structured so agents can use it without parsing prose.
- Adding a consumer costs little; adding a finding kind touches one place plus rendering.
- How the Principal reaches advisor functions is an open question (ARCHITECTURE.md).
