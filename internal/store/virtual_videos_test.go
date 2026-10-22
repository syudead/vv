package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func createVirtual(t *testing.T, db *DB, title string, at time.Time) domain.Video {
	t.Helper()
	previous := virtualNow
	virtualNow = func() time.Time { return at }
	t.Cleanup(func() { virtualNow = previous })
	video, err := db.VirtualVideos().Create(context.Background(), title)
	if err != nil {
		t.Fatalf("バーチャルな動画 %q を作れない: %v", title, err)
	}
	return video
}

// A virtual video is listed and searched by its title for the owner, for a guest only while public, and by its tags
// only for the owner; folder reads and the file kind never return it.
func TestVirtualVideoInLibraryReads(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))
	virtual := createVirtual(t, db, "  Moonlight  ", fixedTime)
	if virtual.Title != "Moonlight" || virtual.ContentKey != virtual.UserKey || virtual.Playable {
		t.Fatalf("作った動画 = %+v, want 前後を詰めたタイトルと再生できない投影", virtual)
	}
	if _, _, err := db.Tags().AttachTagByName(ctx, []int64{virtual.ID}, "piano"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		audience domain.Audience
		query    domain.VideoQuery
		want     []string
	}{
		{"所有者の一覧", owner, domain.VideoQuery{}, []string{"Moonlight", "clip"}},
		{"所有者のタイトル検索", owner, domain.VideoQuery{Query: "moon"}, []string{"Moonlight"}},
		{"所有者の短い語", owner, domain.VideoQuery{Query: "mo"}, []string{"Moonlight"}},
		{"所有者のタグ検索", owner, domain.VideoQuery{Query: "piano"}, []string{"Moonlight"}},
		{"非公開のときゲストの一覧", guest, domain.VideoQuery{}, []string{}},
		{"ファイルのある動画だけ", owner, domain.VideoQuery{Kind: domain.VideoKindFile}, []string{"clip"}},
		{"バーチャルな動画だけ", owner, domain.VideoQuery{Kind: domain.VideoKindVirtual}, []string{"Moonlight"}},
	} {
		if got, _ := listTitles(t, db, tc.audience, tc.query); !slices.Equal(got, tc.want) {
			t.Errorf("%s: titles = %v, want %v", tc.name, got, tc.want)
		}
	}
	if _, err := db.Library().GetVideo(ctx, guest, virtual.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("非公開をゲストが読めた: err = %v", err)
	}

	setPublic(t, db, true, virtual.ID)
	if got, _ := listTitles(t, db, guest, domain.VideoQuery{Query: "moon"}); !slices.Equal(got, []string{"Moonlight"}) {
		t.Errorf("公開のときゲストのタイトル検索 = %v, want [Moonlight]", got)
	}
	if got, _ := listTitles(t, db, guest, domain.VideoQuery{Query: "piano"}); len(got) != 0 {
		t.Errorf("ゲストがタグ名で見つけた: %v", got)
	}
	got, err := db.Library().GetVideo(ctx, guest, virtual.ID)
	if err != nil || got.Title != "Moonlight" || got.Path != "" || got.SizeBytes != 0 || !got.Public {
		t.Errorf("公開のときゲストの詳細 = %+v, err = %v", got, err)
	}

	page, err := db.Library().ListFolderVideos(ctx, owner, domain.FolderVideoQuery{
		Dir: fixturePath("/media"), Scope: domain.FolderScopeSubtree, Limit: domain.MaxLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if titles := titlesOf(page); !slices.Equal(titles, []string{"clip"}) {
		t.Errorf("フォルダの読み取り = %v, want [clip]", titles)
	}
}

// The scan's orphan sweep keeps the location-less projected row, and startup recreates the row when it is missing.
func TestVirtualVideoSurvivesScanAndStartup(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/gone.mp4"), "gone", "key-gone", 1))
	virtual := createVirtual(t, db, "Kept", fixedTime)
	setPublic(t, db, true, virtual.ID)

	locations, err := db.Library().VideoLocations(ctx, ids[fixturePath("/media/gone.mp4")])
	if err != nil || len(locations) != 1 {
		t.Fatalf("locations = %v, err = %v", locations, err)
	}
	if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{locations[0].ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := listTitles(t, db, owner, domain.VideoQuery{}); !slices.Equal(got, []string{"Kept"}) {
		t.Errorf("欠けた場所を消した後 = %v, want [Kept]", got)
	}

	if _, err := db.sql.ExecContext(ctx, `delete from videos where content_key = ?`, virtual.UserKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Library().RefreshSearchKeys(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := listTitles(t, db, guest, domain.VideoQuery{}); !slices.Equal(got, []string{"Kept"}) {
		t.Errorf("起動処理の後のゲスト = %v, want 公開のまま作り直された [Kept]", got)
	}
}

// Between two file videos in every time and title order, and last in both directions for duration and size.
func TestVirtualVideoSortPositions(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	alpha := listingFile(fixturePath("/media/a.mp4"), "alpha", "key-a", 1)
	alpha.MTime = fixedTime.Add(time.Minute)
	charlie := listingFile(fixturePath("/media/c.mp4"), "charlie", "key-c", 3)
	charlie.MTime = fixedTime.Add(3 * time.Minute)
	ids := upsertAll(t, db, alpha, charlie)
	for path, duration := range map[string]int64{alpha.Path: 1000, charlie.Path: 3000} {
		if _, err := db.sql.ExecContext(ctx, `update videos set duration_ms = ? where id = ?`, duration, ids[path]); err != nil {
			t.Fatal(err)
		}
	}
	createVirtual(t, db, "bravo", fixedTime.Add(2*time.Minute))

	between := []string{"alpha", "bravo", "charlie"}
	reversed := []string{"charlie", "bravo", "alpha"}
	last := []string{"alpha", "charlie", "bravo"}
	lastDesc := []string{"charlie", "alpha", "bravo"}
	for sort, want := range map[domain.VideoSort][]string{
		domain.SortAddedAsc: between, domain.SortAddedDesc: reversed,
		domain.SortModifiedAsc: between, domain.SortModifiedDesc: reversed,
		domain.SortCreatedAsc: between, domain.SortCreatedDesc: reversed,
		domain.SortTitleAsc: between, domain.SortTitleDesc: reversed,
		domain.SortDurationAsc: last, domain.SortDurationDesc: lastDesc,
		domain.SortSizeAsc: last, domain.SortSizeDesc: lastDesc,
	} {
		videos, err := db.Library().ListVideos(ctx, owner, domain.VideoQuery{Sort: sort, Limit: domain.MaxLimit})
		if err != nil {
			t.Fatalf("%s: %v", sort, err)
		}
		if got := titlesOf(videos); !slices.Equal(got, want) {
			t.Errorf("ListVideos %s = %v, want %v", sort, got, want)
		}
		library, err := db.Library().ListLibrary(ctx, owner, domain.VideoQuery{Sort: sort, Limit: domain.MaxLimit})
		if err != nil {
			t.Fatalf("%s: %v", sort, err)
		}
		var got []string
		for _, item := range library.Items {
			got = append(got, item.Video.Title)
		}
		if !slices.Equal(got, want) {
			t.Errorf("ListLibrary %s = %v, want %v", sort, got, want)
		}
	}
}

func TestDeleteVirtualVideoRemovesUserData(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	virtual := createVirtual(t, db, "Doomed", fixedTime)
	key := virtual.UserKey
	if _, _, err := db.Tags().AttachTagByName(ctx, []int64{virtual.ID}, "piano"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{VideoIDs: []int64{virtual.ID}, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	setPublic(t, db, true, virtual.ID)
	if added := addVideos(t, db, createPlaylist(t, db, "list"), virtual.ID); added.Added != 1 {
		t.Fatalf("プレイリストに足した数 = %d, want 1", added.Added)
	}
	if _, err := db.WorkInfo().Set(ctx, virtual.ID, domain.WorkInfo{Description: "notes", URLs: []string{"https://example.com/"}}); err != nil {
		t.Fatal(err)
	}
	recorder := &releasedRecorder{}
	db.PublishTo(recorder)

	if err := db.VirtualVideos().Delete(ctx, virtual.ID, key); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"virtual_videos", "videos", "video_tags", "video_favorites", "public_videos",
		"playlist_items", "video_work_info", "video_work_urls"} {
		column := "content_key"
		if table == "virtual_videos" {
			column = "user_key"
		}
		var count int
		if err := db.sql.QueryRowContext(ctx, `select count(*) from `+table+` where `+column+` = ?`, key).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s に %d 行残った", table, count)
		}
	}
	if keys := recorder.take(); !slices.Equal(keys, []string{key}) {
		t.Errorf("参照の無くなったキー = %v, want [%s]", keys, key)
	}
	if err := db.VirtualVideos().Delete(ctx, virtual.ID, key); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("消した後の削除: err = %v, want domain.ErrNotFound", err)
	}
}

// A stale version, a file video and an empty title are rejected without changing the virtual video.
func TestVirtualVideoWritesRejectWrongTargets(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))
	virtual := createVirtual(t, db, "Before", fixedTime)
	stale := virtualUserKey(virtual.ID + 1000)

	if _, err := db.VirtualVideos().SaveTitle(ctx, virtual.ID, stale, "After"); !errors.Is(err, domain.ErrContentChanged) {
		t.Errorf("古い版で保存: err = %v, want domain.ErrContentChanged", err)
	}
	if err := db.VirtualVideos().Delete(ctx, virtual.ID, stale); !errors.Is(err, domain.ErrContentChanged) {
		t.Errorf("古い版で削除: err = %v, want domain.ErrContentChanged", err)
	}
	if err := db.VirtualVideos().Delete(ctx, ids[fixturePath("/media/clip.mp4")], "key-clip"); !errors.Is(err, domain.ErrNotVirtual) {
		t.Errorf("ファイルのある動画を削除: err = %v, want domain.ErrNotVirtual", err)
	}
	if _, err := db.VirtualVideos().SaveTitle(ctx, virtual.ID, virtual.UserKey, " "); !errors.Is(err, domain.ErrInvalidVirtualTitle) {
		t.Errorf("空のタイトルで保存: err = %v, want domain.ErrInvalidVirtualTitle", err)
	}
	if _, err := db.VirtualVideos().Create(ctx, "\t"); !errors.Is(err, domain.ErrInvalidVirtualTitle) {
		t.Errorf("空のタイトルで作成: err = %v, want domain.ErrInvalidVirtualTitle", err)
	}
	if got, _ := listTitles(t, db, owner, domain.VideoQuery{Query: "before"}); !slices.Equal(got, []string{"Before"}) {
		t.Errorf("拒んだ後の検索 = %v, want [Before]", got)
	}

	saved, err := db.VirtualVideos().SaveTitle(ctx, virtual.ID, virtual.UserKey, " After ")
	if err != nil || saved.Title != "After" {
		t.Fatalf("保存 = %+v, err = %v", saved, err)
	}
	if got, _ := listTitles(t, db, owner, domain.VideoQuery{Query: "after"}); !slices.Equal(got, []string{"After"}) {
		t.Errorf("保存した後の検索 = %v, want [After]", got)
	}
}

// Related candidates, tag summaries and credit counts include a virtual video.
func TestVirtualVideoInRelatedAndCounts(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))
	virtual := createVirtual(t, db, "Moonlight", fixedTime)
	if _, _, err := db.Tags().AttachTagByName(ctx, []int64{virtual.ID}, "piano"); err != nil {
		t.Fatal(err)
	}
	person := createPerson(t, db, "A")
	linkPeople(t, db, virtual.UserKey, "performer", person.ID)

	candidates, err := db.Library().RelatedCandidates(ctx, owner, ids[fixturePath("/media/clip.mp4")])
	if err != nil {
		t.Fatalf("関連動画の候補: %v", err)
	}
	if !slices.ContainsFunc(candidates, func(c domain.RelatedCandidate) bool {
		return c.VideoID == virtual.ID && c.Title == "Moonlight" && c.Path == ""
	}) {
		t.Errorf("候補 = %+v, want バーチャルな動画をタイトル付きで含む", candidates)
	}
	summary, err := db.Tags().Summary(ctx, []int64{virtual.ID})
	if err != nil || summary.Total != 1 || len(summary.Items) != 1 || summary.Items[0].Tag.Name != "piano" {
		t.Errorf("タグのまとめ = %+v, err = %v, want piano が 1 件", summary, err)
	}
	if got, err := db.Credits().Get(ctx, domain.CreditPerson, person.ID); err != nil || got.VideoCount != 1 {
		t.Errorf("人物の本数 = %+v, err = %v, want 1", got, err)
	}
}

// A kind that drops some videos must not list a group whose members it drops.
func TestVirtualKindLeavesOutFileGroups(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))
	createGroup(t, db, "files", domain.GroupAdd{VideoIDs: []int64{ids[fixturePath("/media/clip.mp4")]}})
	createVirtual(t, db, "Moonlight", fixedTime)

	page, err := db.Library().ListLibrary(ctx, owner, domain.VideoQuery{Kind: domain.VideoKindVirtual, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Video == nil || page.Items[0].Video.Title != "Moonlight" {
		t.Errorf("バーチャルな動画だけのライブラリ = %+v, want [Moonlight] だけ", page.Items)
	}
}
