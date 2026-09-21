// Package testsupport holds helpers for integration tests.
package testsupport

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewSchema creates a throwaway schema from deployments/postgres/init.sql and
// returns a pool plus the schema name; both are cleaned up with the test.
// Skips unless TEST_POSTGRES_DSN is set, e.g.
//
//	TEST_POSTGRES_DSN=postgres://user:pass@localhost:5433/movieapp?sslmode=disable
func NewSchema(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	_, file, _, _ := runtime.Caller(0)
	initSQL, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../deployments/postgres/init.sql"))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("itest_%d", rand.Int64N(1<<40))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	})
	if _, err := pool.Exec(ctx, strings.ReplaceAll(string(initSQL), "interaction", schema)); err != nil {
		t.Fatalf("apply init.sql to %s: %v", schema, err)
	}
	return pool, schema
}
