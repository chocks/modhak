// Package auth defines who is making a request. M1 has only the implicit
// principal used while auth is off; roles, OIDC, API tokens and CSRF arrive in
// M4 (ADR 0006). Web, API and MCP entry points already build and pass a
// Principal so those milestones do not have to thread one through later.
package auth

import "context"

// Role is a principal's permission level.
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
)

// Principal identifies the caller.
type Principal struct {
	Name string
	Role Role
}

// Implicit returns the principal used when auth is off (the default). It is a
// viewer: M1 has no mutating actions, and a later milestone decides what auth
// off may allow (ARCHITECTURE.md open question 4).
func Implicit() Principal { return Principal{Name: "anonymous", Role: RoleViewer} }

type ctxKey struct{}

// NewContext returns ctx carrying p.
func NewContext(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the principal in ctx.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
