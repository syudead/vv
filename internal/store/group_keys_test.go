package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// お気に入りとプレイリストの項目をグループの id で名指す（specs/048-groups/research.md R-10、
// data-model.md「Migration」「Play order」）。

// 受け入れ条件 8: 手で作ったグループをお気に入りにでき、お気に入りの絞り込みに並び、プレイリストの
// 項目として並び、その再生順（深さ優先）で再生される。解散するとお気に入りとプレイリストの項目が
// 消え、プレイリストは詰め直されて更新日時が進む。
func TestHandMadeGroupIsFavoritedAndPlayedFromAPlaylist(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	x, solo, y := ids[fixturePath("/media/mixed/x.mp4")], ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/sub/y.mp4")]
	ep1 := ids[fixturePath("/media/show/ep1.mp4")]
	inner := createGroup(t, db, "inner", domain.GroupAdd{VideoIDs: []int64{x, solo}})
	outer := createGroup(t, db, "outer", domain.GroupAdd{VideoIDs: []int64{y}, GroupIDs: []int64{inner}})

	if applied := setFavorites(t, db, domain.FavoriteChange{GroupIDs: []int64{outer, 999_999}, Favorite: true}); applied != (domain.FavoriteApplied{Groups: 1}) {
		t.Fatalf("applied = %+v, want {0 1}", applied)
	}
	if names := ownerLibraryNames(t, db, domain.VideoQuery{FavoriteOnly: true}); !slices.Equal(names, []string{"group:outer"}) {
		t.Errorf("お気に入りだけ = %v, want [group:outer]", names)
	}

	id := createPlaylist(t, db, "手で作った")
	if added := addToPlaylist(t, db, id, domain.PlaylistAdd{GroupIDs: []int64{outer}}); added != (domain.PlaylistAdded{Added: 1}) {
		t.Fatalf("added = %+v", added)
	}
	addVideos(t, db, id, ep1)
	if got, want := playlistEntries(t, db, id), []string{"g:#" + itoa(outer), "v:" + itoa(ep1)}; !slices.Equal(got, want) {
		t.Fatalf("項目 = %v, want %v", got, want)
	}
	_, items := readPlaylist(t, db, id)
	if got, want := memberIDs(*items[0].Group), []int64{y, x, solo}; !slices.Equal(got, want) {
		t.Errorf("グループの項目のメンバー = %v, want 再生順 %v", got, want)
	}
	groupItem, videoItem := items[0].ID, items[1].ID
	for _, step := range []struct{ from, want [2]int64 }{
		{[2]int64{groupItem, y}, [2]int64{groupItem, x}},
		{[2]int64{groupItem, x}, [2]int64{groupItem, solo}},
		{[2]int64{groupItem, solo}, [2]int64{videoItem, ep1}},
	} {
		playback, err := db.Playlists().PlaylistPlayback(ctx, id, step.from[0], step.from[1])
		if err != nil {
			t.Fatal(err)
		}
		if got := stepOf(playback.Next); got != step.want {
			t.Errorf("%v の次 = %v, want %v", step.from, got, step.want)
		}
	}

	before, _ := readPlaylist(t, db, id)
	if err := db.Groups().DissolveGroup(ctx, outer); err != nil {
		t.Fatal(err)
	}
	if n := favoriteRowCount(t, db, "group_favorites"); n != 0 {
		t.Errorf("解散のあとの group_favorites = %d 行, want 0", n)
	}
	if got := playlistEntries(t, db, id); !slices.Equal(got, videoEntries(ep1)) {
		t.Errorf("解散のあとの項目 = %v, want [v:%d]", got, ep1)
	}
	var position int
	if err := db.sql.QueryRow(`select position from playlist_items where playlist_id = ?`, id).Scan(&position); err != nil || position != 0 {
		t.Errorf("残った項目の位置 = %d (%v), want 0", position, err)
	}
	if after, _ := readPlaylist(t, db, id); !after.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("解散のあとの更新日時 %v が前の %v より進まない", after.UpdatedAt, before.UpdatedAt)
	}
}

// 移行は今のフォルダグループのお気に入りとプレイリストのグループの項目をそのグループのものにし、
// グループでないフォルダのものは落としてプレイリストを詰め直す。Down は folder_favorites を空で戻し、
// 自動グループの項目をフォルダの鍵へ戻す。
func TestGroupKeysMigrationConvertsFolderKeys(t *testing.T) {
	db, provider := migrateTo(t, 42)
	ctx := context.Background()
	execSQL(t, db, `insert into groups (id, name, title_key, origin_folder, created_at, updated_at) values
		(7, 'Show', ?, '/m/show', 1, 1), (8, 'Mine', ?, null, 1, 1)`,
		domain.NaturalSortKey("Show"), domain.NaturalSortKey("Mine"))
	execSQL(t, db, `insert into folder_favorites (path, favorited_at) values ('/m/show', 50), ('/m/gone', 60)`)
	execSQL(t, db, `insert into playlists (id, name, created_at, updated_at) values (1, 'P', 1, 1)`)
	execSQL(t, db, `insert into playlist_items (id, playlist_id, content_key, folder_key, position, added_at) values
		(1, 1, 'key-a', null, 0, 1), (2, 1, null, '/m/gone', 1, 1), (3, 1, null, '/m/show', 2, 1), (4, 1, 'key-b', null, 3, 1)`)

	if _, err := provider.UpTo(ctx, 43); err != nil {
		t.Fatal(err)
	}
	favorites, err := queryStringsForTest(db, `select group_id || ':' || favorited_at from group_favorites order by group_id`)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(favorites, []string{"7:50"}) {
		t.Errorf("group_favorites = %v, want [7:50]", favorites)
	}
	items, err := queryStringsForTest(db, `select id || ':' || coalesce(content_key, 'g' || group_id) || ':' || position
		from playlist_items order by position`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1:key-a:0", "3:g7:1", "4:key-b:2"}; !slices.Equal(items, want) {
		t.Errorf("playlist_items = %v, want %v", items, want)
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = 'folder_favorites'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("folder_favorites が残った")
	}
	assertPlaylistInvariant(t, db)

	// 手で作ったグループの項目は Down で落ちる。
	execSQL(t, db, `insert into playlist_items (playlist_id, group_id, position, added_at) values (1, 8, 3, 1)`)
	if _, err := provider.DownTo(ctx, 42); err != nil {
		t.Fatal(err)
	}
	items, err = queryStringsForTest(db, `select id || ':' || coalesce(content_key, folder_key) || ':' || position
		from playlist_items order by position`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1:key-a:0", "3:/m/show:1", "4:key-b:2"}; !slices.Equal(items, want) {
		t.Errorf("Down のあとの playlist_items = %v, want %v", items, want)
	}
	if n := favoriteRowCount(t, db, "folder_favorites"); n != 0 {
		t.Errorf("Down のあとの folder_favorites = %d 行, want 0", n)
	}
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = 'group_favorites'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("Down のあとも group_favorites が残った")
	}
}
