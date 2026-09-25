package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fixedTime はテストの期待値を安定させるための基準時刻である。
var fixedTime = time.Unix(1_757_000_000, 0).UTC()

// sampleFile は走査で分かる事実を1件組み立てる。
func sampleFile(path, title, key string, size int64, offset time.Duration) domain.VideoFile {
	return domain.VideoFile{
		Path:       path,
		Title:      title,
		ContentKey: key,
		SizeBytes:  size,
		MTime:      fixedTime.Add(offset),
		Container:  domain.ContainerFromPath(path),
	}
}

// 新規の取り込み。走査で分かる事実だけが入り、解析はこれからの状態になる。
func TestUpsertVideoAddsNewRow(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	got, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/海辺の散歩.mp4", "海辺の散歩", "key-a", 1024, 0))
	if err != nil {
		t.Fatalf("取り込めない: %v", err)
	}
	if got.Outcome != domain.OutcomeAdded {
		t.Errorf("Outcome = %q, want %q", got.Outcome, domain.OutcomeAdded)
	}
	if got.ID == 0 {
		t.Error("ID が返っていない")
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if video.Title != "海辺の散歩" {
		t.Errorf("Title = %q", video.Title)
	}
	if video.ContentKey != "key-a" {
		t.Errorf("ContentKey = %q", video.ContentKey)
	}
	// 解析前は「再生できない」側に倒す。
	if video.Playable {
		t.Error("解析前なのに再生できる扱いになっている")
	}
	if video.ProbeState != domain.ProbeStatePending {
		t.Errorf("ProbeState = %q, want pending", video.ProbeState)
	}
	if video.ThumbnailState != domain.ThumbnailStatePending {
		t.Errorf("ThumbnailState = %q, want pending", video.ThumbnailState)
	}
	// 取得できていない尺は null のままにする。0 で代用しない。
	if video.DurationMs != nil {
		t.Errorf("DurationMs = %v, want nil", *video.DurationMs)
	}
}

// 同じパス・同じサイズ・同じ mtime の再取り込みでは何も変わらない。
// 2 回目以降のスキャンを安く保つ前提である。
func TestUpsertVideoIsUnchangedWhenNothingMoved(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	file := sampleFile("/media/a.mp4", "a", "key-a", 1024, 0)
	first, err := db.ScanIndex().UpsertVideo(ctx, file)
	if err != nil {
		t.Fatal(err)
	}

	second, err := db.ScanIndex().UpsertVideo(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != domain.OutcomeUnchanged {
		t.Errorf("Outcome = %q, want %q", second.Outcome, domain.OutcomeUnchanged)
	}
	if second.ID != first.ID {
		t.Errorf("ID が変わった: %d -> %d", first.ID, second.ID)
	}
}

// 内容が変われば（サイズか mtime が変わる）属性を更新し、解析をやり直す。
func TestUpsertVideoUpdatesChangedFileAndResetsProbe(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().ApplyProbe(ctx, added.ID, domain.Probe{
		DurationMs: 8000, Width: 640, Height: 360, VideoCodec: "h264", AudioCodec: "aac",
	}, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}

	// 同じパスで内容が差し替わった（content_key も変わる）。
	updated, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a2", 2048, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Outcome != domain.OutcomeUpdated {
		t.Errorf("Outcome = %q, want %q", updated.Outcome, domain.OutcomeUpdated)
	}
	if updated.ID == added.ID {
		t.Errorf("内容が変わったのに論理動画IDが維持された: %d", added.ID)
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ContentKey != "key-a2" {
		t.Errorf("ContentKey = %q, want key-a2", video.ContentKey)
	}
	if video.SizeBytes != 2048 {
		t.Errorf("SizeBytes = %d, want 2048", video.SizeBytes)
	}
	if video.ProbeState != domain.ProbeStatePending {
		t.Errorf("ProbeState = %q, want pending（内容が変わったら解析し直す）", video.ProbeState)
	}
	if video.Playable {
		t.Error("解析し直す前なのに再生できる扱いのままになっている")
	}
}

func TestReassigningRepresentativeLocationSynchronizesOldVideo(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	oldVideo, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mkv", "a", "old", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/b.mp4", "b", "old", 1, 0)); err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, oldVideo.ID, probe, domain.EvaluatePlayability("mkv", probe)); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mkv", "replacement", "new", 2, time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := db.Library().GetVideo(ctx, domain.AudienceOwner, oldVideo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/media/b.mp4" || got.Container != "mp4" || !got.Playable {
		t.Fatalf("old video representative was not synchronized: %+v", got)
	}
}

