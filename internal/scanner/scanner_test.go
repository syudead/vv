package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

// Record scanner effects without SQLite to isolate scan rules.
type fakeIndex struct {
	folders []domain.MediaFolder
	rows    map[string]domain.IndexedVideo // Path to indexed row.
	nextID  int64
	upserts []domain.VideoFile
	deleted []int64
	jobs    []jobCall
	// Deciders align with jobs in enqueue order.
	deciders     []domain.ActivityDecider
	enqueueCalls int
	// Count paths hashed to detect unnecessary work on unchanged files.
	progress       []domain.ScanResult
	reportErr      error
	ensureErr      error
	issues         []domain.ScanFileIssue
	issueErr       error
	reports        []string
	createdAtCalls []createdAtCall
	createdAtErr   error
	// Force mode: forcedBefore is what ForcedVideos returns; calls logs upserts and forced jobs in order.
	forcedBefore []int64
	forceJobs    []jobCall
	marked       []forcedMark
	calls        []string
}

type forcedMark struct {
	scanID, videoID int64
}

func (f *fakeIndex) ForceScanJobs(
	_ context.Context, scanID, videoID int64, _ domain.ActivityDecider, kinds ...domain.JobKind,
) error {
	f.calls = append(f.calls, fmt.Sprintf("force %d", videoID))
	for _, kind := range kinds {
		f.forceJobs = append(f.forceJobs, jobCall{kind: kind, videoID: videoID})
	}
	f.marked = append(f.marked, forcedMark{scanID: scanID, videoID: videoID})
	return nil
}

func (f *fakeIndex) ForcedVideos(context.Context, int64) ([]int64, error) {
	return f.forcedBefore, nil
}

type createdAtCall struct {
	locationID int64
	createdAt  time.Time
}

func (f *fakeIndex) UpdateLocationCreatedAt(_ context.Context, locationID int64, createdAt time.Time) error {
	if f.createdAtErr != nil {
		return f.createdAtErr
	}
	f.createdAtCalls = append(f.createdAtCalls, createdAtCall{locationID: locationID, createdAt: createdAt})
	for path, row := range f.rows {
		if row.LocationID == locationID {
			row.FileCreatedAt = indexedCreatedAt(createdAt)
			f.rows[path] = row
		}
	}
	return nil
}

// Match stored second precision; zero means null.
func indexedCreatedAt(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return time.Unix(t.Unix(), 0)
}

type failingInfoEntry struct {
	fs.DirEntry
}

