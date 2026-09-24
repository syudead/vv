package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

// fakeIndex は保存層の代わりに、走査が何をしたかを覚えておく。
// 走査規則そのものを検証したいので、SQLite には触れない。
type fakeIndex struct {
	folders []domain.MediaFolder
	rows    map[string]domain.IndexedVideo // path -> 行
	nextID  int64
	upserts []domain.VideoFile
	deleted []int64
	jobs    []jobCall
	// keyCalls は content_key を計算したパス。変わっていないファイルで
	// 再計算していないことを確かめるために数える。
	progress  []domain.ScanResult
	reportErr error
}

func (f *fakeIndex) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return f.folders, nil
}

type jobCall struct {
	kind    domain.JobKind
	videoID int64
}

func newFakeIndex() *fakeIndex {
	return &fakeIndex{rows: map[string]domain.IndexedVideo{}, nextID: 1}
}

func (f *fakeIndex) IndexedVideosByPath(context.Context) (map[string]domain.IndexedVideo, error) {
	out := map[string]domain.IndexedVideo{}
	for path, row := range f.rows {
		out[path] = row
	}
	return out, nil
}

func (f *fakeIndex) UpsertVideo(_ context.Context, file domain.VideoFile) (domain.UpsertResult, error) {
	f.upserts = append(f.upserts, file)

	for path, row := range f.rows {
		if row.ContentKey == file.ContentKey {
			delete(f.rows, path)
			f.rows[file.Path] = domain.IndexedVideo{
				ID: row.ID, LocationID: row.LocationID, ContentKey: file.ContentKey, SizeBytes: file.SizeBytes, MTime: file.MTime,
				ProbeState: row.ProbeState, ThumbnailState: row.ThumbnailState,
				PreviewState: row.PreviewState,
			}
			outcome := domain.OutcomeMoved
			if path == file.Path {
				outcome = domain.OutcomeUnchanged
			}
			return domain.UpsertResult{ID: row.ID, Outcome: outcome}, nil
		}
	}

	if row, ok := f.rows[file.Path]; ok {
		f.rows[file.Path] = domain.IndexedVideo{
			ID: row.ID, LocationID: row.LocationID, ContentKey: file.ContentKey, SizeBytes: file.SizeBytes, MTime: file.MTime,
			ProbeState: domain.ProbeStatePending, ThumbnailState: domain.ThumbnailStatePending, PreviewState: domain.PreviewStatePending,
		}
		return domain.UpsertResult{ID: row.ID, Outcome: domain.OutcomeUpdated, NeedsProbe: true, NeedsThumbnail: true}, nil
	}

	id := f.nextID
	f.nextID++
	f.rows[file.Path] = domain.IndexedVideo{
		ID: id, LocationID: id, ContentKey: file.ContentKey, SizeBytes: file.SizeBytes, MTime: file.MTime,
		ProbeState: domain.ProbeStatePending, ThumbnailState: domain.ThumbnailStatePending, PreviewState: domain.PreviewStatePending,
	}
	return domain.UpsertResult{ID: id, Outcome: domain.OutcomeAdded, NeedsProbe: true, NeedsThumbnail: true}, nil
}

func (f *fakeIndex) DeleteVideos(_ context.Context, ids []int64) error {
	f.deleted = append(f.deleted, ids...)
	for path, row := range f.rows {
		for _, id := range ids {
			if row.ID == id {
				delete(f.rows, path)
			}
		}
	}
	return nil
}

func (f *fakeIndex) DeleteVideoLocations(ctx context.Context, ids []int64) error {
	return f.DeleteVideos(ctx, ids)
}

func (f *fakeIndex) EnqueueJob(_ context.Context, kind domain.JobKind, videoID int64) error {
	f.jobs = append(f.jobs, jobCall{kind: kind, videoID: videoID})
	return nil
}

func (f *fakeIndex) EnsureJob(_ context.Context, kind domain.JobKind, videoID int64) error {
	for _, job := range f.jobs {
		if job.kind == kind && job.videoID == videoID {
			return nil
		}
	}
	f.jobs = append(f.jobs, jobCall{kind: kind, videoID: videoID})
	return nil
}

func (f *fakeIndex) ReportScanProgress(_ context.Context, result domain.ScanResult) error {
	f.progress = append(f.progress, result)
	return f.reportErr
}

