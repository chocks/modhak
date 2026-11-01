package target

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chocks/modhak/internal/capability"
	"github.com/chocks/modhak/internal/config"
)

// Integration test against a live server; skipped unless MODHAK_TEST_ADDR
// (host:port) is set. Credentials default to the docker compose dev user.
// Run with: MODHAK_TEST_ADDR=127.0.0.1:33061 go test ./internal/target
// Flavor-agnostic: it works on either MariaDB or MySQL.
func TestLiveReadOnlySession(t *testing.T) {
	addr := os.Getenv("MODHAK_TEST_ADDR")
	if addr == "" {
		t.Skip("MODHAK_TEST_ADDR not set")
	}
	host, port, _ := strings.Cut(addr, ":")
	var p int
	for _, ch := range port {
		p = p*10 + int(ch-'0')
	}
	c, err := Open(config.Target{Name: "live", Host: host, Port: p, User: "modhak", Password: "modhak",
		ConnectTimeout: 3 * time.Second, QueryTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()

	// The variable is tx_read_only on older servers and transaction_read_only
	// on newer ones; accept whichever exists.
	var v string
	for _, q := range []string{"SELECT @@session.transaction_read_only", "SELECT @@session.tx_read_only"} {
		if got, err := c.Scalar(ctx, q); err == nil {
			v = got.String
			break
		}
	}
	if v != "1" {
		t.Fatalf("session read-only flag = %q, want 1", v)
	}

	rep, err := capability.Detect(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("detected:\n%s", rep.Text())
}
