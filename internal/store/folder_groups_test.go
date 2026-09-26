package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// storedGroup は索引の表に書かれたグループ1件である。
type storedGroup struct {
	Path    string
	Name    string
	Members []int64
}

// storedGroups は folder_groups と folder_group_members を path_key と並びの順に読む。
func storedGroups(t *testing.T, db *DB) []storedGroup {
	t.Helper()
	rows, err := db.sql.Query(`select g.id, g.path, g.name, g.title_key, m.video_id
		from folder_groups g left join folder_group_members m on m.group_id = g.id
		order by g.path_key, m.position`)
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

// storedFolderNames は video_folder_names を動画ごとに読む。
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

// スキャンを閉じる前の作り直しで、グループとフォルダ名の表が今の所在の形に置き換わる。
func TestRebuildFolderIndexAfterScan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	one := upsertFolderVideo(t, db, "/media/Show/1.mp4", "k1")
	ten := upsertFolderVideo(t, db, "/media/Show/10.mp4", "k10")
	two := upsertFolderVideo(t, db, "/media/Show/2.mp4", "k2")
	loose := upsertFolderVideo(t, db, "/media/loose.mp4", "loose")
	// 取り込みだけでは索引は変わらない。
	assertGroups(t, db, nil)

	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{{Path: "/media/Show", Name: "Show", Members: []int64{one, two, ten}}})
	if names := storedFolderNames(t, db); !reflect.DeepEqual(names, map[int64][]string{one: {"Show"}, two: {"Show"}, ten: {"Show"}}) {
		t.Fatalf("folder names = %v (loose=%d)", names, loose)
	}

	// 次のスキャンで子フォルダができると、そのフォルダはグループでなくなる。
	nested := upsertFolderVideo(t, db, "/media/Show/Extra/a.mp4", "extra")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, nil)
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

// メディアフォルダの追加と削除は、同じ取引で索引を作り直す。削除では、連鎖で
// 消える行だけでなく、残った動画の代表の所在が移ってできるグループも入る。
func TestRebuildFolderIndexOnMediaFolderChanges(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	// /aaa はまだ登録されていないので、その下の所在は索引に入らない。
	shared := upsertFolderVideo(t, db, "/aaa/c/1.mp4", "shared")
	onlyAAA := upsertFolderVideo(t, db, "/aaa/c/3.mp4", "only-aaa")
	upsertFolderVideo(t, db, "/media/b/1.mp4", "shared")
	onlyMedia := upsertFolderVideo(t, db, "/media/b/2.mp4", "only-media")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{{Path: "/media/b", Name: "b", Members: []int64{shared, onlyMedia}}})

	added, err := db.Settings().AddMediaFolder(ctx, "/aaa")
	if err != nil {
		t.Fatal(err)
	}
	// 共有する動画の代表の所在は /aaa/c/1.mp4 に移り、/media/b は1本になる。
	assertGroups(t, db, []storedGroup{{Path: "/aaa/c", Name: "c", Members: []int64{shared, onlyAAA}}})
	if names := storedFolderNames(t, db); !reflect.DeepEqual(names[shared], []string{"b", "c"}) {
		t.Fatalf("shared folder names = %v", names[shared])
	}

	if err := db.Settings().DeleteMediaFolder(ctx, added.ID, added.Version); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{{Path: "/media/b", Name: "b", Members: []int64{shared, onlyMedia}}})
	if names := storedFolderNames(t, db); !reflect.DeepEqual(names[shared], []string{"b"}) {
		t.Fatalf("shared folder names after delete = %v", names[shared])
	}
}

// 例外の設定と解除は同じ取引で索引を作り直し、例外は再スキャンと再オープンの後も残る。
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
	if _, err := db.Settings().AddMediaFolder(ctx, "/media"); err != nil {
		t.Fatal(err)
	}
	a := upsertFolderVideo(t, db, "/media/Show/a.mp4", "a")
	b := upsertFolderVideo(t, db, "/media/Show/b.mp4", "b")
	rootA := upsertFolderVideo(t, db, "/media/x.mp4", "x")
	rootB := upsertFolderVideo(t, db, "/media/y.mp4", "y")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	show := storedGroup{Path: "/media/Show", Name: "Show", Members: []int64{a, b}}
	root := storedGroup{Path: "/media", Name: "media", Members: []int64{rootA, rootB}}
	assertGroups(t, db, []storedGroup{show})

	groups := db.FolderGroups()
	if err := groups.SetOverride(ctx, "/media/Show/", domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, nil)
	// 同じフォルダの例外は置き換わり、2つが同時に付くことはない。
	if err := groups.SetOverride(ctx, "/media/Show", domain.FolderGroupDirect); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{show})
	if err := groups.SetOverride(ctx, "/media", domain.FolderGroupDirect); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{root, show})
	if err := groups.ClearOverride(ctx, "/media"); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, []storedGroup{show})
	if err := groups.SetOverride(ctx, "/media/Show", "both"); err == nil {
		t.Fatal("unknown mode was accepted")
	}
	// 一致するフォルダの無い例外も保存する。
	if err := groups.SetOverride(ctx, "/media/Gone", domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	if err := groups.SetOverride(ctx, "/media/Show", domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, nil)

	// 再スキャン（取り込みと作り直し）の後も例外は効く。
	c := upsertFolderVideo(t, db, "/media/Show/c.mp4", "c")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, nil)

	// 再オープンの後も例外は残り、索引は作り直さなくてよい。
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
	// 例外の付いていたフォルダが一度消えて戻っても、例外が効く。
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{a, b, c}); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	upsertFolderVideo(t, db, "/media/Show/a.mp4", "a")
	upsertFolderVideo(t, db, "/media/Show/b.mp4", "b")
	if err := db.FolderGroups().ClearOverride(ctx, "/media/Nothing"); err != nil {
		t.Fatal(err)
	}
	assertGroups(t, db, nil)
}