// upsertedPaths は取り込もうとしたパスを並べて返す。
func (f *fakeIndex) upsertedPaths() []string {
	out := make([]string, 0, len(f.upserts))
	for _, file := range f.upserts {
		out = append(out, file.Path)
	}
	sort.Strings(out)
	return out
}

// mediaTree は検証用のファイルを一時ディレクトリに作る。
func mediaTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// runScan は走査を1回実行する。
func runScan(t *testing.T, root string, index *fakeIndex) domain.ScanResult {
	t.Helper()

	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("走査に失敗した: %v", err)
	}
	return result
}

// 対象の拡張子。再生できない形式も取り込む。一覧に出したうえで「再生できない」
// と示すためで、一覧から消してしまうと利用者は手元に何があるか分からない。
func TestScanIncludesAllSupportedExtensions(t *testing.T) {
	files := map[string]string{}
	for _, ext := range []string{
		".mp4", ".m4v", ".webm", ".mkv", ".mov", ".avi", ".wmv", ".flv", ".ts", ".mpg", ".mpeg",
	} {
		files["video"+ext] = "content" + ext
	}
	// 大文字の拡張子も同じ扱いにする。
	files["大文字.MP4"] = "大文字"
	root := mediaTree(t, files)

	index := newFakeIndex()
	result := runScan(t, root, index)

	if result.Total != len(files) {
		t.Errorf("Total = %d, want %d: %v", result.Total, len(files), index.upsertedPaths())
	}
	if result.Added != len(files) {
		t.Errorf("Added = %d, want %d", result.Added, len(files))
	}
}

// 除外の規則。走査の対象から外すものを1つずつ確かめる。
func TestScanExcludesNonMediaAndHiddenEntries(t *testing.T) {
	root := mediaTree(t, map[string]string{
		"見える.mp4":            "ok",
		"メモ.txt":             "対象外の拡張子",
		"画像.jpg":             "対象外の拡張子",
		".hidden.mp4":        "隠しファイル",
		".hidden/中身.mp4":     "隠しディレクトリの中",
		"@eaDir/thumb.mp4":   "NAS のサムネイル置き場",
		"#recycle/消した.mp4":   "NAS のごみ箱",
		"lost+found/破片.mp4":  "fsck の置き場",
		"途中.mp4.part":        "書き込み途中",
		"途中2.mp4.crdownload": "書き込み途中",
		"途中3.mp4.tmp":        "書き込み途中",
		"入れ子/さらに/見える2.mp4":   "再帰的に走る",
	})

	index := newFakeIndex()
	result := runScan(t, root, index)

	want := []string{
		filepath.Join(root, "入れ子/さらに/見える2.mp4"),
		filepath.Join(root, "見える.mp4"),
	}
	got := index.upsertedPaths()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("取り込んだパス = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("取り込んだパス[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
}

// 題名は拡張子を除いたファイル名。
func TestScanDerivesTitleFromFileName(t *testing.T) {
	root := mediaTree(t, map[string]string{
		"夏休みの旅行.mp4":        "a",
		"入れ子/second.v2.mp4": "b",
	})

	index := newFakeIndex()
	runScan(t, root, index)

	titles := map[string]string{}
	for _, file := range index.upserts {
		titles[filepath.Base(file.Path)] = file.Title
	}
	if got := titles["夏休みの旅行.mp4"]; got != "夏休みの旅行" {
		t.Errorf("Title = %q, want 夏休みの旅行", got)
	}
	// 最後の拡張子だけを外す。"second.v2" は題名の一部である。
	if got := titles["second.v2.mp4"]; got != "second.v2" {
		t.Errorf("Title = %q, want second.v2", got)
	}
}

// 実在pathはファイルシステムへ再入力できる綴りを保ち、表示用titleだけをNFC化する。
func TestScanPreservesPathAndNormalizesTitleToNFC(t *testing.T) {
	// "が" を NFD（か + 濁点）で作る。
	decomposed := norm.NFD.String("がっこう.mp4")
	if decomposed == norm.NFC.String(decomposed) {
		t.Skip("この環境では NFD と NFC が一致するため検証できない")
	}

	root := mediaTree(t, map[string]string{decomposed: "a"})

	index := newFakeIndex()
	runScan(t, root, index)

	if len(index.upserts) != 1 {
		t.Fatalf("取り込んだ数 = %d, want 1", len(index.upserts))
	}
	got := index.upserts[0]
	wantPath := filepath.Join(root, decomposed)
	if got.Path != wantPath {
		t.Errorf("Path = %q, want exact filesystem path %q", got.Path, wantPath)
	}
	if got.Title != norm.NFC.String(got.Title) {
		t.Errorf("Title が NFC でない: %q", got.Title)
	}
}

// サイズも mtime も変わらず解析済みの既存行は何もしない。content_key の
// 再計算もしないので、2 回目以降の走査は比較だけで済む。
func TestScanSkipsUnchangedFiles(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})

	index := newFakeIndex()
	first := runScan(t, root, index)
	if first.Added != 1 {
		t.Fatalf("1 回目: Added = %d, want 1", first.Added)
	}
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.PreviewState = domain.PreviewStateDone
		index.rows[path] = row
	}

	upsertsAfterFirst := len(index.upserts)
	second := runScan(t, root, index)

	if second.Total != 0 || second.Completed() != 0 {
		t.Errorf("2 回目の進捗 = %d / %d, want 0 / 0", second.Completed(), second.Total)
	}
	if second.Added != 0 || second.Updated != 0 || second.Moved != 0 {
		t.Errorf("2 回目に変化が記録された: %+v", second)
	}
	if len(index.upserts) != upsertsAfterFirst {
		t.Errorf("変わっていないファイルを取り込み直した: %d 回 -> %d 回",
			upsertsAfterFirst, len(index.upserts))
	}
	// 解析のジョブも積み直さない。積むと毎回の走査で ffprobe が走る。
	jobsAfterFirst := 2
	if len(index.jobs) != jobsAfterFirst {
		t.Errorf("ジョブ = %d 件, want %d（変化が無ければ積み直さない）",
			len(index.jobs), jobsAfterFirst)
	}
}

