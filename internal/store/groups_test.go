package store

import (
	"context"
	"database/sql"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// groupItem はグループの動画の項目1件である。Folder は from_folder = 1 かどうか。
type groupItem struct {
	Key    string
	Folder bool
}

// groupRow はグループ1つと、その項目を位置の順に持つ。
type groupRow struct {
	ID     int64
	Name   string
	Origin string
	Items  []groupItem
}

// readGroups は groups と動画の項目を id と位置の順に読む。
func readGroups(t *testing.T, db *DB) []groupRow {
	t.Helper()
	rows, err := db.sql.Query(`select g.id, g.name, g.title_key, coalesce(g.origin_folder, ''), i.content_key, i.from_folder
		from groups g left join group_items i on i.group_id = g.id and i.content_key is not null
		order by g.id, i.position`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []groupRow{}
	for rows.Next() {
		var row groupRow
		var titleKey string
		var key sql.NullString
		var folder sql.NullBool
		if err := rows.Scan(&row.ID, &row.Name, &titleKey, &row.Origin, &key, &folder); err != nil {
			t.Fatal(err)
		}
		if titleKey != domain.NaturalSortKey(row.Name) {
			t.Errorf("title_key of %q = %q", row.Name, titleKey)
		}
		if len(out) == 0 || out[len(out)-1].ID != row.ID {
			row.Items = []groupItem{}
			out = append(out, row)
		}
		if key.Valid {
			last := &out[len(out)-1]
			last.Items = append(last.Items, groupItem{Key: key.String, Folder: folder.Bool})
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertGroupRows(t *testing.T, db *DB, want []groupRow) {
	t.Helper()
	if want == nil {
		want = []groupRow{}
	}
	if got := readGroups(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %+v\nwant %+v", got, want)
	}
}

func folderItems(keys ...string) []groupItem {
	items := make([]groupItem, len(keys))
	for i, key := range keys {
		items[i] = groupItem{Key: key, Folder: true}
	}
	return items
}

func groupExclusions(t *testing.T, db *DB, groupID int64) []string {
	t.Helper()
	keys, err := queryStringsForTest(db, `select content_key from group_exclusions where group_id = ? order by content_key`, groupID)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func queryStringsForTest(db *DB, query string, args ...any) ([]string, error) {
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func rebuildIndex(t *testing.T, db *DB) {
	t.Helper()
	if err := db.ScanIndex().RebuildFolderIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// moveLocation は所在 from を to へ移す（to に所在を足し、from の所在を消す）。
func moveLocation(t *testing.T, db *DB, from, to, key string) {
	t.Helper()
	id := upsertFolderVideo(t, db, to, key)
	removeLocation(t, db, id, from)
}

// 移行は今のフォルダグループごとに、その名前、利用者データの鍵のメンバー、その順番を持つ
// グループを 1 つ作る。Down は表を落とす。
func TestGroupsMigrationSeedsFromFolderGroups(t *testing.T) {
	db, provider := migrateTo(t, 41)
	ctx := context.Background()
	for _, key := range []string{"key-a", "key-b", "key-c", "", "key-d"} {
		execSQL(t, db, `insert into videos(content_key, probe_state, thumbnail_state, preview_state, seek_thumbnail_state)
			values (?, 'done', 'done', 'done', 'done')`, key)
	}
	execSQL(t, db, `insert into video_bundles (id, user_key, representative_key, created_at, updated_at) values (1, 'bundle:1', 'key-c', 1, 1)`)
	execSQL(t, db, `insert into video_bundle_members (content_key, bundle_id, added_at) values ('key-c', 1, 1)`)
	execSQL(t, db, `insert into folder_groups (id, path_key, path, name, title_key) values
		(10, '/m/show', '/m/Show', 'Show', ?), (20, '/m/other', '/m/Other', 'Other 2', ?)`,
		domain.NaturalSortKey("Show"), domain.NaturalSortKey("Other 2"))
	// 動画 4 は content_key が空なので写さず、位置は詰め直す。
	execSQL(t, db, `insert into folder_group_members (video_id, group_id, position) values
		(2, 10, 0), (4, 10, 1), (3, 10, 2), (1, 10, 3), (5, 20, 0)`)

	if _, err := provider.UpTo(ctx, 42); err != nil {
		t.Fatal(err)
	}
	groups := readGroups(t, db)
	want := []groupRow{
		{ID: groups[0].ID, Name: "Show", Origin: "/m/show", Items: folderItems("key-b", "bundle:1", "key-a")},
		{ID: groups[1].ID, Name: "Other 2", Origin: "/m/other", Items: folderItems("key-d")},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups = %+v\nwant %+v", groups, want)
	}
	var created, updated int64
	if err := db.sql.QueryRow(`select min(created_at), min(updated_at) from groups`).Scan(&created, &updated); err != nil {
		t.Fatal(err)
	}
	if created < 1_600_000_000_000 || updated != created {
		t.Errorf("created_at = %d, updated_at = %d; want the migration time in Unix ms", created, updated)
	}
	assertGroupPositions(t, db)

	if _, err := provider.DownTo(ctx, 41); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master
		where name in ('groups', 'group_items', 'group_exclusions')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("Down のあとも %d 表が残った", tables)
	}
}

// 規則がグループにする新しいフォルダにファイル名順のグループができ、フォルダに足したファイルは
// 末尾に足され、フォルダの外へ移したファイルは外れる。直下の動画が 1 本になってもグループは残る。
func TestSyncFolderGroupsFollowsTheFolder(t *testing.T) {
	db := migratedDB(t)
	show := fixturePath("/media/Show")
	upsertFolderVideo(t, db, fixturePath("/media/Show/10.mp4"), "k10")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	upsertFolderVideo(t, db, fixturePath("/media/loose.mp4"), "loose")
	rebuildIndex(t, db)
	groups := readGroups(t, db)
	if len(groups) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	id := groups[0].ID
	origin := domain.FolderKey(show)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: folderItems("k2", "k10")}})

	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	rebuildIndex(t, db)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: folderItems("k2", "k10", "k1")}})

	moveLocation(t, db, fixturePath("/media/Show/2.mp4"), fixturePath("/media/Elsewhere/2.mp4"), "k2")
	moveLocation(t, db, fixturePath("/media/Show/10.mp4"), fixturePath("/media/Elsewhere/10.mp4"), "k10")
	rebuildIndex(t, db)
	// /media/Elsewhere は新しくグループになり、/media/Show は 1 本でもグループのまま残る。
	groups = readGroups(t, db)
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	assertGroupRows(t, db, []groupRow{
		{ID: id, Name: "Show", Origin: origin, Items: folderItems("k1")},
		{ID: groups[1].ID, Name: "Elsewhere", Origin: domain.FolderKey(fixturePath("/media/Elsewhere")), Items: folderItems("k2", "k10")},
	})
}

// 手で足したメンバー、外したメンバー（除外）、新しい名前、新しい順番は再構築の後も残り、
// フォルダに新しく置いたファイルは足される（受け入れ条件 5）。
func TestSyncFolderGroupsKeepsOwnerEdits(t *testing.T) {
	db := migratedDB(t)
	k1 := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	upsertFolderVideo(t, db, fixturePath("/media/Show/3.mp4"), "k3")
	other := upsertFolderVideo(t, db, fixturePath("/media/other.mp4"), "other")
	rebuildIndex(t, db)
	id := readGroups(t, db)[0].ID

	// 所有者の編集: 名前を変え、k3 を先頭へ、k2 を外し（除外）、別のフォルダの動画を手で足す。
	execSQL(t, db, `update groups set name = 'Renamed', title_key = ? where id = ?`, domain.NaturalSortKey("Renamed"), id)
	execSQL(t, db, `delete from group_items where group_id = ? and content_key = 'k2'`, id)
	execSQL(t, db, `insert into group_exclusions (group_id, content_key) values (?, 'k2')`, id)
	execSQL(t, db, `update group_items set position = case content_key when 'k3' then 0 else 1 end where group_id = ?`, id)
	execSQL(t, db, `insert into group_items (group_id, content_key, position, from_folder) values (?, 'other', 2, 0)`, id)

	upsertFolderVideo(t, db, fixturePath("/media/Show/4.mp4"), "k4")
	rebuildIndex(t, db)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Renamed", Origin: domain.FolderKey(fixturePath("/media/Show")), Items: []groupItem{
		{Key: "k3", Folder: true}, {Key: "k1", Folder: true}, {Key: "other"}, {Key: "k4", Folder: true},
	}}})
	if got := groupExclusions(t, db, id); !slices.Equal(got, []string{"k2"}) {
		t.Fatalf("exclusions = %v", got)
	}

	// 手で足したメンバーはフォルダから消えても残る。
	removeLocation(t, db, k1, fixturePath("/media/Show/1.mp4"))
	removeLocation(t, db, other, fixturePath("/media/other.mp4"))
	rebuildIndex(t, db)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Renamed", Origin: domain.FolderKey(fixturePath("/media/Show")), Items: []groupItem{
		{Key: "k3", Folder: true}, {Key: "other"}, {Key: "k4", Folder: true},
	}}})
}

