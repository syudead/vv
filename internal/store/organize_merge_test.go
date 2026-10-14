package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// マージ（specs/050-drag-organize/data-model.md「Operations」の Merge、research.md R-5）の保存層の
// テスト。

func groupRef(id int64) domain.OrganizeRef {
	return domain.OrganizeRef{Kind: domain.OrganizeGroup, ID: id}
}
func videoRef(id int64) domain.OrganizeRef {
	return domain.OrganizeRef{Kind: domain.OrganizeVideo, ID: id}
}

// merge は items を into へマージし、取り消しの像を返す。
func merge(t *testing.T, db *DB, into int64, items ...domain.OrganizeRef) []byte {
	t.Helper()
	image, err := db.Organize().Merge(context.Background(), items, into)
	if err != nil {
		t.Fatalf("Merge(%v, %d) error = %v", items, into, err)
	}
	return image
}

func undo(t *testing.T, db *DB, image []byte) {
	t.Helper()
	if err := db.Organize().Undo(context.Background(), image); err != nil {
		t.Fatalf("Undo error = %v", err)
	}
}

// playlistWith はプレイリストを作り、add の動画、次にグループを項目にして、その id を返す。
func playlistWith(t *testing.T, db *DB, name string, add domain.PlaylistAdd) int64 {
	t.Helper()
	ctx := context.Background()
	playlist, err := db.Playlists().CreatePlaylist(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playlists().AddPlaylistItems(ctx, playlist.ID, add); err != nil {
		t.Fatal(err)
	}
	return playlist.ID
}

// playlistEntryIDs はプレイリスト id の項目を位置の順に "<項目 id>=v:<鍵>" か "<項目 id>=g:<グループの id>"
// で返す。
func playlistEntryIDs(t *testing.T, db *DB, id int64) []string {
	t.Helper()
	rows, err := queryStringsForTest(db, `select id || '=' || coalesce('v:' || content_key, 'g:' || group_id)
		from playlist_items where playlist_id = ? order by position`, id)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func groupFavorite(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	var favorite bool
	if err := db.sql.QueryRow(`select exists (select 1 from group_favorites where group_id = ?)`, id).Scan(&favorite); err != nil {
		t.Fatal(err)
	}
	return favorite
}

// organizeSnapshot はマージが書く表を、更新日時（取り消しも進める）を除いて 1 つの文字列にする。
func organizeSnapshot(t *testing.T, db *DB) string {
	t.Helper()
	var b strings.Builder
	for _, query := range []string{
		`select id || '|' || name || '|' || coalesce(origin_folder, '-') from groups order by id`,
		`select id || '|' || group_id || '|' || coalesce(content_key, '') || '|' || coalesce(child_id, '') || '|' ||
			position || '|' || from_folder from group_items order by id`,
		`select group_id || '|' || content_key from group_exclusions order by group_id, content_key`,
		`select group_id || '|' || favorited_at from group_favorites order by group_id`,
		`select id || '|' || playlist_id || '|' || coalesce(content_key, '') || '|' || coalesce(group_id, '') || '|' ||
			position || '|' || added_at from playlist_items order by id`,
		`select path || '|' || mode from folder_group_overrides order by path`,
	} {
		rows, err := queryStringsForTest(db, query)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(&b, rows)
	}
	return b.String()
}

// Y が自分の項目、続いて X の項目の順になり、X は消える（受け入れ条件 4）。お気に入りの X は Y を
// お気に入りにし、X のプレイリストの項目と親の項目は X の位置のまま Y のものになる。取り消すと X が
// id・項目・お気に入り・プレイリストの項目・親ごと戻り、Y も元のとおりになる。
func TestOrganizeMergeHandsOverAndUndoes(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ya, yb, xa, xb := looseVideo(t, db, "ya"), looseVideo(t, db, "yb"), looseVideo(t, db, "xa"), looseVideo(t, db, "xb")
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{ya, yb}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{xa, yb, xb}})
	q := createGroup(t, db, "Q", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "q1")}, GroupIDs: []int64{x}})
	if _, err := db.Groups().AddGroupItems(ctx, q, domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "q2")}}); err != nil {
		t.Fatal(err)
	}
	setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{x}, Favorite: true})
	playlist := playlistWith(t, db, "P", domain.PlaylistAdd{VideoIDs: []int64{ya}, GroupIDs: []int64{x}})
	playlistBefore := playlistEntryIDs(t, db, playlist)
	qItem := groupItemID(t, db, q, fmt.Sprintf("g:%d", x))
	before := organizeSnapshot(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	image := merge(t, db, y, groupRef(x))

	assertEntries(t, db, y, "v:ya", "v:yb", "v:xa", "v:xb")
	if groupExists(t, db, x) {
		t.Fatal("the merged group still exists")
	}
	if !groupFavorite(t, db, y) {
		t.Fatal("the target did not become a favorite")
	}
	assertEntries(t, db, q, "v:q1", fmt.Sprintf("g:%d", y), "v:q2")
	if got := groupItemID(t, db, q, fmt.Sprintf("g:%d", y)); got != qItem {
		t.Fatalf("parent item id = %d, want %d (replaced in place)", got, qItem)
	}
	wantPlaylist := []string{playlistBefore[0], strings.Replace(playlistBefore[1], fmt.Sprintf("g:%d", x), fmt.Sprintf("g:%d", y), 1)}
	if got := playlistEntryIDs(t, db, playlist); !slices.Equal(got, wantPlaylist) {
		t.Fatalf("playlist after merge = %v, want %v", got, wantPlaylist)
	}
	want := []domain.Event{domain.GroupsChanged{GroupIDs: normalizeIDs([]int64{x, y, q})}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("merge events = %+v, want %+v", recorder.events, want)
	}

	undo(t, db, image)
	if after := organizeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
	if groupFavorite(t, db, y) || !groupFavorite(t, db, x) {
		t.Fatal("undo did not put the favorite back on the merged group")
	}
}