// 移動・改名は行の作り直しではなくパスの更新になる。重複を作らないことが
// 要求で、再生位置を引き継ぐ前提でもある。
func TestUpsertVideoTreatsSameContentAtNewPathAsMove(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/海辺の散歩.mp4", "海辺の散歩", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}

	moved, err := db.ScanIndex().UpsertVideo(ctx,
		sampleFile("/media/2026/海辺の散歩（編集済み）.mp4", "海辺の散歩（編集済み）", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if moved.Outcome != domain.OutcomeMoved {
		t.Errorf("Outcome = %q, want %q", moved.Outcome, domain.OutcomeMoved)
	}
	if moved.ID != added.ID {
		t.Errorf("移動で別の行になった: %d -> %d", added.ID, moved.ID)
	}

	total, err := db.Library().CountVideos(ctx, domain.AudienceOwner, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("移動で行が増えた: %d 行, want 1", total)
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if video.Path != "/media/2026/海辺の散歩（編集済み）.mp4" {
		t.Errorf("Path が更新されていない: %q", video.Path)
	}
	if video.Title != "海辺の散歩（編集済み）" {
		t.Errorf("Title が更新されていない: %q", video.Title)
	}
	// 内容は変わっていないので、解析結果は捨てない。
	if video.ProbeState != domain.ProbeStatePending {
		t.Errorf("ProbeState = %q", video.ProbeState)
	}
}

func TestListVideosPagesByRepresentativeLocationTitle(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	for i, title := range []string{"charlie", "alpha", "bravo"} {
		if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/"+title+".mp4", title, fmt.Sprintf("key-%d", i), int64(i+1), 0)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc, Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if first.Items[0].Title != "alpha" || second.Items[0].Title != "bravo" {
		t.Fatalf("pages = %q, %q", first.Items[0].Title, second.Items[0].Title)
	}
}

func TestAddingDuplicateLocationRequestsRecoveryForFailedProcessing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	first, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a/movie.mp4", "movie", "shared", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().MarkProbeFailed(ctx, first.ID, "broken location"); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().SetThumbnailState(ctx, first.ID, domain.ThumbnailStateFailed); err != nil {
		t.Fatal(err)
	}
	second, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/b/movie.mp4", "movie", "shared", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !second.NeedsProbe || !second.NeedsThumbnail {
		t.Fatalf("recovery flags = probe:%v thumbnail:%v", second.NeedsProbe, second.NeedsThumbnail)
	}
}

func TestRepresentativeLocationComesFromRegisteredRoot(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	first, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/legacy/movie.mp4", "legacy", "shared-location", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/current.mp4", "current", "shared-location", 1, 0)); err != nil {
		t.Fatal(err)
	}
	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if video.Path != "/media/current.mp4" || video.Title != "current" {
		t.Fatalf("representative = %q (%q)", video.Path, video.Title)
	}
}

// 存在しない id は「無い」と分かる誤りを返す。
func TestGetVideoReportsMissing(t *testing.T) {
	db := migratedDB(t)

	_, err := db.Library().GetVideo(context.Background(), domain.AudienceOwner, 12345)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want domain.ErrNotFound", err)
	}
}

