package store

import (
	"context"
	"fmt"
	"io/fs"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
)

func legacyHomeDatabase(t *testing.T, version int64) *DB {
	t.Helper()
	files := fstest.MapFS{}
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		number, err := strconv.Atoi(name[:5])
		if err != nil {
			t.Fatal(err)
		}
		if number > 60 || (number > 55 && number < 58) {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if number >= 58 {
			name = fmt.Sprintf("%05d%s", number-2, name[5:])
		}
		files[name] = &fstest.MapFile{Data: body}
	}
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(context.Background(), version); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateReconcilesLegacyHomeWithoutLosingData(t *testing.T) {
	later, latest := migrationsAbove(t, 60)
	for _, version := range []int64{56, 57, 58} {
		t.Run(strconv.FormatInt(version, 10), func(t *testing.T) {
			db := legacyHomeDatabase(t, version)
			execSQL(t, db, `insert into videos(id, content_key) values (1, 'retained')`)
			execSQL(t, db, `insert into playback_progress(content_key, position_ms, completed, updated_at)
				values ('retained', 12345, 0, 100)`)
			execSQL(t, db, `insert into watch_history(content_key, playback_id, title, played_at)
				values ('retained', 'play-1', 'My video', 100)`)
			execSQL(t, db, `insert into video_favorites(content_key, favorited_at) values ('retained', 100)`)
			execSQL(t, db, `insert into smart_folders(id, name, title_key, rule, sort, created_at, updated_at, home_position)
				values (1, 'My shelf', 'my shelf', '{}', 'addedDesc', 100, 100, 1)`)
			execSQL(t, db, `insert into settings(key, value, updated_at) values ('onboarding.step', 'done', 100)`)

			// Reproduce the user's failed startup with the renumbered SQL provider.
			if version == 58 {
				files, err := fs.Sub(migrationsFS, "migrations")
				if err != nil {
					t.Fatal(err)
				}
				provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, files)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := provider.Up(context.Background()); err == nil || !strings.Contains(err.Error(), "group_closure already exists") {
					t.Fatalf("old startup error = %v", err)
				}
			}

			result, err := Migrate(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if result.Version != latest || result.Applied != int(60-version)+later {
				t.Fatalf("result = %+v", result)
			}
			for _, query := range []string{
				`select count(*) from videos where id = 1 and content_key = 'retained'`,
				`select count(*) from playback_progress where content_key = 'retained' and position_ms = 12345`,
				`select count(*) from watch_history where playback_id = 'play-1' and title = 'My video'`,
				`select count(*) from video_favorites where content_key = 'retained' and favorited_at = 100`,
				`select count(*) from smart_folders where id = 1 and name = 'My shelf'`,
				`select count(*) from settings where key = 'onboarding.step' and value = 'done'`,
			} {
				var count int
				if err := db.sql.QueryRow(query).Scan(&count); err != nil || count != 1 {
					t.Fatalf("preserved row (%s) = %d, %v", query, count, err)
				}
			}
			// Both formerly skipped main migrations must be usable, not just recorded.
			execSQL(t, db, `insert into video_keyframe_spacing(video_id, version, size_bytes, mtime_ns, spacing)
				values (1, 1, 10, 123456789, 'short')`)
			execSQL(t, db, `update video_locations set mtime_ns = 123456789`)
			var history string
			if err := db.sql.QueryRow(`select group_concat(version_id, ',') from
				(select version_id from goose_db_version where version_id between 56 and 60 and is_applied = 1 order by version_id)`).Scan(&history); err != nil {
				t.Fatal(err)
			}
			if history != "56,57,58,59,60" {
				t.Fatalf("history = %q", history)
			}
			second, err := Migrate(context.Background(), db)
			if err != nil || second.Applied != 0 || second.Version != latest {
				t.Fatalf("second migration = %+v, %v", second, err)
			}
		})
	}
}

func TestLegacyHomeRepairRollsBackSchemaAndHistoryTogether(t *testing.T) {
	db := legacyHomeDatabase(t, 58)
	// Fail after renumbering and executing the first missing main migration.
	execSQL(t, db, `create trigger fail_repair before insert on goose_db_version
		when new.version_id = 57 begin select raise(abort, 'test repair failure'); end`)
	_, err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "test repair failure") {
		t.Fatalf("migration error = %v", err)
	}
	var version int64
	if err := db.sql.QueryRow(`select max(version_id) from goose_db_version`).Scan(&version); err != nil || version != 58 {
		t.Fatalf("version = %d, %v", version, err)
	}
	if len(tableColumns(t, db, "video_keyframe_spacing")) != 0 {
		t.Fatal("failed repair left the keyframe table")
	}
	if _, exists := tableColumns(t, db, "video_locations")["mtime_ns"]; exists {
		t.Fatal("failed repair left the timestamp column")
	}
	execSQL(t, db, `drop trigger fail_repair`)
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestLegacyHomeRepairLeavesMainSchemasAlone(t *testing.T) {
	_, latest := migrationsAbove(t, 0)
	for _, version := range []int64{55, 56, 57, 58, 59, 60} {
		t.Run(strconv.FormatInt(version, 10), func(t *testing.T) {
			db, _ := migrateTo(t, version)
			if _, err := Migrate(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			want, _ := migrateTo(t, latest)
			if got := tableColumns(t, db, "video_locations"); !reflect.DeepEqual(got, tableColumns(t, want, "video_locations")) {
				t.Fatalf("location schema differs: %v", got)
			}
		})
	}
}

func TestLegacyHomeRepairRejectsAmbiguousHistory(t *testing.T) {
	db := legacyHomeDatabase(t, 58)
	execSQL(t, db, `delete from goose_db_version where version_id = 57`)
	_, err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "unrecognized legacy home migration history") {
		t.Fatalf("migration error = %v", err)
	}
	if len(tableColumns(t, db, "video_keyframe_spacing")) != 0 {
		t.Fatal("ambiguous database was modified")
	}
}

func TestLegacyHomeRepairRejectsAmbiguousSchema(t *testing.T) {
	db := legacyHomeDatabase(t, 58)
	execSQL(t, db, `drop table home_search_hits`)
	_, err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "unrecognized legacy home schema") {
		t.Fatalf("migration error = %v", err)
	}
	if len(tableColumns(t, db, "video_keyframe_spacing")) != 0 {
		t.Fatal("ambiguous database was modified")
	}
}
