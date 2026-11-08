package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

// Repeated reads reuse one prepared statement; a full cache and a bad query still answer like the pool does.
func TestStatementCacheReusesStatementsAndFallsBack(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	cache := db.cached
	count := func(query string, arg int) int {
		t.Helper()
		var n int
		if err := cache.QueryRowContext(ctx, query, arg).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(`select ? + 1`, 1) != 2 || count(`select ? + 1`, 41) != 42 {
		t.Fatal("a cached statement answered the wrong value")
	}
	if len(cache.stmts) != 1 {
		t.Fatalf("cached %d statements for one query, want 1", len(cache.stmts))
	}
	for i := len(cache.stmts); i < maxCachedStatements; i++ {
		count(fmt.Sprintf(`select ? + %d`, i), 0)
	}
	if got := count(`select ? * 3`, 5); got != 15 || len(cache.stmts) != maxCachedStatements {
		t.Fatalf("past the bound: %d with %d statements, want 15 with %d", got, len(cache.stmts), maxCachedStatements)
	}
	if got := readLinkCount(t, cache); got != 0 {
		t.Fatalf("rows through the cache = %d, want 0", got)
	}
	if err := cache.QueryRowContext(ctx, `select from nowhere`).Scan(new(int)); err == nil {
		t.Fatal("a bad query read without an error")
	}
}

// Parallel reads leave their connections idle instead of closing all but two.
func TestReadPoolKeepsParallelConnectionsIdle(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	open := make([]*sql.Conn, 6)
	for i := range open {
		conn, err := db.read.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		open[i] = conn
	}
	for _, conn := range open {
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if idle := db.read.Stats().Idle; idle != len(open) {
		t.Fatalf("idle read connections = %d, want %d", idle, len(open))
	}
}

func readLinkCount(t *testing.T, q rowsQueryer) int {
	t.Helper()
	links, err := readLinks(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return len(links)
}
