package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fixedTime はテストの期待値を安定させるための基準時刻である。
var fixedTime = time.Unix(1_757_000_000, 0).UTC()

// sampleFile は走査で分かる事実を1件組み立てる。
func sampleFile(path, title, key string, size int64, offset time.Duration) VideoFile {
	return VideoFile{
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

	got, err := db.UpsertVideo(ctx, sampleFile("/media/海辺の散歩.mp4", "海辺の散歩", "key-a", 1024, 0))
	if err != nil {
		t.Fatalf("取り込めない: %v", err)
	}
	if got.Outcome != OutcomeAdded {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeAdded)
	}
	if got.ID == 0 {
		t.Error("ID が返っていない")
	}

	video, err := db.GetVideo(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if video.Title != "海辺の散歩" {
		t.Errorf("Title = %q", video.Title)
	}
	if video.ContentKey != "key-a" {
		t.Errorf("ContentKey = %q", video.ContentKey)
	}
	// 解析前は「再生できない」側に倒す（R-103）。
	if video.Playable {
		t.Error("解析前なのに再生できる扱いになっている")
	}
	if video.ProbeState != domain.ProbeStatePending {
		t.Errorf("ProbeState = %q, want pending", video.ProbeState)
	}
	if video.ThumbnailState != domain.ThumbnailStatePending {
		t.Errorf("ThumbnailState = %q, want pending", video.ThumbnailState)
	}
	// 取得できていない尺は null のままにする。0 で代用しない（data-model.md）。
	if video.DurationMs != nil {
		t.Errorf("DurationMs = %v, want nil", *video.DurationMs)
	}
}

// 同じパス・同じサイズ・同じ mtime の再取り込みでは何も変わらない。
// 2 回目以降のスキャンを安く保つ前提である（R-107）。
func TestUpsertVideoIsUnchangedWhenNothingMoved(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	file := sampleFile("/media/a.mp4", "a", "key-a", 1024, 0)
	first, err := db.UpsertVideo(ctx, file)
	if err != nil {
		t.Fatal(err)
	}

	second, err := db.UpsertVideo(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != OutcomeUnchanged {
		t.Errorf("Outcome = %q, want %q", second.Outcome, OutcomeUnchanged)
	}
	if second.ID != first.ID {
		t.Errorf("ID が変わった: %d -> %d", first.ID, second.ID)
	}
}

// 内容が変われば（サイズか mtime が変わる）属性を更新し、解析をやり直す。
func TestUpsertVideoUpdatesChangedFileAndResetsProbe(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ApplyProbe(ctx, added.ID, domain.Probe{
		DurationMs: 8000, Width: 640, Height: 360, VideoCodec: "h264", AudioCodec: "aac",
	}, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}

	// 同じパスで内容が差し替わった（content_key も変わる）。
	updated, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a2", 2048, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Outcome != OutcomeUpdated {
		t.Errorf("Outcome = %q, want %q", updated.Outcome, OutcomeUpdated)
	}
	if updated.ID == added.ID {
		t.Errorf("内容が変わったのに論理動画IDが維持された: %d", added.ID)
	}

	video, err := db.GetVideo(ctx, updated.ID)
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

// 移動・改名は行の作り直しではなくパスの更新になる。重複を作らないことが
// FR-004 の要求で、再生位置を引き継ぐ前提でもある。
func TestUpsertVideoTreatsSameContentAtNewPathAsMove(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.UpsertVideo(ctx, sampleFile("/media/海辺の散歩.mp4", "海辺の散歩", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}

	moved, err := db.UpsertVideo(ctx,
		sampleFile("/media/2026/海辺の散歩（編集済み）.mp4", "海辺の散歩（編集済み）", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if moved.Outcome != OutcomeMoved {
		t.Errorf("Outcome = %q, want %q", moved.Outcome, OutcomeMoved)
	}
	if moved.ID != added.ID {
		t.Errorf("移動で別の行になった: %d -> %d", added.ID, moved.ID)
	}

	total, err := db.CountVideos(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("移動で行が増えた: %d 行, want 1", total)
	}

	video, err := db.GetVideo(ctx, added.ID)
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
		if _, err := db.UpsertVideo(ctx, sampleFile("/media/"+title+".mp4", title, fmt.Sprintf("key-%d", i), int64(i+1), 0)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := db.ListVideos(ctx, VideoQuery{Sort: SortTitleAsc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.ListVideos(ctx, VideoQuery{Sort: SortTitleAsc, Limit: 1, Cursor: first.NextCursor})
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
	first, err := db.UpsertVideo(ctx, sampleFile("/media/a/movie.mp4", "movie", "shared", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MarkProbeFailed(ctx, first.ID, "broken location"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetThumbnailState(ctx, first.ID, domain.ThumbnailStateFailed); err != nil {
		t.Fatal(err)
	}
	second, err := db.UpsertVideo(ctx, sampleFile("/media/b/movie.mp4", "movie", "shared", 1, 0))
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
	first, err := db.UpsertVideo(ctx, sampleFile("/legacy/movie.mp4", "legacy", "shared-location", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/current.mp4", "current", "shared-location", 1, 0)); err != nil {
		t.Fatal(err)
	}
	video, err := db.GetVideo(ctx, first.ID)
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

	_, err := db.GetVideo(context.Background(), 12345)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// 解析の結果を反映する。再生可否は domain の判定をそのまま書き込む。
func TestApplyProbeStoresFactsAndPlayability(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.UpsertVideo(ctx, sampleFile("/media/a.mkv", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}

	probe := domain.Probe{DurationMs: 8123, Width: 1280, Height: 720, VideoCodec: "h264", AudioCodec: "aac"}
	play := domain.EvaluatePlayability("mkv", probe)
	if err := db.ApplyProbe(ctx, added.ID, probe, play); err != nil {
		t.Fatal(err)
	}

	video, err := db.GetVideo(ctx, added.ID)
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

	added, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(ctx, added.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}

	video, err := db.GetVideo(ctx, added.ID)
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
// （FR-008）ので、一覧には並んだままになる。
func TestMarkProbeFailedKeepsRowAndRecordsReason(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.UpsertVideo(ctx, sampleFile("/media/壊れた動画.mp4", "壊れた動画", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MarkProbeFailed(ctx, added.ID, "ffprobe が尺を返しませんでした"); err != nil {
		t.Fatal(err)
	}

	video, err := db.GetVideo(ctx, added.ID)
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
		got, err := db.UpsertVideo(ctx, VideoFile{
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
func titlesOf(page VideoPage) []string {
	out := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.Title)
	}
	return out
}

// 並び順は「追加が新しい順」と「題名順」の2つ（R-109）。
func TestListVideosSortOrders(t *testing.T) {
	db, _ := listFixture(t)
	ctx := context.Background()

	added, err := db.ListVideos(ctx, VideoQuery{Sort: SortAddedDesc})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"うみ", "いちご", "はなび", "あさがお", "ちらし"}
	if got := titlesOf(added); !equalStrings(got, want) {
		t.Errorf("addedDesc = %v, want %v", got, want)
	}

	byTitle, err := db.ListVideos(ctx, VideoQuery{Sort: SortTitleAsc})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"あさがお", "いちご", "うみ", "ちらし", "はなび"}
	if got := titlesOf(byTitle); !equalStrings(got, want) {
		t.Errorf("titleAsc = %v, want %v", got, want)
	}
}

// 既定の件数は 60、上限は 200（R-109）。上限を越える指定は上限に丸める。
func TestListVideosLimitDefaultsAndCaps(t *testing.T) {
	db, _ := listFixture(t)
	ctx := context.Background()

	for _, tc := range []struct {
		given int
		want  int
	}{
		{0, DefaultLimit},
		{-1, DefaultLimit},
		{10, 10},
		{MaxLimit, MaxLimit},
		{MaxLimit + 1, MaxLimit},
		{100000, MaxLimit},
	} {
		page, err := db.ListVideos(ctx, VideoQuery{Limit: tc.given})
		if err != nil {
			t.Fatalf("limit=%d: %v", tc.given, err)
		}
		if page.Limit != tc.want {
			t.Errorf("limit=%d: 実際に使われた件数 = %d, want %d", tc.given, page.Limit, tc.want)
		}
	}

	if DefaultLimit != 60 {
		t.Errorf("DefaultLimit = %d, want 60", DefaultLimit)
	}
	if MaxLimit != 200 {
		t.Errorf("MaxLimit = %d, want 200", MaxLimit)
	}
}

// カーソルで続きから取れること。並び順の値と id を境界にするので、
// 途中で行が増減しても取りこぼしと重複が起きない（R-109）。
func TestListVideosPagesWithCursor(t *testing.T) {
	for _, sort := range []VideoSort{SortAddedDesc, SortTitleAsc} {
		t.Run(string(sort), func(t *testing.T) {
			db, _ := listFixture(t)
			ctx := context.Background()

			var seen []string
			cursor := ""
			for page := 0; page < 10; page++ {
				got, err := db.ListVideos(ctx, VideoQuery{Sort: sort, Limit: 2, Cursor: cursor})
				if err != nil {
					t.Fatal(err)
				}
				seen = append(seen, titlesOf(got)...)
				if got.NextCursor == "" {
					break
				}
				cursor = got.NextCursor
			}

			all, err := db.ListVideos(ctx, VideoQuery{Sort: sort, Limit: MaxLimit})
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

	page, err := db.ListVideos(context.Background(), VideoQuery{Limit: MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "" {
		t.Errorf("最後のページで nextCursor が返った: %q", page.NextCursor)
	}
}

// total は絞り込み後の総件数であり、ページの件数とは独立している（FR-012）。
func TestListVideosTotalIsIndependentOfPageSize(t *testing.T) {
	db, _ := listFixture(t)

	page, err := db.ListVideos(context.Background(), VideoQuery{Limit: 2})
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
// 巻き戻って同じ内容を延々と表示することになる（contracts/http-routes.md）。
func TestListVideosRejectsBrokenCursor(t *testing.T) {
	db, _ := listFixture(t)

	for _, cursor := range []string{"not-base64!!", "***", "YWJj", "'; drop table videos; --"} {
		_, err := db.ListVideos(context.Background(), VideoQuery{Cursor: cursor})
		if !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("cursor=%q: err = %v, want ErrInvalidCursor", cursor, err)
		}
	}
}

// 行の削除。消えたファイルを索引から落とす（FR-005）。
func TestDeleteVideos(t *testing.T) {
	db, ids := listFixture(t)
	ctx := context.Background()

	if err := db.DeleteVideos(ctx, ids[:2]); err != nil {
		t.Fatal(err)
	}

	total, err := db.CountVideos(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("残り = %d 行, want 3", total)
	}

	// 空の指定で全件消してしまわないこと。
	if err := db.DeleteVideos(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if total, _ := db.CountVideos(ctx, ""); total != 3 {
		t.Errorf("空の指定で行が消えた: %d 行, want 3", total)
	}
}

// 索引に入っているパスの一覧を取れること。走査は実際のファイルとこれを
// 突き合わせて差分を出す（R-107）。
func TestIndexedVideosByPath(t *testing.T) {
	db, _ := listFixture(t)

	indexed, err := db.IndexedVideosByPath(context.Background())
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

// サムネイルの掃除に使う。参照されている content_key の一覧を取れること。
func TestContentKeys(t *testing.T) {
	db, _ := listFixture(t)

	keys, err := db.ContentKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 5 {
		t.Errorf("%d 件, want 5", len(keys))
	}
	if _, ok := keys["key-3"]; !ok {
		t.Error("key-3 が含まれていない")
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
