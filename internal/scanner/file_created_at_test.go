package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func runScanWithCreatedAt(
	t *testing.T, root string, index *fakeIndex, createdAt func(string, fs.FileInfo) (time.Time, bool),
) domain.ScanResult {
	t.Helper()

	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.createdAt = createdAt
	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err != nil {
		t.Fatalf("走査に失敗した: %v", err)
	}
	return result
}

func fixedCreatedAt(at time.Time) func(string, fs.FileInfo) (time.Time, bool) {
	return func(string, fs.FileInfo) (time.Time, bool) { return at, true }
}

func unreadableCreatedAt(string, fs.FileInfo) (time.Time, bool) { return time.Time{}, false }

func TestScanPassesFileCreatedAtOfNewFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	path := filepath.Join(root, "a.mp4")

	index := newFakeIndex()
	runScan(t, root, index)

	if len(index.upserts) != 1 {
		t.Fatalf("UpsertVideo = %d 回, want 1", len(index.upserts))
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	want, ok := fileCreatedAt(path, info)
	got := index.upserts[0].FileCreatedAt
	switch runtime.GOOS {
	case "darwin", "windows":
		if !ok {
			t.Fatalf("%s で作成日時を読めなかった", runtime.GOOS)
		}
	}
	if !ok {
		// Some Linux filesystems, including tmpfs, lack creation time.
		t.Logf("このファイルシステムは作成日時を返さなかった: %s", root)
		if !got.IsZero() {
			t.Fatalf("FileCreatedAt = %v, want ゼロ値", got)
		}
		return
	}
	if got.IsZero() || !got.Equal(want) {
		t.Fatalf("FileCreatedAt = %v, want %v", got, want)
	}
	if d := time.Since(got); d < -time.Minute || d > time.Hour {
		t.Fatalf("FileCreatedAt = %v は今作ったファイルの作成日時に見えない", got)
	}
}

func TestScanPassesInjectedCreatedAtToUpsert(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	created := time.Date(2024, 5, 6, 7, 8, 9, 500, time.UTC)

	index := newFakeIndex()
	runScanWithCreatedAt(t, root, index, fixedCreatedAt(created))

	if len(index.upserts) != 1 || !index.upserts[0].FileCreatedAt.Equal(created) {
		t.Fatalf("UpsertVideo = %+v, want FileCreatedAt %v", index.upserts, created)
	}
	if len(index.createdAtCalls) != 0 {
		t.Fatalf("新しいファイルで UpdateLocationCreatedAt を呼んだ: %+v", index.createdAtCalls)
	}
}

func TestScanRegistersFileWhenCreatedAtIsUnreadable(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})

	index := newFakeIndex()
	result := runScanWithCreatedAt(t, root, index, unreadableCreatedAt)

	if result.Added != 1 || result.Failed != 0 || len(index.issues) != 0 {
		t.Fatalf("result = %+v, issues = %+v", result, index.issues)
	}
	if !index.upserts[0].FileCreatedAt.IsZero() {
		t.Fatalf("FileCreatedAt = %v, want ゼロ値", index.upserts[0].FileCreatedAt)
	}

	// Do not rewrite absent creation time repeatedly.
	runScanWithCreatedAt(t, root, index, unreadableCreatedAt)
	if len(index.createdAtCalls) != 0 {
		t.Fatalf("UpdateLocationCreatedAt = %+v, want 呼ばない", index.createdAtCalls)
	}
}

// Creation-time changes update only the location, without hashing or queuing jobs.
func TestScanUpdatesCreatedAtOfUnchangedFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	path := filepath.Join(root, "a.mp4")

	index := newFakeIndex()
	// Model a legacy location with null creation time.
	runScanWithCreatedAt(t, root, index, unreadableCreatedAt)
	markProcessed(index)
	upserts, jobs := len(index.upserts), len(index.jobs)

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	result := runScanWithCreatedAt(t, root, index, fixedCreatedAt(created))

	want := []createdAtCall{{locationID: index.rows[path].LocationID, createdAt: created}}
	if !slices.Equal(index.createdAtCalls, want) {
		t.Fatalf("UpdateLocationCreatedAt = %+v, want %+v", index.createdAtCalls, want)
	}
	if len(index.upserts) != upserts || len(index.jobs) != jobs {
		t.Fatalf("UpsertVideo %d -> %d, jobs %d -> %d: 取り込み直した",
			upserts, len(index.upserts), jobs, len(index.jobs))
	}
	if result.Total != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v, want 対象 0", result)
	}

	runScanWithCreatedAt(t, root, index, fixedCreatedAt(created))
	if len(index.createdAtCalls) != 1 {
		t.Fatalf("UpdateLocationCreatedAt = %d 回, want 1", len(index.createdAtCalls))
	}
}

// Ignore subsecond changes because storage keeps whole seconds.
func TestScanIgnoresSubsecondCreatedAtDifference(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})

	index := newFakeIndex()
	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	runScanWithCreatedAt(t, root, index, fixedCreatedAt(created))
	markProcessed(index)

	runScanWithCreatedAt(t, root, index, fixedCreatedAt(created.Add(999*time.Millisecond)))
	if len(index.createdAtCalls) != 0 {
		t.Fatalf("UpdateLocationCreatedAt = %+v, want 呼ばない", index.createdAtCalls)
	}
}

func TestScanClearsCreatedAtThatBecameUnreadable(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	path := filepath.Join(root, "a.mp4")

	index := newFakeIndex()
	runScanWithCreatedAt(t, root, index, fixedCreatedAt(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)))
	markProcessed(index)
	upserts, jobs := len(index.upserts), len(index.jobs)

	result := runScanWithCreatedAt(t, root, index, unreadableCreatedAt)

	want := []createdAtCall{{locationID: index.rows[path].LocationID}}
	if !slices.Equal(index.createdAtCalls, want) {
		t.Fatalf("UpdateLocationCreatedAt = %+v, want %+v", index.createdAtCalls, want)
	}
	if !index.rows[path].FileCreatedAt.IsZero() {
		t.Fatalf("索引の作成日時 = %v, want ゼロ値", index.rows[path].FileCreatedAt)
	}
	if len(index.upserts) != upserts || len(index.jobs) != jobs || result.Failed != 0 {
		t.Fatalf("UpsertVideo %d -> %d, jobs %d -> %d, result %+v",
			upserts, len(index.upserts), jobs, len(index.jobs), result)
	}
}

func TestScanReportsCreatedAtUpdateFailure(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	path := filepath.Join(root, "a.mp4")

	index := newFakeIndex()
	runScanWithCreatedAt(t, root, index, unreadableCreatedAt)
	markProcessed(index)
	index.createdAtErr = errors.New("database is locked")

	result := runScanWithCreatedAt(t, root, index, fixedCreatedAt(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)))

	if result.Total != 1 || result.Failed != 1 {
		t.Fatalf("result = %+v, want 1 件の失敗", result)
	}
	want := []domain.ScanFileIssue{{VideoID: index.rows[path].ID, Path: path, Kind: domain.IssueRegisterFailed}}
	if !slices.Equal(index.issues, want) {
		t.Fatalf("報告 = %+v, want %+v", index.issues, want)
	}
}

// Mark fixtures processed so unchanged files do not requeue jobs.
func markProcessed(index *fakeIndex) {
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.SeekThumbnailState = domain.SeekThumbnailDone
		row.PreviewState = domain.PreviewStateDone
		index.rows[path] = row
	}
}
