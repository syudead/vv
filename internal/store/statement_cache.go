package store

import (
	"context"
	"database/sql"
	"sync"
)

// maxCachedStatements bounds the cache; statement text varies only with the audience, so a few entries cover
// every caller and a full cache runs further statements unprepared.
const maxCachedStatements = 64

// statementCache keeps prepared statements on the read pool, so a hot read such as one thumbnail's video lookup
// is parsed once instead of on every request.
type statementCache struct {
	read  *sql.DB
	mu    sync.Mutex
	stmts map[string]preparedQuery
}

// preparedQuery is a statement the cache owns and closes in close.
type preparedQuery interface {
	QueryRowContext(ctx context.Context, args ...any) *sql.Row
	QueryContext(ctx context.Context, args ...any) (*sql.Rows, error)
	Close() error
}

func prepare(ctx context.Context, db *sql.DB, query string) (preparedQuery, error) {
	return db.PrepareContext(ctx, query)
}

func (c *statementCache) prepared(ctx context.Context, query string) preparedQuery {
	c.mu.Lock()
	defer c.mu.Unlock()
	if stmt, ok := c.stmts[query]; ok {
		return stmt
	}
	if len(c.stmts) >= maxCachedStatements {
		return nil
	}
	stmt, err := prepare(ctx, c.read, query)
	if err != nil {
		// The unprepared call reports the same error to its caller.
		return nil
	}
	if c.stmts == nil {
		c.stmts = map[string]preparedQuery{}
	}
	c.stmts[query] = stmt
	return stmt
}

func (c *statementCache) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if stmt := c.prepared(ctx, query); stmt != nil {
		return stmt.QueryRowContext(ctx, args...)
	}
	return c.read.QueryRowContext(ctx, query, args...)
}

func (c *statementCache) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if stmt := c.prepared(ctx, query); stmt != nil {
		return stmt.QueryContext(ctx, args...)
	}
	return c.read.QueryContext(ctx, query, args...)
}

func (c *statementCache) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var first error
	for _, stmt := range c.stmts {
		if err := stmt.Close(); err != nil && first == nil {
			first = err
		}
	}
	c.stmts = nil
	return first
}