func TestScanRequeuesMissingJobsForUnchangedPendingVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)

	upsertsAfterFirst := len(index.upserts)
	index.jobs = nil // EnqueueJob が一時的に失敗して、DBにジョブが無い状態を再現する。
	runScan(t, root, index)

	if len(index.upserts) != upsertsAfterFirst {
		t.Fatalf("unchanged file was rehashed: upserts = %d, want %d", len(index.upserts), upsertsAfterFirst)
	}
	if len(index.jobs) != 2 {
		t.Fatalf("requeued jobs = %d, want 2", len(index.jobs))
	}
}

func TestScanRequeuesMissingPreviewForProbeCompleteVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.PreviewState = domain.PreviewStatePending
		index.rows[path] = row
	}
	index.jobs = nil

	runScan(t, root, index)
	if len(index.jobs) != 1 || index.jobs[0].kind != domain.JobPreview {
		t.Fatalf("requeued jobs = %+v, want one preview job", index.jobs)
	}
}

func TestScanDoesNotRequeueFailedJobsForUnchangedVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateFailed
		row.ThumbnailState = domain.ThumbnailStateFailed
		index.rows[path] = row
	}
	index.jobs = nil

	runScan(t, root, index)
	if len(index.jobs) != 0 {
		t.Fatalf("failed jobs were requeued: %d", len(index.jobs))
	}
}

// 内容が変われば取り込み直す。
func TestScanUpdatesChangedFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})

	index := newFakeIndex()
	runScan(t, root, index)

	path := filepath.Join(root, "a.mp4")
	if err := os.WriteFile(path, []byte("差し替えた内容（長さも変える）"), 0o600); err != nil {
		t.Fatal(err)
	}
	// mtime も進める。
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	result := runScan(t, root, index)
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1: %+v", result.Updated, result)
	}
	if result.Total != 1 || result.Completed() != 1 {
		t.Errorf("進捗 = %d / %d, want 1 / 1", result.Completed(), result.Total)
	}
}