// 解析の結果を反映する。再生可否は domain の判定をそのまま書き込む。
func TestApplyProbeStoresFactsAndPlayability(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mkv", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}

	probe := domain.Probe{DurationMs: 8123, Width: 1280, Height: 720, DisplayAspectRatio: 16.0 / 9, VideoCodec: "h264", AudioCodec: "aac"}
	play := domain.EvaluatePlayability("mkv", probe)
	if err := db.Ingest().ApplyProbe(ctx, added.ID, probe, play); err != nil {
		t.Fatal(err)
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if video.ProbeState != domain.ProbeStateDone {
		t.Errorf("ProbeState = %q, want done", video.ProbeState)
	}
	if video.DurationMs == nil || *video.DurationMs != 8123 {
		t.Errorf("DurationMs = %v, want 8123", video.DurationMs)
	}
	if video.Width == nil || *video.Width != 1280 {
		t.Errorf("Width = %v, want 1280", video.Width)
	}
	if video.DisplayAspectRatio == nil || *video.DisplayAspectRatio != 16.0/9 {
		t.Errorf("DisplayAspectRatio = %v, want 16/9", video.DisplayAspectRatio)
	}
	if video.Playable {
		t.Error("mkv が再生できる扱いになっている")
	}
	if video.UnplayableReason != domain.ReasonContainer {
		t.Errorf("UnplayableReason = %q, want container", video.UnplayableReason)
	}
}

// 尺が取れなかった場合は null のままにする。0 で代用すると、一覧で
// 「尺が 0 の動画」と「尺が分からない動画」を区別できなくなる。
func TestApplyProbeKeepsUnknownDurationNull(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, added.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if video.DurationMs != nil {
		t.Errorf("DurationMs = %v, want nil", *video.DurationMs)
	}
	if video.Width != nil || video.Height != nil {
		t.Errorf("解像度が 0 で埋まっている: %v x %v", video.Width, video.Height)
	}
}

// 解析に失敗した動画は、理由を添えて failed にする。取り込み全体は止めない
// ので、一覧には並んだままになる。
func TestMarkProbeFailedKeepsRowAndRecordsReason(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/壊れた動画.mp4", "壊れた動画", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().MarkProbeFailed(ctx, added.ID, "ffprobe が尺を返しませんでした"); err != nil {
		t.Fatal(err)
	}

	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, added.ID)
	if err != nil {
		t.Fatalf("失敗した動画が一覧から消えた: %v", err)
	}
	if video.ProbeState != domain.ProbeStateFailed {
		t.Errorf("ProbeState = %q, want failed", video.ProbeState)
	}
	if video.ProbeError == "" {
		t.Error("失敗の理由が記録されていない")
	}
	if video.Playable {
		t.Error("解析に失敗したのに再生できる扱いになっている")
	}
}

// listFixture は並び順の検証用に、追加順と題名順が食い違う行を入れる。
func listFixture(t *testing.T) (*DB, []int64) {
	t.Helper()

	db := migratedDB(t)
	ctx := context.Background()

	rows := []struct {
		title string
		key   string
	}{
		{"ちらし", "key-1"},
		{"あさがお", "key-2"},
		{"はなび", "key-3"},
		{"いちご", "key-4"},
		{"うみ", "key-5"},
	}

	ids := make([]int64, 0, len(rows))
	for i, row := range rows {
		// added_at が行ごとに違わないと、追加順の並びが id 頼みになる。
		got, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path:       fmt.Sprintf("/media/%s.mp4", row.title),
			Title:      row.title,
			ContentKey: row.key,
			SizeBytes:  int64(1000 + i),
			MTime:      fixedTime,
			AddedAt:    fixedTime.Add(time.Duration(i) * time.Minute),
			Container:  "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, got.ID)
	}
	return db, ids
}

// titlesOf は一覧の題名を順に取り出す。
func titlesOf(page domain.VideoPage) []string {
	out := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.Title)
	}
	return out
}

// 並び順のうち、以前からある「追加が新しい順」と「題名順（自然順）」。
func TestListVideosSortOrders(t *testing.T) {
	db, _ := listFixture(t)
	ctx := context.Background()

	added, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortAddedDesc})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"うみ", "いちご", "はなび", "あさがお", "ちらし"}
	if got := titlesOf(added); !equalStrings(got, want) {
		t.Errorf("addedDesc = %v, want %v", got, want)
	}

	byTitle, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: domain.SortTitleAsc})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"あさがお", "いちご", "うみ", "ちらし", "はなび"}
	if got := titlesOf(byTitle); !equalStrings(got, want) {
		t.Errorf("titleAsc = %v, want %v", got, want)
	}
}

// 既定の件数は 60、上限は 200。上限を越える指定は上限に丸める。
func TestListVideosLimitDefaultsAndCaps(t *testing.T) {
	db, _ := listFixture(t)
	ctx := context.Background()

	for _, tc := range []struct {
		given int
		want  int
	}{
		{0, domain.DefaultLimit},
		{-1, domain.DefaultLimit},
		{10, 10},
		{domain.MaxLimit, domain.MaxLimit},
		{domain.MaxLimit + 1, domain.MaxLimit},
		{100000, domain.MaxLimit},
	} {
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Limit: tc.given})
		if err != nil {
			t.Fatalf("limit=%d: %v", tc.given, err)
		}
		if page.Limit != tc.want {
			t.Errorf("limit=%d: 実際に使われた件数 = %d, want %d", tc.given, page.Limit, tc.want)
		}
	}

	if domain.DefaultLimit != 60 {
		t.Errorf("DefaultLimit = %d, want 60", domain.DefaultLimit)
	}
	if domain.MaxLimit != 200 {
		t.Errorf("MaxLimit = %d, want 200", domain.MaxLimit)
	}
}

