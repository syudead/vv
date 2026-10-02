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

// runScanWithCreatedAt は作成日時の読み取りを差し替えて走査を1回実行する。
func runScanWithCreatedAt(
	t *testing.T, root string, index *fakeIndex, createdAt func(string, fs.FileInfo) (time.Time, bool),
) domain.ScanResult {
	t.Helper()

	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.createdAt = createdAt
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("走査に失敗した: %v", err)
	}
	return result
}

func fixedCreatedAt(at time.Time) func(string, fs.FileInfo) (time.Time, bool) {
	return func(string, fs.FileInfo) (time.Time, bool) { return at, true }
}

func unreadableCreatedAt(string, fs.FileInfo) (time.Time, bool) { return time.Time{}, false }

// 新しいファイルは、OS が返す作成日時を UpsertVideo に渡す（受け入れ条件 4）。
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
		// Linux でも tmpfs など作成日時を持たないファイルシステムでは取れない。
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

// 差し替えた読み取りの値が、そのまま新しいファイルの登録に渡る。
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

// 作成日時を読めなくても走査は失敗せず、ゼロ値で登録する（受け入れ条件 5）。
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

	// 2 回目も、索引に値が無いので書き直さない。
	runScanWithCreatedAt(t, root, index, unreadableCreatedAt)
	if len(index.createdAtCalls) != 0 {
		t.Fatalf("UpdateLocationCreatedAt = %+v, want 呼ばない", index.createdAtCalls)
	}
}

// 変わっていないファイルの作成日時が索引と違えば、所在の列だけを書き直す
// （Edge Case「既に登録済みの動画」）。中身の識別子も job も触らない。
func TestScanUpdatesCreatedAtOfUnchangedFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "内容"})
	path := filepath.Join(root, "a.mp4")

	index := newFakeIndex()
	// この feature より前に登録した所在（作成日時が null）を再現する。
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

	// 索引が新しい値になったので、次の走査では書き直さない。
	runScanWithCreatedAt(t, root, index, fixedCreatedAt(created))
	if len(index.createdAtCalls) != 1 {
		t.Fatalf("UpdateLocationCreatedAt = %d 回, want 1", len(index.createdAtCalls))
	}
}

// 秒が同じなら、端数だけ違っても書き直さない（列は秒で持つ）。
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

// 索引に値があるのに読めなくなった所在は、ゼロ値で書き直して null に戻す。
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

// 作成日時を書き直せなかったファイルは、既存の動画の register_failed として報告する。
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

// markProcessed は索引の行を処理済みにし、変わっていないファイルで job を積み直さないようにする。
func markProcessed(index *fakeIndex) {
	for path, row := range index.rows {
		row.ProbeState = domain.ProbeStateDone
		row.ThumbnailState = domain.ThumbnailStateDone
		row.SeekThumbnailState = domain.SeekThumbnailDone
		row.PreviewState = domain.PreviewStateDone
		index.rows[path] = row
	}
}
