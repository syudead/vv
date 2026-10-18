package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func smartFolderCard(t *testing.T, db *DB, id int64) domain.LibrarySmartFolder {
	t.Helper()
	page, err := db.SmartFolders().SmartFolderContents(context.Background(), domain.AudienceOwner, id, "", domain.MaxLimit)
	if err != nil {
		t.Fatalf("SmartFolderContents(%d) error = %v", id, err)
	}
	return page.SmartFolder
}

func TestSmartFolderFavoriteShowsOnItsCard(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	cat := attachNamedTag(t, db, "猫", ids[fixturePath("/media/solo.mp4")])
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)

	applied, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{SmartFolderIDs: []int64{folder, folder, 999}, Favorite: true})
	if err != nil {
		t.Fatal(err)
	}
	if applied != (domain.FavoriteApplied{SmartFolders: 1}) {
		t.Errorf("付けた数 = %+v, want SmartFolders 1", applied)
	}
	if !smartFolderCard(t, db, folder).Favorite {
		t.Error("お気に入りにしたフォルダのカードがお気に入りでない")
	}

	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{SmartFolderIDs: []int64{folder}}); err != nil {
		t.Fatal(err)
	}
	if smartFolderCard(t, db, folder).Favorite {
		t.Error("外したフォルダのカードがお気に入りのまま")
	}
}

func TestPlaylistSmartFolderItemPlaysTheFolderAtPlayTime(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo, x, y := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")],
		ids[fixturePath("/media/mixed/sub/y.mp4")]
	cat := attachNamedTag(t, db, "猫", solo)
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	playlist := createPlaylist(t, db, "猫を流す")
	added := addToPlaylist(t, db, playlist, domain.PlaylistAdd{SmartFolderIDs: []int64{folder, 999}})
	if added != (domain.PlaylistAdded{Added: 1}) {
		t.Fatalf("足した数 = %+v, want Added 1", added)
	}
	if again := addToPlaylist(t, db, playlist, domain.PlaylistAdd{SmartFolderIDs: []int64{folder}}); again != (domain.PlaylistAdded{Existing: 1}) {
		t.Fatalf("2 度目に足した数 = %+v, want Existing 1", again)
	}
	addVideos(t, db, playlist, y)

	summary, items := readPlaylist(t, db, playlist)
	if summary.ItemCount != 2 || len(items) != 2 || items[0].SmartFolder == nil || items[1].Video == nil {
		t.Fatalf("項目 = %+v（数 %d）", items, summary.ItemCount)
	}
	itemF := items[0].ID
	if card := items[0].SmartFolder; card.ID != folder || card.Name != "猫" ||
		!slices.Equal(memberIDs(card.LibraryGroup), []int64{solo}) {
		t.Errorf("フォルダの項目 = %+v", card)
	}
	playback, err := db.Playlists().PlaylistPlayback(ctx, playlist, itemF, solo)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepOf(playback.Next); got != [2]int64{items[1].ID, y} {
		t.Errorf("付ける前の solo の次 = %v, want 次の項目の y", got)
	}

	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{x}, cat); err != nil {
		t.Fatal(err)
	}
	playback, err = db.Playlists().PlaylistPlayback(ctx, playlist, itemF, solo)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepOf(playback.Next); got != [2]int64{itemF, x} {
		t.Errorf("付けた後の solo の次 = %v, want フォルダの x", got)
	}
	if playback.Position != 1 || playback.Count != 2 {
		t.Errorf("位置 = %d / %d, want 1 / 2", playback.Position, playback.Count)
	}
}