// 起動時の作り直しは、行が無いとき・版が違うとき・前回が失敗していたときだけ行う。
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

// 作り直しに失敗したら、その取引は巻き戻り、別の取引で索引を古いと記録する。
func TestRebuildFolderIndexFailureMarksStale(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertFolderVideo(t, db, "/media/Show/1.mp4", "k1")
	upsertFolderVideo(t, db, "/media/Show/2.mp4", "k2")
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`create trigger break_folder_names before insert on video_folder_names begin select raise(abort, 'broken'); end`); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err == nil {
		t.Fatal("rebuild succeeded with a broken table")
	}
	// 前の索引は残る。
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

// BenchmarkRebuildFolderIndex は1万本・約3千フォルダの作り直しにかかる時間を測る
// （specs/017-folder-groups/plan.md の Technical Context、目安は1秒未満）。
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
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values ('/media', 1, 1, 1)`); err != nil {
		b.Fatal(err)
	}
	tx, err := db.sql.Begin()
	if err != nil {
		b.Fatal(err)
	}
	const videos = 10000
	for i := range videos {
		// 30 × 100 = 3000 フォルダに、1フォルダあたり約3本を置く。
		path := fmt.Sprintf("/media/series-%02d/season-%03d/episode-%d.mp4", i%30, (i/30)%100, i)
		res, err := tx.Exec(`insert into videos (added_at, updated_at, content_key, playable, probe_state, thumbnail_state)
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

// まとめ方の読み出しと、グループのタグ化（specs/017-folder-groups/data-model.md §4）。
// タグ化はグループでないフォルダと、タグ名に使えないフォルダ名では何も書かない。
func TestFolderGroupingsAndTagFolderGroup(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Settings().AddMediaFolder(ctx, "/media"); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", domain.TagNameMaxLength+1)
	for _, path := range []string{"/media/Show/a.mp4", "/media/Show/b.mp4", "/media/" + long + "/c.mp4", "/media/" + long + "/d.mp4", "/media/x.mp4"} {
		upsertFolderVideo(t, db, path, filepath.Base(path))
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	groups := db.FolderGroups()

	got, err := groups.FolderGroupings(ctx, []string{"/media/Show/", "/media", "/media/Gone"})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.FolderGrouping{{Grouped: true}, {}, {}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groupings = %+v, want %+v", got, want)
	}
	grouping, err := groups.SetFolderGrouping(ctx, "/media", domain.FolderGroupDirect)
	if err != nil || grouping != (domain.FolderGrouping{Mode: domain.FolderGroupDirect, Grouped: false}) {
		// /media の直下は x だけなので、1本ではグループにならない。
		t.Fatalf("groupDirect = %+v, %v", grouping, err)
	}
	if grouping, err := groups.SetFolderGrouping(ctx, "/media", ""); err != nil || grouping != (domain.FolderGrouping{}) {
		t.Fatalf("auto = %+v, %v", grouping, err)
	}

	countRows := func(table string) int {
		t.Helper()
		var n int
		if err := db.sql.QueryRow(`select count(*) from ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := groups.TagFolderGroup(ctx, "/media/"+long); !errors.Is(err, domain.ErrInvalidTagName) {
		t.Fatalf("タグ名に使えない名前: err = %v", err)
	}
	if _, err := groups.TagFolderGroup(ctx, "/media/Gone"); !errors.Is(err, domain.ErrNotFolderGroup) {
		t.Fatalf("グループでないフォルダ: err = %v", err)
	}
	if tags, overrides := countRows("tags"), countRows("folder_group_overrides"); tags != 0 || overrides != 0 {
		t.Fatalf("失敗したタグ化で書かれた: tags %d, overrides %d", tags, overrides)
	}

	result, err := groups.TagFolderGroup(ctx, "/media/Show")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Tag.Name != "Show" || result.Grouping != (domain.FolderGrouping{Mode: domain.FolderGroupUngroup}) {
		t.Fatalf("result = %+v", result)
	}
	if remaining := storedGroups(t, db); len(remaining) != 1 || remaining[0].Name != long {
		t.Fatalf("タグ化の後のグループ = %+v, want %s だけ", remaining, long)
	}
	if _, err := groups.TagFolderGroup(ctx, "/media/Show"); !errors.Is(err, domain.ErrNotFolderGroup) {
		t.Fatalf("2回目: err = %v", err)
	}
}