func (failingInfoEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("metadata unavailable")
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
	f.calls = append(f.calls, "upsert "+filepath.Base(file.Path))

	for path, row := range f.rows {
		if row.ContentKey == file.ContentKey {
			delete(f.rows, path)
			f.rows[file.Path] = domain.IndexedVideo{
				ID: row.ID, LocationID: row.LocationID, ContentKey: file.ContentKey, SizeBytes: file.SizeBytes, MTime: file.MTime,
				FileCreatedAt: indexedCreatedAt(file.FileCreatedAt),
				ProbeState:    row.ProbeState, ThumbnailState: row.ThumbnailState,
				SeekThumbnailState: row.SeekThumbnailState, PreviewState: row.PreviewState,
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
			FileCreatedAt: indexedCreatedAt(file.FileCreatedAt),
			ProbeState:    domain.ProbeStatePending, ThumbnailState: domain.ThumbnailStatePending,
			SeekThumbnailState: domain.SeekThumbnailPending, PreviewState: domain.PreviewStatePending,
		}
		return domain.UpsertResult{
			ID: row.ID, Outcome: domain.OutcomeUpdated, NeedsProbe: true, NeedsThumbnail: true, NeedsSeekThumbnail: true,
		}, nil
	}

	id := f.nextID
	f.nextID++
	f.rows[file.Path] = domain.IndexedVideo{
		ID: id, LocationID: id, ContentKey: file.ContentKey, SizeBytes: file.SizeBytes, MTime: file.MTime,
		FileCreatedAt: indexedCreatedAt(file.FileCreatedAt),
		ProbeState:    domain.ProbeStatePending, ThumbnailState: domain.ThumbnailStatePending,
		SeekThumbnailState: domain.SeekThumbnailPending, PreviewState: domain.PreviewStatePending,
	}
	return domain.UpsertResult{
		ID: id, Outcome: domain.OutcomeAdded, NeedsProbe: true, NeedsThumbnail: true, NeedsSeekThumbnail: true,
	}, nil
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

func (f *fakeIndex) EnqueueJobs(
	_ context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind,
) error {
	f.enqueueCalls++
	for _, kind := range kinds {
		f.jobs = append(f.jobs, jobCall{kind: kind, videoID: videoID})
		f.deciders = append(f.deciders, decider)
	}
	return nil
}

func (f *fakeIndex) EnsureJobs(
	_ context.Context, videoID int64, _ domain.ActivityDecider, kinds ...domain.JobKind,
) error {
	if f.ensureErr != nil {
		return f.ensureErr
	}
	for _, kind := range kinds {
		if !slices.Contains(f.jobs, jobCall{kind: kind, videoID: videoID}) {
			f.jobs = append(f.jobs, jobCall{kind: kind, videoID: videoID})
		}
	}
	return nil
}

func (f *fakeIndex) ReportScanProgress(_ context.Context, result domain.ScanResult) error {
	f.progress = append(f.progress, result)
	f.reports = append(f.reports, fmt.Sprintf("progress %d/%d", result.Completed(), result.Total))
	return f.reportErr
}

func (f *fakeIndex) ReportScanFile(path string, videoID int64) {
	if path == "" {
		f.reports = append(f.reports, "file done")
		return
	}
	f.reports = append(f.reports, fmt.Sprintf("file %s %d", filepath.Base(path), videoID))
}

func (f *fakeIndex) ReportScanIssue(_ context.Context, issue domain.ScanFileIssue) error {
	f.issues = append(f.issues, issue)
	return f.issueErr
}

func (f *fakeIndex) upsertedPaths() []string {
	out := make([]string, 0, len(f.upserts))
	for _, file := range f.upserts {
		out = append(out, file.Path)
	}
	sort.Strings(out)
	return out
}

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

func runScan(t *testing.T, root string, index *fakeIndex) domain.ScanResult {
	t.Helper()

	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err != nil {
		t.Fatalf("走査に失敗した: %v", err)
	}
	return result
}

// Index unsupported formats so the library still shows all files.
func TestScanIncludesAllSupportedExtensions(t *testing.T) {
	files := map[string]string{}
	for _, ext := range []string{
		".mp4", ".m4v", ".webm", ".mkv", ".mov", ".avi", ".wmv", ".flv", ".ts", ".m2ts", ".mts", ".mpg", ".mpeg",
	} {
		files["video"+ext] = "content" + ext
	}
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
	// Remove only the final extension; "second.v2" remains part of the title.
	if got := titles["second.v2.mp4"]; got != "second.v2" {
		t.Errorf("Title = %q, want second.v2", got)
	}
}

// Preserve filesystem path spelling; normalize only display titles to NFC.
func TestScanPreservesPathAndNormalizesTitleToNFC(t *testing.T) {
	// Construct U+304C as NFD: U+304B plus combining dakuten.
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
		row.SeekThumbnailState = domain.SeekThumbnailDone
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
	// Requeuing probes would invoke ffprobe on every scan.
	jobsAfterFirst := 3
	if len(index.jobs) != jobsAfterFirst {
		t.Errorf("ジョブ = %d 件, want %d（変化が無ければ積み直さない）",
			len(index.jobs), jobsAfterFirst)
	}
}

func TestScanDetectsSameSizeUpdateWithinOneSecond(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "before"})
	path := filepath.Join(root, "a.mp4")
	stamp := time.Unix(1_757_000_000, 100_000_000)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	index := newFakeIndex()
	runScan(t, root, index)
	if err := os.WriteFile(path, []byte("after!"), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp = stamp.Add(100 * time.Millisecond)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	second := runScan(t, root, index)
	if second.Updated != 1 || len(index.upserts) != 2 {
		t.Fatalf("subsecond update skipped: result = %+v, upserts = %d", second, len(index.upserts))
	}
	if !index.upserts[1].MTime.Equal(stamp) {
		t.Fatalf("updated timestamp = %v, want %v", index.upserts[1].MTime, stamp)
	}
}

func TestScanRequeuesMissingJobsForUnchangedPendingVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)

	upsertsAfterFirst := len(index.upserts)
	index.jobs = nil // Simulate a transient enqueue failure leaving no stored job.
	runScan(t, root, index)

	if len(index.upserts) != upsertsAfterFirst {
		t.Fatalf("unchanged file was rehashed: upserts = %d, want %d", len(index.upserts), upsertsAfterFirst)
	}
	if len(index.jobs) != 3 {
		t.Fatalf("requeued jobs = %d, want 3", len(index.jobs))
	}
}

