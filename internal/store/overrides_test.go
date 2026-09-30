package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 動画の表示名（specs/029-video-overrides/data-model.md §1・§3・§4）。

// overrideFixture は題名 alpha・beta・gamma の3本を /media の下に取り込む。
func overrideFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/alpha.mp4"), "alpha", "key-alpha", 1),
		listingFile(fixturePath("/media/beta.mp4"), "beta", "key-beta", 2),
		listingFile(fixturePath("/media/gamma.mp4"), "gamma", "key-gamma", 3),
	)
	return db, ids
}

func setDisplayName(t *testing.T, db *DB, id int64, name string) domain.Video {
	t.Helper()
	video, err := db.Overrides().SetDisplayName(context.Background(), id, name)
	if err != nil {
		t.Fatalf("表示名を設定できない (%q): %v", name, err)
	}
	return video
}

func overrideRowCount(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_overrides`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func titlesInOrder(t *testing.T, db *DB, q domain.VideoQuery) []string {
	t.Helper()
	q.Limit = domain.MaxLimit
	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, q)
	if err != nil {
		t.Fatal(err)
	}
	return titlesOf(page)
}

// 表示名を付けた動画は、どの読み出しでも Title が表示名、FileTitle が所在の題名になり、
// 題名順では表示名の位置に並び、表示名でもファイル名でも検索に出る（受け入れ条件 4）。
func TestSetDisplayNameAppliesToTitleSortAndSearch(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	betaID := ids[fixturePath("/media/beta.mp4")]

	returned := setDisplayName(t, db, betaID, "  zulu 旅行  ")
	if returned.Title != "zulu 旅行" || returned.FileTitle != "beta" || returned.DisplayName != "zulu 旅行" {
		t.Fatalf("返した動画 = Title %q FileTitle %q DisplayName %q", returned.Title, returned.FileTitle, returned.DisplayName)
	}

	check := func(where string, video domain.Video) {
		t.Helper()
		if video.Title != "zulu 旅行" || video.FileTitle != "beta" || video.DisplayName != "zulu 旅行" {
			t.Errorf("%s: Title %q FileTitle %q DisplayName %q", where, video.Title, video.FileTitle, video.DisplayName)
		}
	}
	for _, audience := range []domain.Audience{domain.AudienceOwner} {
		video, err := db.Library().GetVideo(ctx, audience, betaID)
		if err != nil {
			t.Fatal(err)
		}
		check("LibraryStore.GetVideo", video)
	}
	ingested, err := db.Ingest().GetVideo(ctx, betaID)
	if err != nil {
		t.Fatal(err)
	}
	check("IngestStore.GetVideo", ingested)
	related, err := db.Library().VideosByIDs(ctx, domain.AudienceOwner, []int64{betaID})
	if err != nil || len(related) != 1 {
		t.Fatalf("VideosByIDs = %v, %v", related, err)
	}
	check("VideosByIDs", related[0])
	external, err := db.Library().LookupExternalVideo(ctx, domain.VideoRef{ID: betaID})
	if err != nil {
		t.Fatal(err)
	}
	check("LookupExternalVideo", external.Video)
	library, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{Query: "zulu", Limit: domain.MaxLimit})
	if err != nil || len(library.Items) != 1 || library.Items[0].Video == nil {
		t.Fatalf("ListLibrary = %+v, %v", library, err)
	}
	check("ListLibrary", *library.Items[0].Video)

	// 表示名の無い動画は FileTitle と Title が同じ。
	alpha, err := db.Library().GetVideo(ctx, domain.AudienceOwner, ids[fixturePath("/media/alpha.mp4")])
	if err != nil {
		t.Fatal(err)
	}
	if alpha.Title != "alpha" || alpha.FileTitle != "alpha" || alpha.DisplayName != "" || alpha.ThumbnailPositionMs != nil {
		t.Errorf("表示名の無い動画 = %+v", alpha)
	}

	if got := titlesInOrder(t, db, domain.VideoQuery{Sort: domain.SortTitleAsc}); !slices.Equal(got, []string{"alpha", "gamma", "zulu 旅行"}) {
		t.Errorf("titleAsc = %v", got)
	}
	if got := titlesInOrder(t, db, domain.VideoQuery{Sort: domain.SortTitleDesc}); !slices.Equal(got, []string{"zulu 旅行", "gamma", "alpha"}) {
		t.Errorf("titleDesc = %v", got)
	}
	for _, query := range []string{"zulu", "旅行", "beta"} {
		if got := searchTitles(t, db, query); !slices.Equal(got, []string{"zulu 旅行"}) {
			t.Errorf("検索 %q = %v, want [zulu 旅行]", query, got)
		}
	}

	searchKey, titleKey, version := locationKeys(t, db, fixturePath("/media/beta.mp4"))
	if want := "beta\nbeta.mp4\n" + domain.FoldForMatch("zulu 旅行"); searchKey != want {
		t.Errorf("search_key = %q, want %q", searchKey, want)
	}
	if titleKey != domain.NaturalSortKey("zulu 旅行") || version != domain.SearchKeyVersion {
		t.Errorf("title_key = %q, version = %d", titleKey, version)
	}
}

// 表示名は content_key に結ぶので、所在を別のパスへ移しても（改名・移動の再スキャン）
// 残り、新しい所在の鍵にも入る（受け入れ条件 3）。同じ内容の2つ目の所在からも同じ
// Title が返る。
func TestDisplayNameSurvivesMoveAndAppliesToEveryLocation(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	betaID := ids[fixturePath("/media/beta.mp4")]
	setDisplayName(t, db, betaID, "表示名")

	// 2つ目の所在（同じ内容）。
	second := fixturePath("/media/copy/beta-copy.mp4")
	upsertAll(t, db, listingFile(second, "beta-copy", "key-beta", 4))
	searchKey, titleKey, _ := locationKeys(t, db, second)
	if !strings.HasSuffix(searchKey, "\n"+domain.FoldForMatch("表示名")) || titleKey != domain.NaturalSortKey("表示名") {
		t.Errorf("2つ目の所在の鍵 = %q / %q", searchKey, titleKey)
	}
	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: "beta-copy", Limit: domain.MaxLimit})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("2つ目の所在の検索 = %+v, %v", page, err)
	}
	if got := page.Items[0]; got.Title != "表示名" || got.FileTitle != "beta-copy" {
		t.Errorf("2つ目の所在の動画 = Title %q FileTitle %q", got.Title, got.FileTitle)
	}

	// 元の所在を消し、別のパスへ移す。
	moved := fixturePath("/media/moved/renamed.mp4")
	upsertAll(t, db, listingFile(moved, "renamed", "key-beta", 5))
	var locationIDs []int64
	for _, path := range []string{fixturePath("/media/beta.mp4"), second} {
		var id int64
		if err := db.sql.QueryRow(`select id from video_locations where path = ?`, path).Scan(&id); err != nil {
			t.Fatal(err)
		}
		locationIDs = append(locationIDs, id)
	}
	if err := db.ScanIndex().DeleteVideoLocations(ctx, locationIDs); err != nil {
		t.Fatal(err)
	}
	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, betaID)
	if err != nil {
		t.Fatal(err)
	}
	if video.Title != "表示名" || video.FileTitle != "renamed" || video.Path != moved {
		t.Errorf("移動後 = Title %q FileTitle %q Path %q", video.Title, video.FileTitle, video.Path)
	}
	if got := searchTitles(t, db, "表示名"); !slices.Equal(got, []string{"表示名"}) {
		t.Errorf("移動後の検索 = %v", got)
	}
	if got := titlesInOrder(t, db, domain.VideoQuery{Sort: domain.SortTitleAsc}); !slices.Equal(got, []string{"alpha", "gamma", "表示名"}) {
		t.Errorf("移動後の titleAsc = %v", got)
	}
}

// 空・空白だけの名前は解除で、行が消え、鍵は表示名の無い値に戻る。制御文字と
// 201 符号位置は *domain.InvalidDisplayNameError で何も書かない。
func TestSetDisplayNameClearsAndRejects(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	betaID := ids[fixturePath("/media/beta.mp4")]
	setDisplayName(t, db, betaID, "表示名")

	for _, input := range []string{"改行\n", strings.Repeat("あ", domain.DisplayNameMaxLength+1)} {
		_, err := db.Overrides().SetDisplayName(ctx, betaID, input)
		var invalid *domain.InvalidDisplayNameError
		if !errors.As(err, &invalid) {
			t.Fatalf("SetDisplayName(%q) err = %v, want *InvalidDisplayNameError", input, err)
		}
		video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, betaID)
		if err != nil {
			t.Fatal(err)
		}
		if video.DisplayName != "表示名" {
			t.Errorf("誤りのあとの表示名 = %q, want 表示名のまま", video.DisplayName)
		}
	}

	cleared := setDisplayName(t, db, betaID, " 　 ")
	if cleared.Title != "beta" || cleared.FileTitle != "beta" || cleared.DisplayName != "" {
		t.Errorf("解除後 = Title %q FileTitle %q DisplayName %q", cleared.Title, cleared.FileTitle, cleared.DisplayName)
	}
	if count := overrideRowCount(t, db); count != 0 {
		t.Errorf("解除後の video_overrides = %d 行, want 0", count)
	}
	searchKey, titleKey, _ := locationKeys(t, db, fixturePath("/media/beta.mp4"))
	if searchKey != "beta\nbeta.mp4" || titleKey != domain.NaturalSortKey("beta") {
		t.Errorf("解除後の鍵 = %q / %q", searchKey, titleKey)
	}
	if got := searchTitles(t, db, "表示名"); len(got) != 0 {
		t.Errorf("解除後に表示名で %v が当たった", got)
	}

	// 無い行の解除も誤りにしない。
	setDisplayName(t, db, betaID, "")
	if _, err := db.Overrides().SetDisplayName(ctx, 9999, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い動画 err = %v, want ErrNotFound", err)
	}
	if count := overrideRowCount(t, db); count != 0 {
		t.Errorf("video_overrides = %d 行, want 0", count)
	}
}

// 表示名を付けた動画のあるメディアフォルダを登録し直しても（AddMediaFolder・
// ReplaceMediaFolder）、鍵は表示名を含む。
func TestMediaFolderChangesKeepDisplayNameInKeys(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := t.TempDir()
	first, added, replacement := filepath.Join(root, "first"), filepath.Join(root, "added"), filepath.Join(root, "replacement")
	for _, dir := range []string{first, added, replacement} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	folder, err := db.Settings().AddMediaFolder(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	// 同じ内容の所在を3つ置く。登録の下にあるのは first の1つだけである。
	inFirst := filepath.Join(first, "clip.mp4")
	inAdded := filepath.Join(added, "season", "copy.mp4")
	inReplacement := filepath.Join(replacement, "deep", "other.mp4")
	ids := upsertAll(t, db,
		sampleFile(inFirst, "clip", "key-clip", 1, 0),
		sampleFile(inAdded, "copy", "key-clip", 1, 0),
		sampleFile(inReplacement, "other", "key-clip", 1, 0),
	)
	setDisplayName(t, db, ids[inFirst], "名前")
	wantSuffix := "\n" + domain.FoldForMatch("名前")
	for _, path := range []string{inAdded, inReplacement} {
		if searchKey, _, _ := locationKeys(t, db, path); searchKey != "" {
			t.Fatalf("登録外の search_key = %q, want 空", searchKey)
		}
	}

	if _, err := db.Settings().AddMediaFolder(ctx, added); err != nil {
		t.Fatal(err)
	}
	searchKey, titleKey, _ := locationKeys(t, db, inAdded)
	if want := "copy\nseason/copy.mp4" + wantSuffix; searchKey != want {
		t.Errorf("追加後の search_key = %q, want %q", searchKey, want)
	}
	if titleKey != domain.NaturalSortKey("名前") {
		t.Errorf("追加後の title_key = %q", titleKey)
	}

	if _, err := db.Settings().ReplaceMediaFolder(ctx, folder.ID, folder.Version, replacement); err != nil {
		t.Fatal(err)
	}
	searchKey, titleKey, _ = locationKeys(t, db, inReplacement)
	if want := "other\ndeep/other.mp4" + wantSuffix; searchKey != want {
		t.Errorf("差し替え後の search_key = %q, want %q", searchKey, want)
	}
	if titleKey != domain.NaturalSortKey("名前") {
		t.Errorf("差し替え後の title_key = %q", titleKey)
	}
}

// 起動時の埋め直し（RefreshSearchKeys）も表示名を読む。
func TestRefreshSearchKeysReadsDisplayName(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	setDisplayName(t, db, ids[fixturePath("/media/beta.mp4")], "名前")
	if _, err := db.sql.Exec(`update video_locations set search_version = 0, search_key = '', title_key = ''`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Library().RefreshSearchKeys(ctx); err != nil {
		t.Fatal(err)
	}
	searchKey, titleKey, _ := locationKeys(t, db, fixturePath("/media/beta.mp4"))
	if searchKey != "beta\nbeta.mp4\n"+domain.FoldForMatch("名前") || titleKey != domain.NaturalSortKey("名前") {
		t.Errorf("埋め直し後の鍵 = %q / %q", searchKey, titleKey)
	}
	if searchKey, _, _ := locationKeys(t, db, fixturePath("/media/alpha.mp4")); searchKey != "alpha\nalpha.mp4" {
		t.Errorf("表示名の無い所在の鍵 = %q, want 前と同じ値", searchKey)
	}
}

// 確定後に VideoOverrideChanged が1回発行され、誤りでは発行しない。
func TestSetDisplayNamePublishesAfterCommit(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	betaID := ids[fixturePath("/media/beta.mp4")]

	if _, err := db.Overrides().SetDisplayName(ctx, betaID, "a\tb"); err == nil {
		t.Fatal("制御文字を受け付けた")
	}
	if _, err := db.Overrides().SetDisplayName(ctx, 9999, "x"); err == nil {
		t.Fatal("無い動画を受け付けた")
	}
	if len(recorder.events) != 0 {
		t.Fatalf("誤りで発行した: %v", recorder.events)
	}
	setDisplayName(t, db, betaID, "名前")
	if want := []domain.Event{domain.VideoOverrideChanged{VideoID: betaID}}; !slices.Equal(recorder.events, want) {
		t.Errorf("events = %v, want %v", recorder.events, want)
	}
}

// 一括の表示名は1つの取引で、引けない動画か規則に合わない名前が1件でもあれば位置を返して
// 何も残さない。成功すれば指定の順に反映後の動画を返す。
func TestSetDisplayNamesIsAtomic(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	alpha := domain.VideoRef{ID: ids[fixturePath("/media/alpha.mp4")]}
	beta := domain.VideoRef{Path: fixturePath("/media/beta.mp4")}

	_, err := db.Overrides().SetDisplayNames(ctx, []domain.DisplayNameChange{
		{Video: alpha, DisplayName: "一"}, {Video: domain.VideoRef{ContentKey: "missing"}, DisplayName: "二"},
	})
	var notFound *domain.VideoRefNotFoundError
	if !errors.As(err, &notFound) || notFound.Index != 1 || !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("引けない動画 err = %v", err)
	}
	_, err = db.Overrides().SetDisplayNames(ctx, []domain.DisplayNameChange{
		{Video: alpha, DisplayName: "一"}, {Video: beta, DisplayName: strings.Repeat("a", domain.DisplayNameMaxLength+1)},
	})
	var at *domain.DisplayNameAtError
	var invalid *domain.InvalidDisplayNameError
	if !errors.As(err, &at) || at.Index != 1 || !errors.As(err, &invalid) || invalid.Problem != domain.DisplayNameTooLong {
		t.Fatalf("長すぎる名前 err = %v", err)
	}
	if count := overrideRowCount(t, db); count != 0 || len(recorder.events) != 0 {
		t.Fatalf("失敗のあとに行 %d・知らせ %v が残った", count, recorder.events)
	}

	videos, err := db.Overrides().SetDisplayNames(ctx, []domain.DisplayNameChange{
		{Video: beta, DisplayName: "二"}, {Video: alpha, DisplayName: "一"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 2 || videos[0].Title != "二" || videos[0].FileTitle != "beta" || videos[1].Title != "一" {
		t.Fatalf("videos = %+v", videos)
	}
	if got := titlesInOrder(t, db, domain.VideoQuery{Sort: domain.SortTitleAsc}); !slices.Equal(got, []string{"gamma", "一", "二"}) {
		t.Errorf("titleAsc = %v", got)
	}
	if len(recorder.events) != 2 {
		t.Errorf("events = %v, want 2 件", recorder.events)
	}

	// 解除（空）も一括でできる。
	if _, err := db.Overrides().SetDisplayNames(ctx, []domain.DisplayNameChange{{Video: alpha}, {Video: beta, DisplayName: " "}}); err != nil {
		t.Fatal(err)
	}
	if count := overrideRowCount(t, db); count != 0 {
		t.Errorf("解除後の video_overrides = %d 行", count)
	}
}