// メディアフォルダを外すとグループが空になり、足し直すと同じグループに戻る。ungroup の
// フォルダにはグループができない。
func TestSyncFolderGroupsOnMediaFolderChangesAndUngroup(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	added, err := db.Settings().AddMediaFolder(ctx, fixturePath("/extra"))
	if err != nil {
		t.Fatal(err)
	}
	upsertFolderVideo(t, db, fixturePath("/extra/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/extra/Show/2.mp4"), "k2")
	if err := db.FolderGroups().SetOverride(ctx, fixturePath("/media/Skip"), domain.FolderGroupUngroup); err != nil {
		t.Fatal(err)
	}
	upsertFolderVideo(t, db, fixturePath("/media/Skip/1.mp4"), "s1")
	upsertFolderVideo(t, db, fixturePath("/media/Skip/2.mp4"), "s2")
	rebuildIndex(t, db)
	groups := readGroups(t, db)
	origin := domain.FolderKey(fixturePath("/extra/Show"))
	if len(groups) != 1 || groups[0].Origin != origin {
		t.Fatalf("groups = %+v; want only /extra/Show", groups)
	}
	id := groups[0].ID

	if err := db.Settings().DeleteMediaFolder(ctx, added.ID, added.Version); err != nil {
		t.Fatal(err)
	}
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: []groupItem{}}})

	if _, err := db.Settings().AddMediaFolder(ctx, fixturePath("/extra")); err != nil {
		t.Fatal(err)
	}
	upsertFolderVideo(t, db, fixturePath("/extra/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/extra/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: folderItems("k1", "k2")}})
}

// 束ねで 1 つのグループの中の 2 つのバージョンが 1 つの項目にまとまり、手で足した項目なら
// まとまった項目も手で足した項目になる。除外も集まりの鍵へ付け替わる。
func TestBundlingRekeysGroupItems(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "ka")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "kx")
	b := upsertFolderVideo(t, db, fixturePath("/media/Show/3.mp4"), "kb")
	c := upsertFolderVideo(t, db, fixturePath("/media/Other/c.mp4"), "kc")
	d := upsertFolderVideo(t, db, fixturePath("/media/Other/d.mp4"), "kd")
	rebuildIndex(t, db)
	groups := readGroups(t, db)
	show, other := groups[1].ID, groups[0].ID
	if groups[1].Name != "Show" {
		show, other = other, show
	}
	// Show の kb を手で足した項目にし、Other の kd を外す（除外）。
	execSQL(t, db, `update group_items set from_folder = 0 where group_id = ? and content_key = 'kb'`, show)
	execSQL(t, db, `delete from group_items where group_id = ? and content_key = 'kd'`, other)
	execSQL(t, db, `insert into group_exclusions (group_id, content_key) values (?, 'kd')`, other)
	if err := renumberPositionsForTest(db, other); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Versions().Bundle(ctx, []int64{a, b}, a); err != nil {
		t.Fatal(err)
	}
	bundleKey := ownerVideo(t, db, a).UserKey
	if _, err := db.Versions().Bundle(ctx, []int64{c, d}, c); err != nil {
		t.Fatal(err)
	}
	otherKey := ownerVideo(t, db, c).UserKey
	got := readGroups(t, db)
	want := map[int64][]groupItem{
		show:  {{Key: bundleKey}, {Key: "kx", Folder: true}},
		other: {{Key: otherKey, Folder: true}},
	}
	for _, group := range got {
		if !reflect.DeepEqual(group.Items, want[group.ID]) {
			t.Errorf("group %s items = %+v, want %+v", group.Name, group.Items, want[group.ID])
		}
	}
	// 除外の kd は集まりの鍵へ付け替わるが、その鍵の項目が見えているので消える。
	if got := groupExclusions(t, db, other); len(got) != 0 {
		t.Errorf("other exclusions = %v, want none", got)
	}
	// 再構築しても重ならない。
	rebuildIndex(t, db)
	if again := readGroups(t, db); !reflect.DeepEqual(again, got) {
		t.Fatalf("after rebuild = %+v\nwant %+v", again, got)
	}
}