func TestScanCountsFailedPendingJobRepairAsTarget(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	index.jobs = nil
	index.ensureErr = errors.New("queue unavailable")

	result := runScan(t, root, index)
	if result.Total != 1 || result.Completed() != 0 || result.Failed != 1 {
		t.Fatalf("failed pending job repair should be 0 / 1 with one failure, got %+v", result)
	}
	path := filepath.Join(root, "a.mp4")
	want := []domain.ScanFileIssue{{VideoID: index.rows[path].ID, Path: path, Kind: domain.IssueRegisterFailed}}
	if !slices.Equal(index.issues, want) {
		t.Fatalf("報告 = %+v, want %+v", index.issues, want)
	}
}

func TestScanReportsEnqueueFailureWithRegisteredVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a"})
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: failingQueue{}, Reporter: index})
	if _, err := scanner.Scan(context.Background(), domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "a.mp4")
	want := []domain.ScanFileIssue{{VideoID: index.rows[path].ID, Path: path, Kind: domain.IssueRegisterFailed}}
	if !slices.Equal(index.issues, want) {
		t.Fatalf("報告 = %+v, want %+v", index.issues, want)
	}
}

type failingQueue struct{}

func (failingQueue) EnqueueJobs(context.Context, int64, domain.ActivityDecider, ...domain.JobKind) error {
	return errors.New("queue unavailable")
}

func (failingQueue) EnsureJobs(context.Context, int64, domain.ActivityDecider, ...domain.JobKind) error {
	return errors.New("queue unavailable")
}

func (failingQueue) ForceScanJobs(context.Context, int64, int64, domain.ActivityDecider, ...domain.JobKind) error {
	return errors.New("queue unavailable")
}

func TestScanRequeuesMissingPreviewForProbeCompleteVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.SeekThumbnailState = domain.SeekThumbnailDone
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
		row.SeekThumbnailState = domain.SeekThumbnailFailed
		index.rows[path] = row
	}
	index.jobs = nil

	runScan(t, root, index)
	if len(index.jobs) != 0 {
		t.Fatalf("failed jobs were requeued: %d", len(index.jobs))
	}
}

func TestScanRequeuesOnlySeekThumbnailForPendingSeekThumbnail(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	var id int64
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.SeekThumbnailState = domain.SeekThumbnailPending
		row.PreviewState = domain.PreviewStateDone
		index.rows[path] = row
		id = row.ID
	}
	index.jobs = nil

	runScan(t, root, index)
	want := []jobCall{{kind: domain.JobSeekThumbnail, videoID: id}}
	if !slices.Equal(index.jobs, want) {
		t.Fatalf("requeued jobs = %+v, want %+v", index.jobs, want)
	}
}

// Exhausted fingerprints are retried on the next scan because no terminal status column exists.
func TestScanRequeuesMissingFingerprint(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	var id int64
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.SeekThumbnailState = domain.SeekThumbnailDone
		row.PreviewState = domain.PreviewStateDone
		row.FingerprintMissing = true
		index.rows[path] = row
		id = row.ID
	}
	index.jobs = nil

	runScan(t, root, index)
	want := []jobCall{{kind: domain.JobFingerprint, videoID: id}}
	if !slices.Equal(index.jobs, want) {
		t.Fatalf("requeued jobs = %+v, want %+v", index.jobs, want)
	}
}

