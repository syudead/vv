package store

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// 動画とグループのお気に入り（specs/035-favorites/data-model.md §1〜§4、research.md R-1・R-8）。

// setFavorites はお気に入りを付け外しし、反映した数を返す。
func setFavorites(t *testing.T, db *DB, change domain.FavoriteChange) domain.FavoriteApplied {
	t.Helper()
	applied, err := db.Favorites().SetFavorites(context.Background(), change)
	if err != nil {
		t.Fatalf("お気に入りを付け外しできない (%+v): %v", change, err)
	}
	return applied
}

// favoriteVideos は ids の動画にお気に入りを付ける（favorite が偽なら外す）。
func favoriteVideos(t *testing.T, db *DB, favorite bool, ids ...int64) domain.FavoriteApplied {
	t.Helper()
	return setFavorites(t, db, domain.FavoriteChange{VideoIDs: ids, Favorite: favorite})
}

// favoritedAt は表 table の鍵 key の favorited_at を返す。行が無ければ -1。
func favoritedAt(t *testing.T, db *DB, table, column, key string) int64 {
	t.Helper()
	var at int64
	err := db.sql.QueryRow(`select favorited_at from `+table+` where `+column+` = ?`, key).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func favoriteRowCount(t *testing.T, db *DB, table string) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from ` + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// freezeFavoriteClock は付け外しの時計を at に止め、試験の終わりに戻す。
func freezeFavoriteClock(t *testing.T, at time.Time) {
	t.Helper()
	previous := favoriteNow
	favoriteNow = func() time.Time { return at }
	t.Cleanup(func() { favoriteNow = previous })
}

// libraryFavorites は ListLibrary の動画の項目の id とグループの項目のフォルダのパスごとに
// Favorite を返し、グループのメンバーの Favorite も id ごとに返す。
func libraryFavorites(t *testing.T, db *DB) (videos map[int64]bool, groups map[string]bool, members map[int64]bool) {
	t.Helper()
	videos, groups, members = map[int64]bool{}, map[string]bool{}, map[int64]bool{}
	for _, item := range libraryPages(t, db, domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit}) {
		if item.Group != nil {
			groups[item.Group.Path] = item.Group.Favorite
			for _, member := range item.Group.Members {
				members[member.ID] = member.Favorite
			}
			continue
		}
		videos[item.Video.ID] = item.Video.Favorite
	}
	return videos, groups, members
}

func listedFavorites(t *testing.T, db *DB) map[int64]bool {
	t.Helper()
	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]bool{}
	for _, item := range page.Items {
		out[item.ID] = item.Favorite
	}
	return out
}

func folderGroupFavorite(t *testing.T, db *DB, dir string) bool {
	t.Helper()
	group, err := db.Library().FolderGroup(context.Background(), domain.AudienceOwner, dir)
	if err != nil {
		t.Fatalf("グループ %s を読めない: %v", dir, err)
	}
	return group.Favorite
}

// 付けると GetVideo・ListVideos・ListLibrary の Video.Favorite と ListLibrary・FolderGroup の
// LibraryGroup.Favorite が真になり、外すと偽に戻る（受け入れ条件 1）。グループに付けてもメンバーは
// 偽、メンバーに付けてもグループは偽（受け入れ条件 3、要件 4）。
func TestSetFavoritesReflectsInReads(t *testing.T) {
	db, ids := itemsFixture(t)
	solo := ids[fixturePath("/media/solo.mp4")]
	p1 := ids[fixturePath("/media/pair/p1.mp4")]
	show, pair := fixturePath("/media/show"), fixturePath("/media/pair")

	applied := setFavorites(t, db, domain.FavoriteChange{
		VideoIDs: []int64{solo, p1}, FolderPaths: []string{show}, Favorite: true,
	})
	if applied != (domain.FavoriteApplied{Videos: 2, Folders: 1}) {
		t.Fatalf("applied = %+v", applied)
	}

	if !ownerVideo(t, db, solo).Favorite || !ownerVideo(t, db, p1).Favorite {
		t.Errorf("GetVideo の Favorite が偽")
	}
	listed := listedFavorites(t, db)
	for id, want := range map[int64]bool{solo: true, p1: true, ids[fixturePath("/media/show/ep1.mp4")]: false} {
		if listed[id] != want {
			t.Errorf("ListVideos の動画 %d の Favorite = %v, want %v", id, listed[id], want)
		}
	}
	videos, groups, members := libraryFavorites(t, db)
	if !videos[solo] || videos[ids[fixturePath("/media/mixed/x.mp4")]] {
		t.Errorf("ListLibrary の動画の Favorite = %v", videos)
	}
	if !groups[show] || groups[pair] {
		t.Errorf("ListLibrary のグループの Favorite = %v", groups)
	}
	for _, path := range []string{"/media/show/ep1.mp4", "/media/show/ep2.mp4", "/media/show/ep10.mp4", "/media/pair/p2.mp4"} {
		if members[ids[fixturePath(path)]] {
			t.Errorf("メンバー %s の Favorite が真", path)
		}
	}
	if !members[p1] {
		t.Errorf("付けたメンバー p1 の Favorite が偽")
	}
	if !folderGroupFavorite(t, db, show) || folderGroupFavorite(t, db, pair) {
		t.Errorf("FolderGroup の Favorite が show・pair で真・偽でない")
	}

	applied = setFavorites(t, db, domain.FavoriteChange{
		VideoIDs: []int64{solo, p1}, FolderPaths: []string{show}, Favorite: false,
	})
	if applied != (domain.FavoriteApplied{Videos: 2, Folders: 1}) {
		t.Fatalf("外したときの applied = %+v", applied)
	}
	if ownerVideo(t, db, solo).Favorite || listedFavorites(t, db)[p1] || folderGroupFavorite(t, db, show) {
		t.Errorf("外したのに Favorite が真のまま")
	}
	videos, groups, _ = libraryFavorites(t, db)
	if videos[solo] || groups[show] {
		t.Errorf("外したのに ListLibrary の Favorite が真: %v / %v", videos, groups)
	}
	if n := favoriteRowCount(t, db, "video_favorites") + favoriteRowCount(t, db, "folder_favorites"); n != 0 {
		t.Errorf("外したあとに %d 行残った", n)
	}
}

// ライブラリに無い id・今グループでないフォルダ・登録フォルダの外のパスは数えず誤りにしない。
// 既に同じ状態のものは数え、日時を変えない。同じフォルダの重複は 1 つに数える（Edge Case）。
func TestSetFavoritesCountsOnlyApplicable(t *testing.T) {
	db, ids := itemsFixture(t)
	freezeFavoriteClock(t, fixedTime)
	solo := ids[fixturePath("/media/solo.mp4")]
	show := fixturePath("/media/show")

	applied := setFavorites(t, db, domain.FavoriteChange{
		VideoIDs:    []int64{solo, solo, 999_999},
		FolderPaths: []string{show, show, fixturePath("/media/mixed"), fixturePath("/other/show"), fixturePath("/media/missing")},
		Favorite:    true,
	})
	if applied != (domain.FavoriteApplied{Videos: 1, Folders: 1}) {
		t.Fatalf("applied = %+v, want {1 1}", applied)
	}
	if favoriteRowCount(t, db, "video_favorites") != 1 || favoriteRowCount(t, db, "folder_favorites") != 1 {
		t.Fatalf("行の数が 1・1 でない")
	}
	videoAt := favoritedAt(t, db, "video_favorites", "content_key", ownerVideo(t, db, solo).UserKey)
	folderAt := favoritedAt(t, db, "folder_favorites", "path", domain.FolderKey(show))

	applied = setFavorites(t, db, domain.FavoriteChange{VideoIDs: []int64{solo}, FolderPaths: []string{show}, Favorite: true})
	if applied != (domain.FavoriteApplied{Videos: 1, Folders: 1}) {
		t.Errorf("既に付いているものの applied = %+v, want {1 1}", applied)
	}
	if got := favoritedAt(t, db, "video_favorites", "content_key", ownerVideo(t, db, solo).UserKey); got != videoAt {
		t.Errorf("付け直しで動画の favorited_at が %d から %d に変わった", videoAt, got)
	}
	if got := favoritedAt(t, db, "folder_favorites", "path", domain.FolderKey(show)); got != folderAt {
		t.Errorf("付け直しでグループの favorited_at が %d から %d に変わった", folderAt, got)
	}

	other := ids[fixturePath("/media/mixed/x.mp4")]
	if applied := favoriteVideos(t, db, false, other, 999_999); applied.Videos != 1 {
		t.Errorf("付いていない動画を外したときの Videos = %d, want 1", applied.Videos)
	}
	if applied := setFavorites(t, db, domain.FavoriteChange{Favorite: true}); applied != (domain.FavoriteApplied{}) {
		t.Errorf("空の入力の applied = %+v", applied)
	}
}

// 同じ集まりの 2 本の id を送ると Videos は 2（鍵の数でなく id の数）で、行は集まりの鍵の 1 行。
func TestSetFavoritesCountsBundleMembersByID(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])

	if applied := favoriteVideos(t, db, true, ids["a"], ids["b"]); applied.Videos != 2 {
		t.Errorf("Videos = %d, want 2", applied.Videos)
	}
	if n := favoriteRowCount(t, db, "video_favorites"); n != 1 {
		t.Errorf("video_favorites = %d 行, want 1", n)
	}
	if !ownerVideo(t, db, ids["a"]).Favorite || !ownerVideo(t, db, ids["b"]).Favorite {
		t.Errorf("集まりのバージョンの Favorite が偽")
	}
}

// 時計を止めても、続けて付けた後のほうが大きい favorited_at を持つ。動画とグループは同じ列で
// 比べるので、2 表をまたいでも後のほうが大きい（data-model.md §4）。
func TestFavoritedAtIncreasesWithFrozenClock(t *testing.T) {
	db, ids := itemsFixture(t)
	freezeFavoriteClock(t, fixedTime)
	solo := ids[fixturePath("/media/solo.mp4")]
	x := ids[fixturePath("/media/mixed/x.mp4")]
	show := fixturePath("/media/show")

	favoriteVideos(t, db, true, solo)
	setFavorites(t, db, domain.FavoriteChange{FolderPaths: []string{show}, Favorite: true})
	favoriteVideos(t, db, true, x)

	first := favoritedAt(t, db, "video_favorites", "content_key", ownerVideo(t, db, solo).UserKey)
	second := favoritedAt(t, db, "folder_favorites", "path", domain.FolderKey(show))
	third := favoritedAt(t, db, "video_favorites", "content_key", ownerVideo(t, db, x).UserKey)
	if first != fixedTime.UnixMilli() || first >= second || second >= third {
		t.Errorf("favorited_at = %d, %d, %d; want 止めた時刻 %d から増える", first, second, third, fixedTime.UnixMilli())
	}

	// 外して付け直すと、いちばん大きい値になる。
	favoriteVideos(t, db, false, solo)
	favoriteVideos(t, db, true, solo)
	if again := favoritedAt(t, db, "video_favorites", "content_key", ownerVideo(t, db, solo).UserKey); again <= third {
		t.Errorf("付け直した favorited_at = %d, want %d より大きい", again, third)
	}
}

// 付け外しは動画の更新日時（video_edits）を進めない（research.md R-8）。
func TestSetFavoritesKeepsEditedAt(t *testing.T) {
	db, ids := versionFixture(t)
	before := editedAt(t, db, ids["a"])
	favoriteVideos(t, db, true, ids["a"])
	favoriteVideos(t, db, false, ids["a"])
	favoriteVideos(t, db, true, ids["a"])
	assertEditedAt(t, db, ids["a"], before)
	if n := editRowCount(t, db); n != 0 {
		t.Errorf("video_edits = %d 行, want 0", n)
	}
}

// 同じ内容の所在を別のフォルダへ移しても（新しい所在の取り込みと前の所在の削除）お気に入りの
// ままで（受け入れ条件 2）、再生位置を完了にしてもお気に入りのまま（受け入れ条件 6）。
func TestFavoriteSurvivesMoveAndCompletion(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	a := ids["a"]
	favoriteVideos(t, db, true, a)

	moved := upsertOne(t, db, listingFile(fixturePath("/media/moved/a.mp4"), "a", "key-a", 1))
	if moved != a {
		t.Fatalf("移した所在の動画 = %d, want %d", moved, a)
	}
	locations, err := db.Library().VideoLocations(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range locations {
		if location.Path == fixturePath("/media/a.mp4") {
			if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{location.ID}); err != nil {
				t.Fatal(err)
			}
		}
	}
	video := ownerVideo(t, db, a)
	if video.Path != fixturePath("/media/moved/a.mp4") || !video.Favorite {
		t.Errorf("移したあと Path = %s, Favorite = %v", video.Path, video.Favorite)
	}

	if _, err := db.Playback().SaveProgress(ctx, video.UserKey,
		domain.Progress{PositionMs: 100_000, DurationMs: 100_000, Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	if !ownerVideo(t, db, a).Favorite {
		t.Errorf("再生を完了にしたら Favorite が偽になった")
	}
}

// 束ねると代表の値が集まりの鍵に写って全バージョンが真になり、外したバージョンは自分の鍵の
// 値に戻る（data-model.md §1「引き継ぎと束ね」）。
func TestFavoriteFollowsBundles(t *testing.T) {
	db, ids := versionFixture(t)
	favoriteVideos(t, db, true, ids["a"])
	bundle(t, db, ids["a"], ids["a"], ids["b"], ids["c"])
	for _, name := range []string{"a", "b", "c"} {
		if !ownerVideo(t, db, ids[name]).Favorite {
			t.Errorf("束ねたあとの %s の Favorite が偽", name)
		}
	}

	b, err := db.Versions().Unbundle(context.Background(), ids["b"])
	if err != nil {
		t.Fatal(err)
	}
	if b.Favorite || ownerVideo(t, db, ids["b"]).Favorite {
		t.Errorf("外した B の Favorite が真（自分の鍵には行が無い）")
	}
	if !ownerVideo(t, db, ids["a"]).Favorite || !ownerVideo(t, db, ids["c"]).Favorite {
		t.Errorf("集まりに残った A・C の Favorite が偽")
	}
}

// 同じパスの中身の引き継ぎは、前の内容のお気に入りを新しい内容へ移す。
func TestSuccessionCarriesFavorite(t *testing.T) {
	db, a := taggedVideoFixture(t)
	favoriteVideos(t, db, true, a)

	scan := startTestScan(t, db)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, b, 100_000)

	if !ownerVideo(t, db, b).Favorite {
		t.Errorf("引き継いだ新しい内容の Favorite が偽")
	}
	if got := favoritedAt(t, db, "video_favorites", "content_key", "key-a"); got != -1 {
		t.Errorf("前の内容の行が残った")
	}
}

// フォルダの索引の作り直し・動画の削除（releaseContentIndex）・メディアフォルダの削除のあとも、
// 2 表の行は残る（Edge Case）。生成物の片付け（artifacts の RemoveContent）はファイルだけを消し、
// データベースに触れない。
func TestFavoritesSurviveIndexChanges(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo := ids[fixturePath("/media/solo.mp4")]
	setFavorites(t, db, domain.FavoriteChange{
		VideoIDs: []int64{solo}, FolderPaths: []string{fixturePath("/media/show")}, Favorite: true,
	})
	assertRows := func(step string) {
		t.Helper()
		if favoriteRowCount(t, db, "video_favorites") != 1 || favoriteRowCount(t, db, "folder_favorites") != 1 {
			t.Errorf("%s のあとに行が消えた", step)
		}
	}

	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	assertRows("フォルダの索引の作り直し")

	if err := db.ScanIndex().DeleteVideos(ctx, []int64{solo}); err != nil {
		t.Fatal(err)
	}
	assertRows("動画の削除")

	folders, err := db.ScanIndex().ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range folders {
		if err := db.Settings().DeleteMediaFolder(ctx, folder.ID, folder.Version); err != nil {
			t.Fatal(err)
		}
	}
	assertRows("メディアフォルダの削除")
}

// 移行は空の 2 表を作り、Down は表を落とす。
func TestFavoritesMigration(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 28); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into videos(content_key, probe_state, thumbnail_state, preview_state,
		seek_thumbnail_state) values ('key-a', 'done', 'done', 'done', 'done')`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 29); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"video_favorites", "folder_favorites"} {
		if n := favoriteRowCount(t, db, table); n != 0 {
			t.Errorf("移行のあとの %s = %d 行, want 0", table, n)
		}
	}
	assertFavoriteInvariant(t, db)
	if _, err := provider.DownTo(ctx, 28); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master
		where name in ('video_favorites', 'folder_favorites')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("Down のあとも %d 表が残った", tables)
	}
}