func renumberPositionsForTest(db *DB, groupID int64) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := renumberPositions(context.Background(), tx, groupPositions, []int64{groupID}); err != nil {
		return err
	}
	return tx.Commit()
}

// 除外だけを持つ鍵も、束ねで集まりの鍵へ付け替わる。
func TestBundlingRekeysGroupExclusions(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a := upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "ka")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "kx")
	b := upsertFolderVideo(t, db, fixturePath("/media/loose.mp4"), "kb")
	rebuildIndex(t, db)
	id := readGroups(t, db)[0].ID
	execSQL(t, db, `delete from group_items where group_id = ? and content_key = 'ka'`, id)
	execSQL(t, db, `insert into group_exclusions (group_id, content_key) values (?, 'ka')`, id)
	if err := renumberPositionsForTest(db, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Versions().Bundle(ctx, []int64{a, b}, a); err != nil {
		t.Fatal(err)
	}
	bundleKey := ownerVideo(t, db, a).UserKey
	if got := groupExclusions(t, db, id); !slices.Equal(got, []string{bundleKey}) {
		t.Fatalf("exclusions = %v, want [%s]", got, bundleKey)
	}
	// 集まりの代表はフォルダの直下にあるが、除外したので足さない。
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: domain.FolderKey(fixturePath("/media/Show")), Items: folderItems("kx")}})
}