func TestScanDoesNotRequeueFailedSeekThumbnail(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.SeekThumbnailState = domain.SeekThumbnailFailed
		row.PreviewState = domain.PreviewStateDone
		index.rows[path] = row
	}
	index.jobs = nil

	runScan(t, root, index)
	if len(index.jobs) != 0 {
		t.Fatalf("requeued jobs = %+v, want none", index.jobs)
	}
}

func TestScanEnqueuesSeekThumbnailForNewContent(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)
	var kinds []domain.JobKind
	for _, job := range index.jobs {
		kinds = append(kinds, job.kind)
	}
	want := []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail}
	if !slices.Equal(kinds, want) {
		t.Fatalf("enqueued kinds = %v, want %v", kinds, want)
	}
}

func TestScanUpdatesChangedFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})

	index := newFakeIndex()
	runScan(t, root, index)

	path := filepath.Join(root, "a.mp4")
	if err := os.WriteFile(path, []byte("差し替えた内容（長さも変える）"), 0o600); err != nil {
		t.Fatal(err)
	}
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

// recordingSlots records each read slot taken and returned, in the same log as the content-key reads.
type recordingSlots struct {
	events *[]string
}

func (r recordingSlots) AcquireRead(_ context.Context, root string) (func(), error) {
	*r.events = append(*r.events, "acquire "+root)
	return func() { *r.events = append(*r.events, "release "+root) }, nil
}