// 移動・改名は、消えたパスと新しいパスの content_key が一致することで
// 判定し、パスの更新として扱う。重複を作らない。
func TestScanDetectsMoveWithoutDuplicating(t *testing.T) {
	root := mediaTree(t, map[string]string{"海辺の散歩.mp4": "同じ内容"})

	index := newFakeIndex()
	runScan(t, root, index)

	from := filepath.Join(root, "海辺の散歩.mp4")
	to := filepath.Join(root, "2026", "海辺の散歩（編集済み）.mp4")
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}

	result := runScan(t, root, index)

	if result.Moved != 1 {
		t.Errorf("Moved = %d, want 1: %+v", result.Moved, result)
	}
	if result.Removed != 0 {
		t.Errorf("移動なのに削除が記録された: %d", result.Removed)
	}
	if len(index.deleted) != 0 {
		t.Errorf("移動で行が消された: %v", index.deleted)
	}
	if len(index.rows) != 1 {
		t.Errorf("行が %d 件になった, want 1", len(index.rows))
	}
}

// 消えたファイルは行を削除する。
func TestScanRemovesMissingFiles(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "b.mp4": "b"})

	index := newFakeIndex()
	runScan(t, root, index)

	if err := os.Remove(filepath.Join(root, "b.mp4")); err != nil {
		t.Fatal(err)
	}

	result := runScan(t, root, index)
	if result.Removed != 1 {
		t.Errorf("Removed = %d, want 1", result.Removed)
	}
	if len(index.rows) != 1 {
		t.Errorf("行が %d 件残った, want 1", len(index.rows))
	}
}

// 新しく取り込んだ動画には、解析とサムネイルのジョブを積む。
func TestScanEnqueuesJobsForNewVideos(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a"})

	index := newFakeIndex()
	runScan(t, root, index)

	kinds := map[domain.JobKind]int{}
	for _, job := range index.jobs {
		kinds[job.kind]++
	}
	if kinds[domain.JobProbe] != 1 {
		t.Errorf("probe のジョブ = %d 件, want 1", kinds[domain.JobProbe])
	}
	if kinds[domain.JobThumbnail] != 1 {
		t.Errorf("thumbnail のジョブ = %d 件, want 1", kinds[domain.JobThumbnail])
	}
}

// 個別のファイルの失敗で走査全体を止めない。読めないファイルは
// failed に数えて次へ進む。
func TestScanContinuesAfterFileFailure(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "b.mp4": "b", "c.mp4": "c"})
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.contentKey = func(path string) (string, error) {
		if filepath.Base(path) == "b.mp4" {
			return "", &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
		}
		return ContentKey(path)
	}
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || result.Completed() != 2 || result.Failed != 1 || result.Added != 2 {
		t.Fatalf("want completed=2 total=3 failed=1 added=2, got %+v", result)
	}
	paths := index.upsertedPaths()
	if len(paths) != 2 || paths[0] != filepath.Join(root, "a.mp4") || paths[1] != filepath.Join(root, "c.mp4") {
		t.Fatalf("files before and after the failure must be indexed: %v", paths)
	}
}

func TestScanContinuesAfterPermissionFailure(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "壊れた.mp4": "b", "c.mp4": "c"})

	// 読み取り権限を落として、鍵の計算を失敗させる。
	broken := filepath.Join(root, "壊れた.mp4")
	if err := os.Chmod(broken, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(broken, 0o600) })
	if file, err := os.Open(broken); err == nil {
		_ = file.Close()
		t.Skip("この環境では chmod で読み取りを禁止できない（root / Windows）")
	}

	index := newFakeIndex()
	result := runScan(t, root, index)

	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1: %+v", result.Failed, result)
	}
	if result.Added != 2 {
		t.Errorf("Added = %d, want 2（1件の失敗で残りが止まってはならない）", result.Added)
	}
}

// 動画ファイルは読み取りのみで扱う。変更・移動・削除をしない。
func TestScanDoesNotModifyMediaFiles(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "入れ子/b.mkv": "b"})

	before := snapshot(t, root)
	index := newFakeIndex()
	runScan(t, root, index)
	after := snapshot(t, root)

	if len(before) != len(after) {
		t.Fatalf("ファイルの数が変わった: %d -> %d", len(before), len(after))
	}
	for path, want := range before {
		got, ok := after[path]
		if !ok {
			t.Errorf("%s が無くなった", path)
			continue
		}
		if got != want {
			t.Errorf("%s が書き換えられた: %q -> %q", path, want, got)
		}
	}
}

