package scanner

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const rescanID = 7

// seedIndex registers every file under the folders as an ordinary scan would and clears the call log.
func seedIndex(t *testing.T, index *fakeIndex, roots ...string) {
	t.Helper()
	index.folders = nil
	for i, root := range roots {
		index.folders = append(index.folders, domain.MediaFolder{ID: int64(i + 1), Path: root, Version: 1})
	}
	if _, err := New(Options{Index: index, Queue: index, Reporter: index}).Scan(
		context.Background(), domain.ActivityByOwner,
	); err != nil {
		t.Fatal(err)
	}
	index.calls = nil
}

func rescan(t *testing.T, index *fakeIndex, scope domain.RescanScope) domain.ScanResult {
	t.Helper()
	result, err := New(Options{Index: index, Queue: index, Reporter: index}).Rescan(
		context.Background(), rescanID, scope, domain.ActivityByOwner,
	)
	if err != nil {
		t.Fatalf("rescan failed: %v", err)
	}
	return result
}

func forcedKindsFor(videoID int64) []jobCall {
	return []jobCall{
		{kind: domain.JobProbe, videoID: videoID},
		{kind: domain.JobThumbnail, videoID: videoID},
		{kind: domain.JobSeekThumbnail, videoID: videoID},
	}
}

func TestRescanForcesAnUnchangedFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "content"})
	index := newFakeIndex()
	seedIndex(t, index, root)
	id := rowFor(t, index, filepath.Join(root, "a.mp4")).ID

	rescan(t, index, domain.RescanScope{Kind: domain.RescanLibrary})

	if len(index.calls) != 1 || index.calls[0] != "force 1" {
		t.Fatalf("calls = %v, want only the forced jobs without UpsertVideo", index.calls)
	}
	if !slices.Equal(index.forceJobs, forcedKindsFor(id)) {
		t.Errorf("forced jobs = %v, want %v", index.forceJobs, forcedKindsFor(id))
	}
	if want := []forcedMark{{scanID: rescanID, videoID: id}}; !slices.Equal(index.marked, want) {
		t.Errorf("marked = %v, want %v", index.marked, want)
	}
}

// A forced probe that fails leaves the preview pending, so the force pass still repairs the missing pending jobs.
func TestRescanRepairsTheMissingPendingJobsOfAnUnchangedFile(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "content"})
	path := filepath.Join(root, "a.mp4")
	index := newFakeIndex()
	seedIndex(t, index, root)
	row := index.rows[path]
	row.ProbeState = domain.ProbeStateDone
	row.ThumbnailState = domain.ThumbnailStateDone
	row.SeekThumbnailState = domain.SeekThumbnailDone
	row.PreviewState = domain.PreviewStatePending
	index.rows[path] = row
	index.jobs = nil

	rescan(t, index, domain.RescanScope{Kind: domain.RescanLibrary})

	if want := []jobCall{{kind: domain.JobPreview, videoID: row.ID}}; !slices.Equal(index.jobs, want) {
		t.Errorf("repaired jobs = %v, want %v", index.jobs, want)
	}
	if !slices.Equal(index.forceJobs, forcedKindsFor(row.ID)) {
		t.Errorf("forced jobs = %v, want %v", index.forceJobs, forcedKindsFor(row.ID))
	}
}

func TestRescanUpsertsAChangedFileBeforeForcingIt(t *testing.T) {
	stamp := time.Unix(1_757_000_000, 0)
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, path string)
	}{
		{"same size and mtime, new bytes", func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("after!!"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
		}},
		{"mtime only", func(t *testing.T, path string) {
			t.Helper()
			later := stamp.Add(time.Hour)
			if err := os.Chtimes(path, later, later); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mediaTree(t, map[string]string{"a.mp4": "before!"})
			path := filepath.Join(root, "a.mp4")
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			index := newFakeIndex()
			seedIndex(t, index, root)
			tc.change(t, path)

			rescan(t, index, domain.RescanScope{Kind: domain.RescanLibrary})

			if want := []string{"upsert a.mp4", "force 1"}; !slices.Equal(index.calls, want) {
				t.Fatalf("calls = %v, want %v", index.calls, want)
			}
			if !slices.Equal(index.forceJobs, forcedKindsFor(1)) {
				t.Errorf("forced jobs = %v, want %v", index.forceJobs, forcedKindsFor(1))
			}
		})
	}
}

