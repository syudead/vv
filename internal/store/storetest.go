package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ForTest helpers expose otherwise unreachable fixtures to external test packages; production code must not call them.

func SetJobAttemptsForTest(ctx context.Context, db *DB, kind domain.JobKind, videoID int64, attempts int) error {
	_, err := db.sql.ExecContext(ctx,
		`update jobs set attempts = ? where kind = ? and video_id = ?`, attempts, string(kind), videoID)
	return err
}

// MakeJobsDueForTest ends every retry wait, for tests whose contract lies past a failed round rather than in its delay.
func MakeJobsDueForTest(ctx context.Context, db *DB) error {
	_, err := db.sql.ExecContext(ctx, `update jobs set run_after = 0 where state = 'queued'`)
	return err
}

func DeleteJobsForTest(ctx context.Context, db *DB) error {
	_, err := db.sql.ExecContext(ctx, `delete from jobs`)
	return err
}

func CountJobsForTest(ctx context.Context, db *DB) (int, error) {
	var count int
	err := db.sql.QueryRowContext(ctx, `select count(*) from jobs`).Scan(&count)
	return count, err
}

func CountVideoJobsForTest(ctx context.Context, db *DB, kind domain.JobKind, videoID int64) (int, error) {
	var count int
	err := db.sql.QueryRowContext(ctx,
		`select count(*) from jobs where kind = ? and video_id = ?`, string(kind), videoID).Scan(&count)
	return count, err
}

// Cache one migrated database image per process to avoid repeating migration cost for every fixture.
var migratedImage struct {
	once  sync.Once
	bytes []byte
	err   error
}

// OpenMigratedForTest copies a cached schema for fresh fixtures but migrates existing databases in place; migration tests must use Open and Migrate directly.
func OpenMigratedForTest(dataDir string) (*DB, error) {
	if _, err := os.Stat(DatabasePath(dataDir)); err == nil {
		return openAndMigrate(dataDir)
	}
	migratedImage.once.Do(func() {
		migratedImage.bytes, migratedImage.err = buildMigratedImage()
	})
	if migratedImage.err != nil {
		return nil, migratedImage.err
	}
	if err := os.WriteFile(DatabasePath(dataDir), migratedImage.bytes, 0o600); err != nil {
		return nil, fmt.Errorf("cannot write the migrated database: %w", err)
	}
	return Open(dataDir)
}

// Checkpoint WAL before reading the migrated database image.
func buildMigratedImage() ([]byte, error) {
	dir, err := os.MkdirTemp("", "mdm-migrated-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	db, err := openAndMigrate(dir)
	if err != nil {
		return nil, err
	}
	if _, err := db.sql.Exec(`pragma wal_checkpoint(truncate)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(DatabasePath(dir))
}

func openAndMigrate(dataDir string) (*DB, error) {
	db, err := Open(dataDir)
	if err != nil {
		return nil, err
	}
	if _, err := Migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// RecordActivityForTest writes through the production activity helper and publishes after commit.
func RecordActivityForTest(ctx context.Context, db *DB, entry domain.ActivityEntry, now time.Time) (int64, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	id, err := recordActivity(ctx, tx, c, entry, now)
	if err != nil {
		return 0, err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return 0, err
	}
	return id, nil
}

// SetPlayedAtForTest saves a position for contentKey last updated at playedAt, the value the played sorts order by.
func SetPlayedAtForTest(ctx context.Context, db *DB, contentKey string, playedAt time.Time) error {
	_, err := db.sql.ExecContext(ctx, `
		insert into playback_progress (content_key, position_ms, duration_ms, completed, updated_at)
		values (?, 1000, 60000, 0, ?)
		on conflict (content_key) do update set updated_at = excluded.updated_at`, contentKey, playedAt.Unix())
	return err
}