// カーソルで続きから取れること。並び順の値と id を境界にするので、
// 途中で行が増減しても取りこぼしと重複が起きない。
func TestListVideosPagesWithCursor(t *testing.T) {
	for _, sort := range []domain.VideoSort{domain.SortAddedDesc, domain.SortTitleAsc} {
		t.Run(string(sort), func(t *testing.T) {
			db, _ := listFixture(t)
			ctx := context.Background()

			var seen []string
			cursor := ""
			for page := 0; page < 10; page++ {
				got, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: sort, Limit: 2, Cursor: cursor})
				if err != nil {
					t.Fatal(err)
				}
				seen = append(seen, titlesOf(got)...)
				if got.NextCursor == "" {
					break
				}
				cursor = got.NextCursor
			}

			all, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Sort: sort, Limit: domain.MaxLimit})
			if err != nil {
				t.Fatal(err)
			}
			if want := titlesOf(all); !equalStrings(seen, want) {
				t.Errorf("ページを繋いだ結果 = %v, want %v", seen, want)
			}
		})
	}
}

// 最後のページでは次のカーソルを返さない。返し続けると無限スクロールが
// 止まらなくなる。
func TestListVideosStopsAtLastPage(t *testing.T) {
	db, _ := listFixture(t)

	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "" {
		t.Errorf("最後のページで nextCursor が返った: %q", page.NextCursor)
	}
}

// total は絞り込み後の総件数であり、ページの件数とは独立している。
func TestListVideosTotalIsIndependentOfPageSize(t *testing.T) {
	db, _ := listFixture(t)

	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Errorf("items = %d 件, want 2", len(page.Items))
	}
	if page.Total != 5 {
		t.Errorf("total = %d, want 5", page.Total)
	}
}

// 壊れたカーソルは誤りとして返す。黙って先頭から返すと、無限スクロールが
// 巻き戻って同じ内容を延々と表示することになる。
func TestListVideosRejectsBrokenCursor(t *testing.T) {
	db, _ := listFixture(t)

	for _, cursor := range []string{"not-base64!!", "***", "YWJj", "'; drop table videos; --"} {
		_, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Cursor: cursor})
		if !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("cursor=%q: err = %v, want domain.ErrInvalidCursor", cursor, err)
		}
	}
}

// 行の削除。消えたファイルを索引から落とす。
func TestDeleteVideos(t *testing.T) {
	db, ids := listFixture(t)
	ctx := context.Background()

	if err := db.ScanIndex().DeleteVideos(ctx, ids[:2]); err != nil {
		t.Fatal(err)
	}

	total, err := db.Library().CountVideos(ctx, domain.AudienceOwner, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("残り = %d 行, want 3", total)
	}

	// 空の指定で全件消してしまわないこと。
	if err := db.ScanIndex().DeleteVideos(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if total, _ := db.Library().CountVideos(ctx, domain.AudienceOwner, ""); total != 3 {
		t.Errorf("空の指定で行が消えた: %d 行, want 3", total)
	}
}

// 索引に入っているパスの一覧を取れること。走査は実際のファイルとこれを
// 突き合わせて差分を出す。
func TestIndexedVideosByPath(t *testing.T) {
	db, _ := listFixture(t)

	indexed, err := db.ScanIndex().IndexedVideosByPath(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed) != 5 {
		t.Fatalf("%d 件, want 5", len(indexed))
	}

	got, ok := indexed["/media/うみ.mp4"]
	if !ok {
		t.Fatal("パスで引けない")
	}
	if got.ContentKey != "key-5" {
		t.Errorf("ContentKey = %q, want key-5", got.ContentKey)
	}
	if got.SizeBytes == 0 || got.MTime.IsZero() {
		t.Errorf("差分判定に要る値が欠けている: %+v", got)
	}
}

// 生成中に動画が消えたかどうかを、内容の識別子1つで確かめられること。
func TestContentKeyReferenced(t *testing.T) {
	db, _ := listFixture(t)
	ctx := context.Background()

	for key, want := range map[string]bool{"key-3": true, "消えた内容": false} {
		got, err := db.Library().ContentKeyReferenced(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("ContentKeyReferenced(%q) = %v, want %v", key, got, want)
		}
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// 同じ内容の場所を後から足して代表場所が入れ替わる場合。取り込みは新しい行を
// 作らず場所だけを足すので、container は既存の値のまま残りうる。
// assertRepresentativeInvariant が働くのはこの状態である（invariants_test.go）。
func TestUpsertVideoResyncsWhenAddedLocationBecomesRepresentative(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/b.mkv", "movie", "same", 10, 0)); err != nil {
		t.Fatal(err)
	}
	// /media/a.mp4 はパス順で先にくるので、足した時点で代表場所になる。
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "movie", "same", 10, 0)); err != nil {
		t.Fatal(err)
	}

	var container string
	if err := db.sql.QueryRow(
		`select coalesce(container, '') from videos where content_key = 'same'`).Scan(&container); err != nil {
		t.Fatal(err)
	}
	if want := domain.ContainerFromPath("/media/a.mp4"); container != want {
		t.Fatalf("代表場所が入れ替わっても container が古いまま: 得 %q / 期待 %q", container, want)
	}
}