// 走査中に進捗を報告する。取り込みの規模と残りが見える。
func TestScanReportsProgress(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "b.mp4": "b"})

	index := newFakeIndex()
	runScan(t, root, index)

	if len(index.progress) == 0 {
		t.Fatal("進捗が1度も報告されていない")
	}
	first := index.progress[0]
	if first.Total != 2 || first.Completed() != 0 {
		t.Errorf("対象確定時の進捗 = %d / %d, want 0 / 2", first.Completed(), first.Total)
	}
	last := index.progress[len(index.progress)-1]
	if last.Total != 2 || last.Completed() != 2 {
		t.Errorf("最後に報告した進捗 = %d / %d, want 2 / 2", last.Completed(), last.Total)
	}
}

func TestScanProgressCountsOnlyFilesThatNeedImport(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "b.mp4": "b"})
	index := newFakeIndex()
	runScan(t, root, index)
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.PreviewState = domain.PreviewStateDone
		index.rows[path] = row
	}

	changed := filepath.Join(root, "b.mp4")
	if err := os.WriteFile(changed, []byte("changed length"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(changed, later, later); err != nil {
		t.Fatal(err)
	}
	index.progress = nil

	result := runScan(t, root, index)
	if result.Total != 1 || result.Completed() != 1 || result.Updated != 1 {
		t.Fatalf("one changed file should be 1 / 1, got %+v", result)
	}
	if first := index.progress[0]; first.Total != 1 || first.Completed() != 0 {
		t.Fatalf("target discovery progress = %d / %d, want 0 / 1", first.Completed(), first.Total)
	}
}

// 読めないrootは失敗として数え、既存索引を保持する。
func TestScanFailsWhenMediaDirIsUnreadable(t *testing.T) {
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: filepath.Join(t.TempDir(), "missing"), Version: 1}}
	scanner := New(Options{Index: index})

	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("root I/O failure should be isolated: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
}

// context の取り消しで止まること。停止指示を受けたら走査も終える。
func TestScanStopsOnCancel(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 50; i++ {
		files[strings.Repeat("a", i+1)+".mp4"] = "内容"
	}
	root := mediaTree(t, files)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index})
	if _, err := scanner.Scan(ctx); err == nil {
		t.Error("取り消し済みの context で成功した")
	}
}

func TestReportingFailurePreventsMissingDeletion(t *testing.T) {
	root := mediaTree(t, map[string]string{"present.mp4": "present"})
	missing := filepath.Join(root, "missing.mp4")
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	index.rows[missing] = domain.IndexedVideo{ID: 9, LocationID: 19, ContentKey: "missing", SizeBytes: 1, MTime: time.Unix(1, 0)}
	index.reportErr = errors.New("database unavailable")

	_, err := New(Options{Index: index, Reporter: index}).Scan(context.Background())
	if err == nil {
		t.Fatal("reporting failure was ignored")
	}
	if len(index.deleted) != 0 {
		t.Fatalf("deleted locations after reporting failure: %v", index.deleted)
	}
}

func TestSuccessfulScanPreservesMigratedLocationOutsideRegisteredRoots(t *testing.T) {
	root := mediaTree(t, map[string]string{})
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	index.rows[filepath.Join(t.TempDir(), "legacy.mp4")] = domain.IndexedVideo{
		ID: 9, LocationID: 19, ContentKey: "legacy", SizeBytes: 1, MTime: time.Unix(1, 0),
	}
	if _, err := New(Options{Index: index}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(index.deleted) != 0 {
		t.Fatalf("deleted location outside registered roots: %v", index.deleted)
	}
}

func TestWalkErrorOnlySkipsDirectories(t *testing.T) {
	root := t.TempDir()
	failedFile := filepath.Join(root, "b.mp4")
	indexed := map[string]domain.IndexedVideo{
		failedFile:                             {ID: 1},
		filepath.Join(root, "locked", "c.mp4"): {ID: 2},
	}
	if walkErrorIsDirectory(failedFile, root, nil, indexed) {
		t.Fatal("file error would skip the remaining siblings")
	}
	if !walkErrorIsDirectory(filepath.Join(root, "locked"), root, nil, indexed) {
		t.Fatal("directory error would not skip its unreadable subtree")
	}
}

// snapshot はディレクトリ配下のパスと内容を読み取る。
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()

	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