func TestScanHoldsAReadSlotForEachContentKeyRead(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "unchanged", "b.mp4": "before"})
	index := newFakeIndex()
	runScan(t, root, index)
	b := filepath.Join(root, "b.mp4")
	if err := os.WriteFile(b, []byte("after, with another size"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "c.mp4"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	var events []string
	scanner := New(Options{Index: index, Queue: index, Reporter: index, ReadSlots: recordingSlots{events: &events}})
	scanner.contentKey = func(path string) (string, error) {
		events = append(events, "read "+filepath.Base(path))
		return ContentKey(path)
	}
	if _, err := scanner.Scan(context.Background(), domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	// The unchanged a.mp4 is not read and takes no slot.
	want := []string{
		"acquire " + root, "read b.mp4", "release " + root,
		"acquire " + root, "read c.mp4", "release " + root,
	}
	if !slices.Equal(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

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

func TestScanRemovesIndexedFileDeletedAfterDiscovery(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "z.mp4": "z"})
	index := newFakeIndex()
	runScan(t, root, index)

	a := filepath.Join(root, "a.mp4")
	z := filepath.Join(root, "z.mp4")
	if err := os.WriteFile(z, []byte("changed z"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(z, later, later); err != nil {
		t.Fatal(err)
	}

	scanner := New(Options{Index: index})
	scanner.contentKey = func(path string) (string, error) {
		if path == z {
			if err := os.Remove(a); err != nil {
				return "", err
			}
		}
		return ContentKey(path)
	}
	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 {
		t.Fatalf("file deleted after discovery was not removed: %+v", result)
	}
	if _, ok := index.rows[a]; ok {
		t.Fatal("indexed location remained after its file disappeared")
	}
}

func TestScanPreservesIndexWhenRootDisappearsAfterDiscovery(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "z.mp4": "z"})
	index := newFakeIndex()
	runScan(t, root, index)

	z := filepath.Join(root, "z.mp4")
	if err := os.WriteFile(z, []byte("changed z"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(z, later, later); err != nil {
		t.Fatal(err)
	}

	scanner := New(Options{Index: index})
	scanner.contentKey = func(string) (string, error) {
		if err := os.RemoveAll(root); err != nil {
			return "", err
		}
		return "", fs.ErrNotExist
	}
	_, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err == nil {
		t.Fatal("root disappearance was reported as a completed scan")
	}
	assertScanFailure(t, err, domain.ScanErrorMediaFolderUnreadable, root)
	if len(index.deleted) != 0 {
		t.Fatalf("indexed locations were deleted after root disappearance: %v", index.deleted)
	}
	if len(index.rows) != 2 {
		t.Fatalf("indexed rows = %d, want 2", len(index.rows))
	}
}

func TestScanPreservesIndexWhenParentDisappearsAfterDiscovery(t *testing.T) {
	root := mediaTree(t, map[string]string{"gone/a.mp4": "a", "z.mp4": "z"})
	index := newFakeIndex()
	runScan(t, root, index)

	z := filepath.Join(root, "z.mp4")
	if err := os.WriteFile(z, []byte("changed z"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(z, later, later); err != nil {
		t.Fatal(err)
	}

	scanner := New(Options{Index: index})
	scanner.contentKey = func(path string) (string, error) {
		if path == z {
			if err := os.RemoveAll(filepath.Join(root, "gone")); err != nil {
				return "", err
			}
		}
		return ContentKey(path)
	}
	_, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err == nil {
		t.Fatal("parent disappearance was reported as a completed scan")
	}
	if len(index.deleted) != 0 {
		t.Fatalf("indexed locations were deleted after parent disappearance: %v", index.deleted)
	}
}

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
	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
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

func TestScanReportsUnreadableFileAsIssue(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "読めない.mp4": "b"})
	broken := filepath.Join(root, "読めない.mp4")
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.contentKey = func(path string) (string, error) {
		if path == broken {
			return "", &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
		}
		return ContentKey(path)
	}
	if _, err := scanner.Scan(context.Background(), domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	want := []domain.ScanFileIssue{{Path: broken, Kind: domain.IssueUnreadable}}
	if !slices.Equal(index.issues, want) {
		t.Fatalf("報告 = %+v, want %+v", index.issues, want)
	}
}

func TestScanReportsFileWithUnreadableInfoAsIssue(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a"})
	path := filepath.Join(root, "a.mp4")
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.walkDir = func(root string, fn fs.WalkDirFunc) error {
		return filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
			if p == path {
				entry = failingInfoEntry{entry}
			}
			return fn(p, entry, err)
		})
	}
	if _, err := scanner.Scan(context.Background(), domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	want := []domain.ScanFileIssue{{Path: path, Kind: domain.IssueUnreadable}}
	if !slices.Equal(index.issues, want) {
		t.Fatalf("報告 = %+v, want %+v", index.issues, want)
	}
}

// Unrecorded failures would be invisible to users.
func TestScanStopsWhenIssueCannotBeRecorded(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a"})
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	index.issueErr = errors.New("store unavailable")
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.contentKey = func(string) (string, error) { return "", errors.New("unreadable") }
	if _, err := scanner.Scan(context.Background(), domain.ActivityByOwner); !errors.Is(err, index.issueErr) {
		t.Fatalf("err = %v, want the recording failure", err)
	}
}

func TestScanContinuesAfterPermissionFailure(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "壊れた.mp4": "b", "c.mp4": "c"})

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

func TestScanReportsProgressForEveryFile(t *testing.T) {
	root := mediaTree(t, map[string]string{
		"a.mp4": "a", "b.mp4": "b", "c.mp4": "c", "d.mp4": "d", "e.mp4": "e",
	})

	index := newFakeIndex()
	runScan(t, root, index)

	want := []string{"progress 0/5"}
	for i, name := range []string{"a.mp4", "b.mp4", "c.mp4", "d.mp4", "e.mp4"} {
		want = append(want, "file "+name+" 0", fmt.Sprintf("progress %d/5", i+1))
	}
	want = append(want, "file done")
	if !slices.Equal(index.reports, want) {
		t.Errorf("報告 = %q\nwant %q", index.reports, want)
	}
}

func TestScanReportsKnownVideoOfCurrentFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a"})
	index := newFakeIndex()
	runScan(t, root, index)
	id := index.rows[filepath.Join(root, "a.mp4")].ID

	changed := filepath.Join(root, "a.mp4")
	if err := os.WriteFile(changed, []byte("changed length"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(changed, later, later); err != nil {
		t.Fatal(err)
	}
	index.reports = nil
	runScan(t, root, index)

	if want := fmt.Sprintf("file a.mp4 %d", id); !slices.Contains(index.reports, want) {
		t.Errorf("報告 = %q, want %q を含む", index.reports, want)
	}
}

func TestScanProgressCountsOnlyFilesThatNeedImport(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "b.mp4": "b"})
	index := newFakeIndex()
	runScan(t, root, index)
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.SeekThumbnailState = domain.SeekThumbnailDone
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

func TestScanRejectsFileChangedWhileContentKeyIsCalculated(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "old"})
	path := filepath.Join(root, "a.mp4")
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.contentKey = func(path string) (string, error) {
		if err := os.WriteFile(path, []byte("new and longer"), 0o600); err != nil {
			return "", err
		}
		return ContentKey(path)
	}

	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Completed() != 0 || result.Failed != 1 {
		t.Fatalf("changed file should be 0 / 1 with one failure, got %+v", result)
	}
	if len(index.upserts) != 0 {
		t.Fatalf("changed file was indexed with inconsistent metadata: %+v", index.upserts)
	}
	if want := []domain.ScanFileIssue{{Path: path, Kind: domain.IssueChangedDuringImport}}; !slices.Equal(index.issues, want) {
		t.Fatalf("報告 = %+v, want %+v", index.issues, want)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "new and longer" {
		t.Fatalf("fixture change failed: content=%q err=%v", got, err)
	}
}

// An unreadable stat after hashing does not prove replacement; only disappearance is
// changed_during_import.
func TestScanClassifiesMetadataFailureAfterContentKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want domain.ScanIssueKind
	}{
		{name: "読めない", err: errors.New("input/output error"), want: domain.IssueUnreadable},
		{name: "消えた", err: fs.ErrNotExist, want: domain.IssueChangedDuringImport},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mediaTree(t, map[string]string{"a.mp4": "old"})
			path := filepath.Join(root, "a.mp4")
			index := newFakeIndex()
			index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
			scanner := New(Options{Index: index, Queue: index, Reporter: index})
			hashed := false
			scanner.contentKey = func(path string) (string, error) {
				hashed = true
				return ContentKey(path)
			}
			scanner.lstat = func(name string) (fs.FileInfo, error) {
				if hashed && name == path {
					return nil, &fs.PathError{Op: "lstat", Path: name, Err: tc.err}
				}
				return os.Lstat(name)
			}

			result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
			if err != nil {
				t.Fatal(err)
			}
			if result.Failed != 1 || len(index.upserts) != 0 {
				t.Fatalf("result = %+v, upserts = %+v, want 1件の失敗", result, index.upserts)
			}
			if want := []domain.ScanFileIssue{{Path: path, Kind: tc.want}}; !slices.Equal(index.issues, want) {
				t.Fatalf("報告 = %+v, want %+v", index.issues, want)
			}
		})
	}
}

