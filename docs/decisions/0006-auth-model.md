# 0006: Authentication and authorization

Status: accepted (implementation in M4)

## Context
Many users run modhak on an internal network and want zero setup. Others need access control, and AI agents need non-interactive credentials. Some actions (kill) are destructive.

## Decision
- Auth is off by default.
- Later: generic OIDC (go-oidc + oauth2) with PKCE and group-to-role mapping to `viewer` and `operator`; API tokens for agents and MCP; CSRF protection on POSTs; an audit log for destructive actions (kill).
- Every request path constructs a `Principal`, even when auth is off.

## Consequences
- Running with auth off on an untrusted network is the operator's risk; the docs must say so, and the default listen address should be loopback (TBD).
- Roles are coarse (two roles); finer permissions are out of scope for now.
- Token hashing, scoping, and session details are TBD.
- The audit log lives in the store, so it is lost if the store is lost; export is a Later item.
