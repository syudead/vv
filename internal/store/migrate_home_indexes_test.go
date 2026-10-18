package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestHomeIndexMigrationPreservesLibraryAndTracksProgress(t *testing.T) {
	ctx := context.Background()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	files, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 60); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `insert into videos(id, content_key) values (1, 'kept')`)
	execSQL(t, db, `insert into playback_progress(content_key, position_ms, completed, updated_at)
		values ('kept', 12345, 0, 100)`)
	execSQL(t, db, `insert into smart_folders(id, name, title_key, rule, sort, created_at, updated_at, home_position)
		values (1, 'My shelf', 'my shelf', '{}', 'addedDesc', 100, 100, 1)`)
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var names string
	err = db.sql.QueryRow(`select group_concat(name, ',') from
		(select name from sqlite_schema where type = 'table' and name glob 'home_*' order by name)`).Scan(&names)
	if err != nil || names != "home_index_state,home_search_hits" {
		t.Fatalf("home tables = %v, %v", names, err)
	}
	var position, folder int
	if err := db.sql.QueryRow(`select p.position_ms, f.id from videos v
		join playback_progress p on p.content_key = v.content_key
		join smart_folders f on f.home_position = 1 where v.id = 1`).Scan(&position, &folder); err != nil || position != 12345 || folder != 1 {
		t.Fatalf("preserved library = position %d, folder %d, %v", position, folder, err)
	}
	// Progress writes must still update the index after the migration replaces its triggers.
	execSQL(t, db, `update playback_progress set completed = 1 where content_key = 'kept'`)
	if err := db.maintainHomeIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.sql.QueryRow(`select state from video_watch_states where content_key = 'kept'`).Scan(&state); err != nil || state != "watched" {
		t.Fatalf("watch state = %q, %v", state, err)
	}
	// Renaming the shared state table must also update the existing search triggers.
	execSQL(t, db, `update smart_folders set rule = '{"q":"kept"}' where id = 1`)
	var dirty bool
	if err := db.sql.QueryRow(`select search_dirty from home_index_state`).Scan(&dirty); err != nil || !dirty {
		t.Fatalf("search dirty = %v, %v", dirty, err)
	}
}