// An unreadable root prevents establishing the complete target set.
func TestScanFailsWhenMediaDirIsUnreadable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	index.rows[filepath.Join(root, "existing.mp4")] = domain.IndexedVideo{ID: 1, LocationID: 2}
	scanner := New(Options{Index: index})

	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err == nil {
		t.Fatal("root I/O failure was reported as a completed scan")
	}
	if !strings.Contains(err.Error(), root) {
		t.Fatalf("failure reason does not identify the root: %v", err)
	}
	assertScanFailure(t, err, domain.ScanErrorMediaFolderUnreadable, root)
	if result.Total != 0 || result.Failed != 0 {
		t.Fatalf("directory failure was mixed into file counts: %+v", result)
	}
	if len(index.deleted) != 0 {
		t.Fatalf("existing locations were deleted after root failure: %v", index.deleted)
	}
}

func TestScanFailsWhenSubdirectoryCannotBeRead(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "locked", "existing.mp4")
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	index.rows[protected] = domain.IndexedVideo{ID: 1, LocationID: 2}
	scanner := New(Options{Index: index})
	failedDir := filepath.Join(root, "locked")
	scanner.walkDir = func(_ string, walk fs.WalkDirFunc) error {
		return walk(failedDir, nil, errors.New("permission denied"))
	}

	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err == nil {
		t.Fatal("subdirectory I/O failure was reported as a completed scan")
	}
	if !strings.Contains(err.Error(), failedDir) {
		t.Fatalf("failure reason does not identify the unreadable directory: %v", err)
	}
	assertScanFailure(t, err, domain.ScanErrorLocationUnreadable, failedDir)
	if result.Total != 0 || result.Failed != 0 {
		t.Fatalf("directory failure was mixed into file counts: %+v", result)
	}
	if len(index.deleted) != 0 {
		t.Fatalf("existing locations were deleted after subtree failure: %v", index.deleted)
	}
}