// 親とプレイリストの計画の各行: すでに Y を持つ親、Y の子孫の親（Y 自身を含む）、両方を持つ
// プレイリストからは X の項目が消える。X の項目を消して空になった親は解散する。取り消しはそれも戻す。
func TestOrganizeMergeRemovesWhereTargetIsThere(t *testing.T) {
	db := migratedDB(t)
	ya, xa := looseVideo(t, db, "ya"), looseVideo(t, db, "xa")
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{xa}})
	emptied := createGroup(t, db, "Emptied", domain.GroupAdd{GroupIDs: []int64{x}})
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{ya}, GroupIDs: []int64{x, emptied}})
	both := createGroup(t, db, "Both", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "b1")}, GroupIDs: []int64{x, y}})
	playlist := playlistWith(t, db, "P", domain.PlaylistAdd{GroupIDs: []int64{y, x}})
	playlistBefore := playlistEntryIDs(t, db, playlist)
	before := organizeSnapshot(t, db)

	image := merge(t, db, y, groupRef(x))

	assertEntries(t, db, y, "v:ya", "v:xa")
	if groupExists(t, db, emptied) {
		t.Fatal("the parent emptied by the merge was kept")
	}
	assertEntries(t, db, both, "v:b1", fmt.Sprintf("g:%d", y))
	if got := playlistEntryIDs(t, db, playlist); !slices.Equal(got, playlistBefore[:1]) {
		t.Fatalf("playlist after merge = %v, want %v", got, playlistBefore[:1])
	}

	undo(t, db, image)
	if after := organizeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
}

// 動画と X をこの順で選んだマージは、1 つの取引で動画、続いて X の項目を足す。取り消しは 1 つの像で
// 両方を戻す。
func TestOrganizeMergeMixedSelection(t *testing.T) {
	db := migratedDB(t)
	ya, v, xa := looseVideo(t, db, "ya"), looseVideo(t, db, "v"), looseVideo(t, db, "xa")
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{ya}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{xa, v}})
	before := organizeSnapshot(t, db)

	image := merge(t, db, y, videoRef(v), groupRef(x), videoRef(v))
	assertEntries(t, db, y, "v:ya", "v:v", "v:xa")
	if groupExists(t, db, x) {
		t.Fatal("the merged group still exists")
	}

	undo(t, db, image)
	if after := organizeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
}

// マージした自動グループの X はそのフォルダに ungroup を書き、作り直しで戻らない。取り消しは X を
// 同じ id で戻し、ungroup を消す。
func TestOrganizeMergeAutomaticGroupWritesUngroup(t *testing.T) {
	db := migratedDB(t)
	show := fixturePath("/media/Show")
	upsertFolderVideo(t, db, fixturePath("/media/Show/1.mp4"), "k1")
	upsertFolderVideo(t, db, fixturePath("/media/Show/2.mp4"), "k2")
	rebuildIndex(t, db)
	auto := readGroups(t, db)[0].ID
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	before := organizeSnapshot(t, db)

	image := merge(t, db, y, groupRef(auto))
	assertEntries(t, db, y, "v:ya", "v:k1", "v:k2")
	if grouping := folderGroupingOf(t, db, show); grouping.Mode != domain.FolderGroupUngroup || grouping.GroupID != 0 {
		t.Fatalf("grouping after merge = %+v", grouping)
	}
	rebuildIndex(t, db)
	if groupExists(t, db, auto) || len(readGroups(t, db)) != 1 {
		t.Fatalf("groups after the rebuild = %+v", readGroups(t, db))
	}

	undo(t, db, image)
	if after := organizeSnapshot(t, db); after != before {
		t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
	}
	if grouping := folderGroupingOf(t, db, show); grouping.Mode != "" || grouping.GroupID != auto {
		t.Fatalf("grouping after undo = %+v", grouping)
	}
}

