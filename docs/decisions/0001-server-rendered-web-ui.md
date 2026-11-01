# 0001: Go backend, server-rendered UI (html/template + htmx + uPlot)

Status: accepted

## Context
modhak ships as a single static binary for operators who don't want a Node toolchain or a separate frontend deploy. The UI is mostly tables, findings, and a few time-series charts.

## Decision
Go backend. Pages rendered with `html/template`. htmx for partial updates. uPlot for charts. Templates and static files are embedded with `go:embed`. No Vue, no SPA, no Node build.

## Consequences
- One artifact, simple deploys, no frontend build step in CI.
- Interactivity is limited to what htmx and small scripts provide; complex client state is out of scope.
- Vendored htmx and uPlot files are committed; updating them is a manual step.
- Partials and full pages share templates (rule in ARCHITECTURE.md).