// 同じパスの継承で、項目は新しい中身の鍵へ付け替わる。
func TestSuccessionRekeysGroupItems(t *testing.T) {
	db := migratedDB(t)
	a := upsertOne(t, db, listingFile(fixturePath("/media/Show/a.mp4"), "a", "key-a", 1))
	upsertOne(t, db, listingFile(fixturePath("/media/Show/b.mp4"), "b", "key-x", 2))
	probeDuration(t, db, a, 100_000)
	rebuildIndex(t, db)
	id := readGroups(t, db)[0].ID

	scan := startTestScan(t, db)
	next := upsertOne(t, db, listingFile(fixturePath("/media/Show/a.mp4"), "a", "key-b", 3))
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, next, 100_000)
	origin := domain.FolderKey(fixturePath("/media/Show"))
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: folderItems("key-b", "key-x")}})
	rebuildIndex(t, db)
	assertGroupRows(t, db, []groupRow{{ID: id, Name: "Show", Origin: origin, Items: folderItems("key-b", "key-x")}})
}

// 同期で変わったグループとその祖先の id を、確定後に 1 つの GroupsChanged で知らせる。
// 変わらない作り直しは知らせない。
func TestSyncFolderGroupsPublishesGroupsChanged(t *testing.T) {
	db := migratedDB(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	recorder.events = nil
	rebuildIndex(t, db)
	id := readGroups(t, db)[0].ID
	if want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{id}}}; !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %+v, want %+v", recorder.events, want)
	}

	recorder.events = nil
	rebuildIndex(t, db)
	if len(recorder.events) != 0 {
		t.Fatalf("unchanged rebuild published %+v", recorder.events)
	}

	// 手で作った親グループ（子に Show を持つ）と、その親も祖先として入る。
	execSQL(t, db, `insert into groups (id, name, title_key, created_at, updated_at) values (100, 'Parent', 'parent', 1, 1), (200, 'Top', 'top', 1, 1)`)
	execSQL(t, db, `insert into group_items (group_id, child_id, position) values (100, ?, 0), (200, 100, 0)`, id)
	upsertFolderVideo(t, db, fixturePath("/media/Show/3.mp4"), "k3")
	recorder.events = nil
	rebuildIndex(t, db)
	if want := []domain.Event{domain.GroupsChanged{GroupIDs: []int64{id, 100, 200}}}; !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %+v, want %+v", recorder.events, want)
	}
}
