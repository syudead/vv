package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestRemoveHomeShelvesPreservesLibrary(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 61); err != nil {
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

	var count int
	if err := db.sql.QueryRow(`select count(*) from sqlite_schema where name glob 'home_*' or name glob 'group_closure*' or name glob 'group_root_values*' or name glob 'video_watch_*' or (type = 'trigger' and (sql like '%home_%' or sql like '%group_closure%' or sql like '%group_root_values%' or sql like '%video_watch_%'))`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("retired schema objects = %d, %v", count, err)
	}
	for _, column := range []string{"home_position", "owner_kept"} {
		if _, exists := tableColumns(t, db, "smart_folders")[column]; exists {
			t.Fatalf("retired column %s remains", column)
		}
	}
	var position, folder int
	if err := db.sql.QueryRow(`select p.position_ms, f.id from videos v join playback_progress p on p.content_key = v.content_key join smart_folders f on f.id = 1 where v.id = 1`).Scan(&position, &folder); err != nil || position != 12345 || folder != 1 {
		t.Fatalf("preserved library = position %d, folder %d, %v", position, folder, err)
	}
	// Writes must not retain triggers referencing the removed indexes.
	execSQL(t, db, `update playback_progress set completed = 1 where content_key = 'kept'`)
	execSQL(t, db, `update smart_folders set rule = '{"q":"kept"}' where id = 1`)
}
