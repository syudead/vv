package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Path is the automatic folder key; Members resolves stored user keys to videos in item order.
type storedGroup struct {
	Path    string
	Name    string
	Members []int64
}

func storedGroups(t *testing.T, db *DB) []storedGroup {
	t.Helper()
	rows, err := db.sql.Query(`select g.id, g.origin_folder, g.name, g.title_key, v.id
		from groups g
		left join group_items i on i.group_id = g.id and i.content_key is not null
		left join videos v on v.content_key = i.content_key
		where g.origin_folder is not null
		order by g.origin_folder, i.position`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []storedGroup{}
	lastID := int64(-1)
	for rows.Next() {
		var id int64
		var path, name, titleKey string
		var member sql.NullInt64
		if err := rows.Scan(&id, &path, &name, &titleKey, &member); err != nil {
			t.Fatal(err)
		}
		if titleKey != domain.NaturalSortKey(name) {
			t.Errorf("title_key of %q = %q", name, titleKey)
		}
		if id != lastID {
			out = append(out, storedGroup{Path: path, Name: name, Members: []int64{}})
			lastID = id
		}
		if member.Valid {
			out[len(out)-1].Members = append(out[len(out)-1].Members, member.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func storedFolderNames(t *testing.T, db *DB) map[int64][]string {
	t.Helper()
	rows, err := db.sql.Query(`select video_id, name from video_folder_names order by video_id, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	names := map[int64][]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		names[id] = append(names[id], name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func assertGroups(t *testing.T, db *DB, want []storedGroup) {
	t.Helper()
	if want == nil {
		want = []storedGroup{}
	}
	if got := storedGroups(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %+v, want %+v", got, want)
	}
}

func upsertFolderVideo(t *testing.T, db *DB, path, key string) int64 {
	t.Helper()
	result, err := db.ScanIndex().UpsertVideo(context.Background(), sampleFile(path, key, key, 10, 0))
	if err != nil {
		t.Fatal(err)
	}
	return result.ID
}

func TestRebuildFolderIndexAfterScan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	one := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	ten := upsertFolderVideo(t, db, fixturePath("/media/Show/10.mp4"), "k10")
	two := upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	loose := upsertFolderVideo(t, db, fixturePath("/media/loose.mp4"), "loose")
	assertGroups(t, db, nil)

	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{{Path: fixturePath("/media/Show"), Name: "Show", Members: []int64{one, two, ten}}})
	if names := storedFolderNames(t, db); !reflect.DeepEqual(names, map[int64][]string{one: {"Show"}, two: {"Show"}, ten: {"Show"}}) {
		t.Fatalf("folder names = %v (loose=%d)", names, loose)
	}

	nested := upsertFolderVideo(t, db, fixturePath("/media/Show/Extra/a.mp4"), "extra")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{{Path: fixturePath("/media/Show"), Name: "Show", Members: []int64{one, two, ten}}})
	if names := storedFolderNames(t, db); !reflect.DeepEqual(names[nested], []string{"Extra", "Show"}) {
		t.Fatalf("nested folder names = %v", names[nested])
	}

	var version, searchVersion, stale int
	if err := db.sql.QueryRow(`select version, search_version, stale from folder_index_state where id = 1`).
		Scan(&version, &searchVersion, &stale); err != nil {
		t.Fatal(err)
	}
	if version != domain.FolderIndexVersion || searchVersion != domain.SearchKeyVersion || stale != 0 {
		t.Fatalf("state = %d, %d, %d", version, searchVersion, stale)
	}
}

// Removing registration can move a surviving video's representative and create a new folder group in the same transaction.
func TestRebuildFolderIndexOnMediaFolderChanges(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	shared := upsertFolderVideo(t, db, fixturePath("/aaa/c/1.mp4"), "shared")
	onlyAAA := upsertFolderVideo(t, db, fixturePath("/aaa/c/3.mp4"), "only-aaa")
	upsertFolderVideo(t, db, fixturePath("/media/b/1.mp4"), "shared")
	onlyMedia := upsertFolderVideo(t, db, fixturePath("/media/b/2.mp4"), "only-media")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{{Path: fixturePath("/media/b"), Name: "b", Members: []int64{shared, onlyMedia}}})

	added, err := db.Settings().AddMediaFolder(ctx, fixturePath("/aaa"))
	if err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{
		{Path: fixturePath("/aaa/c"), Name: "c", Members: []int64{shared, onlyAAA}},
		{Path: fixturePath("/media/b"), Name: "b", Members: []int64{shared, onlyMedia}},
	})
	if names := storedFolderNames(t, db); !reflect.DeepEqual(names[shared], []string{"b", "c"}) {
		t.Fatalf("shared folder names = %v", names[shared])
	}

	if err := db.Settings().DeleteMediaFolder(ctx, added.ID, added.Version); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{
		{Path: fixturePath("/aaa/c"), Name: "c", Members: []int64{}},
		{Path: fixturePath("/media/b"), Name: "b", Members: []int64{shared, onlyMedia}},
	})
	if names := storedFolderNames(t, db); !reflect.DeepEqual(names[shared], []string{"b"}) {
		t.Fatalf("shared folder names after delete = %v", names[shared])
	}
}

func TestFolderGroupOverrides(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Settings().AddMediaFolder(ctx, fixturePath("/media")); err != nil {
		t.Fatal(err)
	}
	a := upsertFolderVideo(t, db, fixturePath("/media/Show/a.mp4"), "a")
	b := upsertFolderVideo(t, db, fixturePath("/media/Show/b.mp4"), "b")
	rootA := upsertFolderVideo(t, db, fixturePath("/media/x.mp4"), "x")
	rootB := upsertFolderVideo(t, db, fixturePath("/media/y.mp4"), "y")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	show := storedGroup{Path: fixturePath("/media/Show"), Name: "Show", Members: []int64{a, b}}
	root := storedGroup{Path: fixturePath("/media"), Name: "media", Members: []int64{rootA, rootB}}
	assertGroups(t, db, []storedGroup{show})

	groups := db.FolderGroups()
	if err := groups.SetOverride(ctx, fixturePath("/media/Show/"), domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, nil)
	if err := groups.SetOverride(ctx, fixturePath("/media/Show"), domain.FolderGroupDirect); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{show})
	if err := groups.SetOverride(ctx, fixturePath("/media"), domain.FolderGroupDirect); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{root, show})
	if err := groups.ClearOverride(ctx, fixturePath("/media")); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{root, show})
	if err := groups.SetOverride(ctx, fixturePath("/media/Show"), "both"); err == nil {
		t.Fatal("unknown mode was accepted")
	}
	if err := groups.SetOverride(ctx, fixturePath("/media/Gone"), domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	if err := groups.SetOverride(ctx, fixturePath("/media/Show"), domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{root})

	c := upsertFolderVideo(t, db, fixturePath("/media/Show/c.mp4"), "c")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{root})

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if rebuilt, err := db.ScanIndex().RefreshFolderIndex(ctx); err != nil || rebuilt {
		t.Fatalf("refresh = %v, %v; want no rebuild", rebuilt, err)
	}
	var overrides int
	if err := db.sql.QueryRow(`select count(*) from folder_group_overrides`).Scan(&overrides); err != nil {
		t.Fatal(err)
	}
	if overrides != 2 {
		t.Fatalf("overrides = %d, want 2", overrides)
	}
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{a, b, c}); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	upsertFolderVideo(t, db, fixturePath("/media/Show/a.mp4"), "a")
	upsertFolderVideo(t, db, fixturePath("/media/Show/b.mp4"), "b")
	if err := db.FolderGroups().ClearOverride(ctx, fixturePath("/media/Nothing")); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{root})
}

func TestRefreshFolderIndex(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	store := db.ScanIndex()
	refresh := func() bool {
		t.Helper()
		rebuilt, err := store.RefreshFolderIndex(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rebuilt
	}
	if !refresh() {
		t.Fatal("no state row: want rebuild")
	}
	if refresh() {
		t.Fatal("current state: want no rebuild")
	}
	for _, update := range []string{
		`update folder_index_state set version = version + 1`,
		`update folder_index_state set search_version = search_version + 1`,
		`update folder_index_state set stale = 1`,
	} {
		if _, err := db.sql.Exec(update); err != nil {
			t.Fatal(err)
		}
		if !refresh() {
			t.Fatalf("%s: want rebuild", update)
		}
		if refresh() {
			t.Fatalf("after %s: want no rebuild", update)
		}
	}
}

// A failed rebuild preserves the old index and marks it stale in a separate transaction.
func TestRebuildFolderIndexFailureMarksStale(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`create trigger break_folder_names before insert on video_folder_names begin select raise(abort, 'broken'); end`); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err == nil {
		t.Fatal("rebuild succeeded with a broken table")
	}
	if groups := storedGroups(t, db); len(groups) != 1 {
		t.Fatalf("groups after failed rebuild = %+v", groups)
	}
	var stale int
	if err := db.sql.QueryRow(`select stale from folder_index_state where id = 1`).Scan(&stale); err != nil || stale != 1 {
		t.Fatalf("stale = %d, %v; want 1", stale, err)
	}
	if _, err := db.sql.Exec(`drop trigger break_folder_names`); err != nil {
		t.Fatal(err)
	}
	if rebuilt, err := db.ScanIndex().RefreshFolderIndex(ctx); err != nil || !rebuilt {
		t.Fatalf("refresh = %v, %v; want rebuild", rebuilt, err)
	}
}

func BenchmarkRebuildFolderIndex(b *testing.B) {
	db, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := Migrate(ctx, db); err != nil {
		b.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`, fixturePath("/media")); err != nil {
		b.Fatal(err)
	}
	tx, err := db.sql.Begin()
	if err != nil {
		b.Fatal(err)
	}
	const videos = 10000
	for i := range videos {
		// Distribute roughly three videos across each of 3,000 folders.
		path := fmt.Sprintf(fixturePath("/media/series-%02d/season-%03d/episode-%d.mp4"), i%30, (i/30)%100, i)
		res, err := tx.Exec(`insert into videos (added_at, indexed_at, content_key, playable, probe_state, thumbnail_state)
			values (1, 1, ?, 0, 'pending', 'pending')`, fmt.Sprintf("key-%d", i))
		if err != nil {
			b.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := tx.Exec(`insert into video_locations (video_id, path, version, title, size_bytes, mtime, created_at, updated_at)
			values (?, ?, 1, ?, 1, 1, 1, 1)`, id, path, path); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func TestFolderGroupings(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Settings().AddMediaFolder(ctx, fixturePath("/media")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fixturePath("/media/Show/a.mp4"), fixturePath("/media/Show/b.mp4"), fixturePath("/media/x.mp4")} {
		upsertFolderVideo(t, db, path, filepath.Base(path))
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	var showID int64
	if err := db.sql.QueryRow(`select id from groups where origin_folder = ?`, domain.FolderKey(fixturePath("/media/Show"))).Scan(&showID); err != nil {
		t.Fatal(err)
	}
	groups := db.FolderGroups()

	got, err := groups.FolderGroupings(ctx, []string{fixturePath("/media/Show/"), fixturePath("/media"), fixturePath("/media/Gone")})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.FolderGrouping{{GroupID: showID}, {}, {}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groupings = %+v, want %+v", got, want)
	}
	grouping, err := groups.SetFolderGrouping(ctx, fixturePath("/media"), domain.FolderGroupDirect)
	if err != nil || grouping != (domain.FolderGrouping{Mode: domain.FolderGroupDirect}) {
		t.Fatalf("groupDirect = %+v, %v", grouping, err)
	}
	if grouping, err := groups.SetFolderGrouping(ctx, fixturePath("/media"), ""); err != nil || grouping != (domain.FolderGrouping{}) {
		t.Fatalf("auto = %+v, %v", grouping, err)
	}
	if grouping, err := groups.SetFolderGrouping(ctx, fixturePath("/media/Show"), domain.FolderGroupUngroup); err != nil || grouping != (domain.FolderGrouping{Mode: domain.FolderGroupUngroup}) {
		t.Fatalf("ungroup = %+v, %v", grouping, err)
	}
}

// Rollback restores empty index tables and marks them stale for the older application's startup rebuild.
func TestDropFolderGroupsMigration(t *testing.T) {
	db, provider := migrateTo(t, 44)
	ctx := context.Background()
	execSQL(t, db, `insert into folder_index_state (id, version, search_version, stale) values (1, 3, 1, 0)`)
	execSQL(t, db, `insert into folder_groups (id, path_key, path, name, title_key) values (1, '/m/show', '/m/Show', 'Show', 'show')`)
	countTables := func() int {
		t.Helper()
		var n int
		if err := db.sql.QueryRow(`select count(*) from sqlite_master
			where name in ('folder_groups', 'folder_group_members')`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if _, err := provider.UpTo(ctx, 45); err != nil {
		t.Fatal(err)
	}
	if n := countTables(); n != 0 {
		t.Fatalf("Up のあとも %d 表が残った", n)
	}

	if _, err := provider.DownTo(ctx, 44); err != nil {
		t.Fatal(err)
	}
	if n := countTables(); n != 2 {
		t.Fatalf("Down のあとの表 = %d, want 2", n)
	}
	var rows, stale int
	if err := db.sql.QueryRow(`select count(*) from folder_groups`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("Down のあとの folder_groups = %d 行, %v; want 0", rows, err)
	}
	if err := db.sql.QueryRow(`select stale from folder_index_state where id = 1`).Scan(&stale); err != nil || stale != 1 {
		t.Fatalf("Down のあとの stale = %d, %v; want 1", stale, err)
	}
}