func TestVideoContextFollowsTheSmartFolderOrTheGroup(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	p1, p2 := ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")]
	solo, x := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/x.mp4")]
	pair := groupIDOf(t, db, fixturePath("/media/pair"))
	cat := attachNamedTag(t, db, "猫", p1, solo)
	dog := attachNamedTag(t, db, "犬", x)
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	other := newSmartFolder(t, db, "犬", domain.SmartRule{Tags: []int64{dog}}, domain.SortTitleAsc, 0)
	gone := attachNamedTag(t, db, "消す", p1)
	missing := newSmartFolder(t, db, "消す", domain.SmartRule{Tags: []int64{gone}}, domain.SortTitleAsc, 0)
	if err := db.Tags().DeleteTag(ctx, gone); err != nil {
		t.Fatal(err)
	}

	read := func(id int64, opened domain.VideoContext) (domain.VideoGroup, bool) {
		t.Helper()
		group, found, err := db.Library().VideoGroup(ctx, domain.AudienceOwner, id, opened, domain.GroupWindow{Limit: 10})
		if err != nil {
			t.Fatalf("VideoGroup(%d, %+v) error = %v", id, opened, err)
		}
		return group, found
	}
	type want struct {
		smartFolder bool
		id          int64
		members     []int64
		prev, next  int64
	}
	for _, tc := range []struct {
		name   string
		video  int64
		opened domain.VideoContext
		want   want
	}{
		{"smartFolder はフォルダの再生順", p1, domain.VideoContext{SmartFolderID: folder},
			want{true, folder, []int64{p1, solo}, 0, solo}},
		{"group はグループの再生順", p1, domain.VideoContext{GroupID: pair}, want{false, pair, []int64{p1, p2}, 0, p2}},
		{"両方なら group", p1, domain.VideoContext{GroupID: pair, SmartFolderID: folder},
			want{false, pair, []int64{p1, p2}, 0, p2}},
		{"動画を持たないフォルダは無視", p1, domain.VideoContext{SmartFolderID: other},
			want{false, pair, []int64{p1, p2}, 0, p2}},
		{"無いフォルダは無視", p1, domain.VideoContext{SmartFolderID: 999}, want{false, pair, []int64{p1, p2}, 0, p2}},
		{"無いタグを名指すフォルダは無視", p1, domain.VideoContext{SmartFolderID: missing},
			want{false, pair, []int64{p1, p2}, 0, p2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			group, found := read(tc.video, tc.opened)
			if !found {
				t.Fatal("文脈が無い")
			}
			prev, next := group.Neighbors(tc.video)
			got := want{group.SmartFolder, group.ID, group.MemberIDs, prev, next}
			if got.smartFolder != tc.want.smartFolder || got.id != tc.want.id || !slices.Equal(got.members, tc.want.members) ||
				got.prev != tc.want.prev || got.next != tc.want.next {
				t.Errorf("文脈 = %+v, want %+v", got, tc.want)
			}
			if details := videoIDsOf(group.Members); !slices.Equal(details, tc.want.members) {
				t.Errorf("窓のメンバー = %v, want %v", details, tc.want.members)
			}
		})
	}

	// An explicit group takes precedence even if it does not contain the video; do not fall through to the smart folder.
	if group, found := read(solo, domain.VideoContext{GroupID: pair, SmartFolderID: folder}); found && group.SmartFolder {
		t.Errorf("group と両方のとき solo の文脈がフォルダ = %+v", group)
	}
	if _, found := read(x, domain.VideoContext{SmartFolderID: folder}); found {
		t.Error("フォルダにもグループにも無い x に文脈がある")
	}
	if group, found := read(x, domain.VideoContext{SmartFolderID: other}); !found || !group.SmartFolder || group.Name != "犬" {
		t.Errorf("x の犬のフォルダの文脈 = %+v, %v", group, found)
	}
}

func TestDeletingASmartFolderRemovesItsFavoriteAndPlaylistItem(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo, y := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/mixed/sub/y.mp4")]
	cat := attachNamedTag(t, db, "猫", solo)
	folder := newSmartFolder(t, db, "猫", domain.SmartRule{Tags: []int64{cat}}, domain.SortTitleAsc, 0)
	playlist := createPlaylist(t, db, "混ぜる")
	addVideos(t, db, playlist, solo)
	addToPlaylist(t, db, playlist, domain.PlaylistAdd{SmartFolderIDs: []int64{folder}})
	addVideos(t, db, playlist, y)
	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{SmartFolderIDs: []int64{folder}, Favorite: true}); err != nil {
		t.Fatal(err)
	}

	if err := db.SmartFolders().DeleteSmartFolder(ctx, folder); err != nil {
		t.Fatal(err)
	}
	var favorites int
	if err := db.sql.QueryRow(`select count(*) from smart_folder_favorites`).Scan(&favorites); err != nil || favorites != 0 {
		t.Fatalf("お気に入りの行 = %d, %v", favorites, err)
	}
	positions, err := queryStringsForTest(db, `select coalesce(content_key, 'folder') || ':' || position
		from playlist_items where playlist_id = ? order by position`, playlist)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"key" + fixturePath("/media/solo.mp4") + ":0", "key" + fixturePath("/media/mixed/sub/y.mp4") + ":1"}
	if !slices.Equal(positions, want) {
		t.Errorf("項目 = %v, want %v", positions, want)
	}
	if summary, items := readPlaylist(t, db, playlist); summary.ItemCount != 2 || len(items) != 2 {
		t.Errorf("消した後の項目 = %+v（数 %d）", items, summary.ItemCount)
	}
}

func videoIDsOf(videos []domain.Video) []int64 {
	out := make([]int64, 0, len(videos))
	for _, video := range videos {
		out = append(out, video.ID)
	}
	return out
}