// ドラッグしたグループが Y そのものか Y の祖先なら ErrGroupCycle、Y・グループ・動画が無ければ
// ErrGroupTargetNotFound で、どれも何も書かない。
func TestOrganizeMergeRefusals(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	v := looseVideo(t, db, "v")
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{v}})
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{y}})
	grand := createGroup(t, db, "Grand", domain.GroupAdd{GroupIDs: []int64{parent}})
	other := createGroup(t, db, "Other", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "o")}})
	before := organizeSnapshot(t, db)
	missing := int64(999999)
	for name, c := range map[string]struct {
		into  int64
		items []domain.OrganizeRef
		want  error
	}{
		"自分自身":   {into: y, items: []domain.OrganizeRef{groupRef(other), groupRef(y)}, want: domain.ErrGroupCycle},
		"親":      {into: y, items: []domain.OrganizeRef{groupRef(parent)}, want: domain.ErrGroupCycle},
		"祖父":     {into: y, items: []domain.OrganizeRef{groupRef(other), groupRef(grand)}, want: domain.ErrGroupCycle},
		"無いマージ先": {into: missing, items: []domain.OrganizeRef{groupRef(other)}, want: domain.ErrGroupTargetNotFound},
		"無いグループ": {into: y, items: []domain.OrganizeRef{groupRef(other), groupRef(missing)}, want: domain.ErrGroupTargetNotFound},
		"無い動画":   {into: y, items: []domain.OrganizeRef{groupRef(other), videoRef(missing)}, want: domain.ErrGroupTargetNotFound},
	} {
		if _, err := db.Organize().Merge(ctx, c.items, c.into); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", name, err, c.want)
		}
	}
	if after := organizeSnapshot(t, db); after != before {
		t.Fatalf("refused merge wrote:\n%s\nwant\n%s", after, before)
	}
}

// マージの後で Y に動画を足すと、取り消しは ErrUndoConflict で何も変えない。
func TestOrganizeMergeUndoConflicts(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "xa")}})
	image := merge(t, db, y, groupRef(x))
	if _, err := db.Groups().AddGroupItems(ctx, y, domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "late")}}); err != nil {
		t.Fatal(err)
	}
	before := organizeSnapshot(t, db)
	if err := db.Organize().Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo error = %v, want ErrUndoConflict", err)
	}
	if after := organizeSnapshot(t, db); after != before {
		t.Fatalf("conflicting undo wrote:\n%s\nwant\n%s", after, before)
	}
}

// マージの後で、ドラッグしたグループの祖先ではない Y を改名すると、取り消しは ErrUndoConflict で
// 何も変えない（data-model.md「Undo image」: Merge は Move と同じく Y の groups の行を持つ）。
func TestOrganizeMergeUndoConflictsOnTargetRename(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	y := createGroup(t, db, "Y", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "ya")}})
	x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "xa")}})
	image := merge(t, db, y, groupRef(x))
	if _, err := db.Groups().RenameGroup(ctx, y, "Renamed"); err != nil {
		t.Fatal(err)
	}
	before := organizeSnapshot(t, db)
	if err := db.Organize().Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo error = %v, want ErrUndoConflict", err)
	}
	if after := organizeSnapshot(t, db); after != before {
		t.Fatalf("conflicting undo wrote:\n%s\nwant\n%s", after, before)
	}
}

// マージ先の Y は、マージで空になっても、空になった子の解散が Y へ届いても残る（要件 8 で消えるのは
// 落としたグループだけ）。取り消しは元のとおりに戻す。
func TestOrganizeMergeKeepsAnEmptiedTarget(t *testing.T) {
	for name, nested := range map[string]bool{"Y が X だけを持つ": false, "Y が X だけを持つ子だけを持つ": true} {
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			x := createGroup(t, db, "X", domain.GroupAdd{VideoIDs: []int64{looseVideo(t, db, "lone")}})
			holder := x
			if nested {
				holder = createGroup(t, db, "P", domain.GroupAdd{GroupIDs: []int64{x}})
			}
			y := createGroup(t, db, "Y", domain.GroupAdd{GroupIDs: []int64{holder}})
			// X を空にする（項目を取り除く操作は空のグループを解散するので、行を直接消す）。
			if _, err := db.sql.Exec(`delete from group_items where group_id = ?`, x); err != nil {
				t.Fatal(err)
			}
			before := organizeSnapshot(t, db)

			image := merge(t, db, y, groupRef(x))
			if !groupExists(t, db, y) {
				t.Fatal("the merge dissolved its target")
			}
			if groupExists(t, db, x) || (nested && groupExists(t, db, holder)) {
				t.Fatal("the merged group or the emptied parent was kept")
			}
			assertEntries(t, db, y)

			undo(t, db, image)
			if after := organizeSnapshot(t, db); after != before {
				t.Fatalf("rows after undo:\n%s\nwant\n%s", after, before)
			}
		})
	}
}