func TestRescanSkipsAVideoTheScanAlreadyForced(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "content"})
	index := newFakeIndex()
	seedIndex(t, index, root)
	index.forcedBefore = []int64{rowFor(t, index, filepath.Join(root, "a.mp4")).ID}

	rescan(t, index, domain.RescanScope{Kind: domain.RescanLibrary})

	if len(index.forceJobs) != 0 || len(index.marked) != 0 {
		t.Fatalf("forced jobs = %v, marked = %v, want neither", index.forceJobs, index.marked)
	}
}

func TestRescanFolderRemovesMissingLocationsOnlyInsideTheFolder(t *testing.T) {
	inside := mediaTree(t, map[string]string{"a.mp4": "a", "gone.mp4": "gone"})
	outside := mediaTree(t, map[string]string{"b.mp4": "b", "gone.mp4": "gone elsewhere"})
	index := newFakeIndex()
	seedIndex(t, index, inside, outside)
	goneInside := rowFor(t, index, filepath.Join(inside, "gone.mp4")).LocationID
	keptInside := rowFor(t, index, filepath.Join(inside, "a.mp4")).ID
	for _, path := range []string{filepath.Join(inside, "gone.mp4"), filepath.Join(outside, "gone.mp4")} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}

	result := rescan(t, index, domain.RescanScope{Kind: domain.RescanFolder, FolderID: 1, Path: inside})

	if result.Removed != 1 || !slices.Equal(index.deleted, []int64{goneInside}) {
		t.Fatalf("removed = %d, deleted = %v, want only %d", result.Removed, index.deleted, goneInside)
	}
	rowFor(t, index, filepath.Join(outside, "gone.mp4"))
	if !slices.Equal(index.forceJobs, forcedKindsFor(keptInside)) {
		t.Errorf("forced jobs = %v, want only the folder's video %d", index.forceJobs, keptInside)
	}
}

func TestRescanVideoForcesOnlyThatVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a", "b.mp4": "b"})
	index := newFakeIndex()
	seedIndex(t, index, root)
	id := rowFor(t, index, filepath.Join(root, "b.mp4")).ID

	rescan(t, index, domain.RescanScope{Kind: domain.RescanVideo, VideoID: id})

	if !slices.Equal(index.forceJobs, forcedKindsFor(id)) {
		t.Errorf("forced jobs = %v, want only video %d", index.forceJobs, id)
	}
}

// A folder that points elsewhere than the recorded rescan_path stops the rescan instead of walking another tree.
func TestRescanFolderStopsWhenTheFolderMoved(t *testing.T) {
	recorded := mediaTree(t, map[string]string{"a.mp4": "a"})
	current := mediaTree(t, map[string]string{"b.mp4": "b"})
	index := newFakeIndex()
	seedIndex(t, index, current)

	_, err := New(Options{Index: index, Queue: index, Reporter: index}).Rescan(
		context.Background(), rescanID, domain.RescanScope{Kind: domain.RescanFolder, FolderID: 1, Path: recorded},
		domain.ActivityByOwner,
	)

	if err == nil {
		t.Fatal("rescan of a moved folder succeeded, want an error")
	}
	if len(index.calls) != 0 || len(index.deleted) != 0 {
		t.Errorf("calls = %v, deleted = %v, want nothing read, forced or removed", index.calls, index.deleted)
	}
}

func TestRescanVideoReportsAMissingLocationAndKeepsIt(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "content"})
	path := filepath.Join(root, "a.mp4")
	index := newFakeIndex()
	seedIndex(t, index, root)
	id := rowFor(t, index, path).ID
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	result := rescan(t, index, domain.RescanScope{Kind: domain.RescanVideo, VideoID: id})

	want := []domain.ScanFileIssue{{VideoID: id, Path: path, Kind: domain.IssueChangedDuringImport}}
	if !slices.Equal(index.issues, want) {
		t.Fatalf("issues = %+v, want %+v", index.issues, want)
	}
	if result.Failed != 1 || len(index.deleted) != 0 || len(index.forceJobs) != 0 {
		t.Errorf("result = %+v, deleted = %v, forced jobs = %v, want one failure and nothing removed or forced",
			result, index.deleted, index.forceJobs)
	}
}