// 代表場所を削除して、残った別拡張子の場所が代表になる場合。
// 削除経路にも同じ再同期が要る。
func TestDeleteVideoLocationsResyncsRemainingRepresentative(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mkv", "movie", "same", 10, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/b.mp4", "movie", "same", 10, 0)); err != nil {
		t.Fatal(err)
	}

	var locationID int64
	if err := db.sql.QueryRow(
		`select id from video_locations where path = '/media/a.mkv'`).Scan(&locationID); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{locationID}); err != nil {
		t.Fatal(err)
	}

	var container string
	if err := db.sql.QueryRow(
		`select coalesce(container, '') from videos where content_key = 'same'`).Scan(&container); err != nil {
		t.Fatal(err)
	}
	if want := domain.ContainerFromPath("/media/b.mp4"); container != want {
		t.Fatalf("代表場所の削除後に container が古いまま: 得 %q / 期待 %q", container, want)
	}
}

// releasedRecorder は発行のうち、参照の無くなった内容の識別子を記録する。
type releasedRecorder struct {
	keys []string
}

func (r *releasedRecorder) Publish(events ...domain.Event) {
	for _, event := range events {
		switch event := event.(type) {
		case domain.VideoIngestChanged:
			if event.VideoID == 0 {
				panic("消した動画の id が無い")
			}
		case domain.ContentUnreferenced:
			r.keys = append(r.keys, event.ContentKeys...)
		}
	}
}

func (r *releasedRecorder) take() []string {
	keys := r.keys
	r.keys = nil
	slices.Sort(keys)
	return keys
}

// 動画の行を消したら、消した動画の内容の識別子を知らせる。生成物の片付けは
// この知らせだけで行い、ライブラリ全体は読まない。所在が残っていて動画の行が
// 消えないときは知らせない。
func TestContentReleasedWhenVideoRowsAreDeleted(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	recorder := &releasedRecorder{}
	db.PublishTo(recorder)

	for _, file := range []domain.VideoFile{
		sampleFile("/media/a.mp4", "a", "key-a", 1, 0),
		sampleFile("/media/a2.mp4", "a", "key-a", 1, 0),
		sampleFile("/media/b.mp4", "b", "key-b", 2, 0),
		sampleFile("/media/c.mp4", "c", "key-c", 3, 0),
	} {
		if _, err := db.ScanIndex().UpsertVideo(ctx, file); err != nil {
			t.Fatal(err)
		}
	}
	location := func(path string) int64 {
		t.Helper()
		var id int64
		if err := db.sql.QueryRow(`select id from video_locations where path = ?`, path).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	// 同じ内容の所在が残るので、動画の行は消えない。
	if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{location("/media/a2.mp4")}); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); len(got) != 0 {
		t.Fatalf("動画の行が残るのに知らせた: %v", got)
	}

	// 最後の所在が消えると、動画の行と一緒に知らせる。
	if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{location("/media/b.mp4")}); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[key-b]" {
		t.Fatalf("DeleteVideoLocations の知らせ = %v, want [key-b]", got)
	}

	// 同じ所在の内容が変わると、前の内容の動画が消える。
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/c.mp4", "c", "key-c2", 4, time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[key-c]" {
		t.Fatalf("内容の変更の知らせ = %v, want [key-c]", got)
	}

	// 登録を外すと、その下の動画がすべて消える。
	folders, err := db.Settings().ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Settings().DeleteMediaFolder(ctx, folders[0].ID, folders[0].Version); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[key-a key-c2]" {
		t.Fatalf("DeleteMediaFolder の知らせ = %v, want [key-a key-c2]", got)
	}
}