func TestScanDiscardsPartialCountsWhenDiscoveryLaterFails(t *testing.T) {
	root := mediaTree(t, map[string]string{"unreadable.mp4": "content"})
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	failedDir := filepath.Join(root, "locked")
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index})
	scanner.walkDir = func(_ string, walk fs.WalkDirFunc) error {
		path := filepath.Join(root, entries[0].Name())
		if err := walk(path, failingInfoEntry{DirEntry: entries[0]}, nil); err != nil {
			return err
		}
		return walk(failedDir, nil, errors.New("permission denied"))
	}

	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err == nil {
		t.Fatal("later discovery failure was reported as a completed scan")
	}
	if result.Total != 0 || result.Failed != 0 {
		t.Fatalf("partial discovery counts escaped after failure: %+v", result)
	}
}

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
	if _, err := scanner.Scan(ctx, domain.ActivityByOwner); err == nil {
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

	_, err := New(Options{Index: index, Reporter: index}).Scan(context.Background(), domain.ActivityByOwner)
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
	if _, err := New(Options{Index: index}).Scan(context.Background(), domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	if len(index.deleted) != 0 {
		t.Fatalf("deleted location outside registered roots: %v", index.deleted)
	}
}

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

func assertScanFailure(t *testing.T, err error, code domain.ScanErrorCode, path string) {
	t.Helper()
	gotCode, gotPath := domain.ScanFailureOf(err)
	if gotCode != code || gotPath != path {
		t.Fatalf("scan failure = %q (%q), want %q (%q): %v", gotCode, gotPath, code, path, err)
	}
}

func TestScanFailureCodeWhenMediaFolderIsNotDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file.mp4")
	if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}

	_, err := New(Options{Index: index}).Scan(context.Background(), domain.ActivityByOwner)
	if err == nil {
		t.Fatal("a file as the media folder was reported as a completed scan")
	}
	assertScanFailure(t, err, domain.ScanErrorMediaFolderNotDirectory, root)
}

func TestScanFailureCodeWhenMediaFolderCannotBeListed(t *testing.T) {
	root := t.TempDir()
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index})
	scanner.walkDir = func(path string, walk fs.WalkDirFunc) error {
		return walk(path, nil, errors.New("permission denied"))
	}

	_, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err == nil {
		t.Fatal("an unreadable media folder was reported as a completed scan")
	}
	assertScanFailure(t, err, domain.ScanErrorMediaFolderUnreadable, root)
}

// Failures unrelated to a filesystem location remain unwrapped and become internal errors at the
// boundary.
func TestScanFailureCodeIsInternalForIndexFailure(t *testing.T) {
	index := newFakeIndex()
	index.folders = nil
	_, err := New(Options{Index: index}).Scan(context.Background(), domain.ActivityByOwner)
	if !errors.Is(err, ErrNoMediaFolders) {
		t.Fatalf("err = %v, want ErrNoMediaFolders", err)
	}
	assertScanFailure(t, err, domain.ScanErrorInternal, "")
}

func TestScanPassesTheDeciderToUpsertVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	index := newFakeIndex()
	runScan(t, root, index)

	if len(index.upserts) != 1 || index.upserts[0].Decider != domain.ActivityByOwner {
		t.Fatalf("UpsertVideo = %+v, want one file with the owner as decider", index.upserts)
	}
}
