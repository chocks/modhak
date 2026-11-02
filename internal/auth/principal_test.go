package auth

import (
	"context"
	"testing"
)

func TestPrincipalContext(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("empty context should have no principal")
	}
	want := Principal{Name: "alice", Role: RoleOperator}
	got, ok := FromContext(NewContext(context.Background(), want))
	if !ok || got != want {
		t.Fatalf("got %+v %v", got, ok)
	}
	if p := Implicit(); p.Role != RoleViewer || p.Name == "" {
		t.Fatalf("implicit principal = %+v", p)
	}
}
