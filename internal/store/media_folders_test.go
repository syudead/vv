package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func TestMediaFolderOperationsAreAtomicAndScoped(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := t.TempDir()
	rootA := filepath.Join(root, "a")
	rootB := filepath.Join(root, "b")
	rootC := filepath.Join(root, "c")
	for _, path := range []string{rootA, rootB, rootC} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	folderA, err := db.AddMediaFolder(ctx, rootA)
	if err != nil {
		t.Fatal(err)
	}
	folderB, err := db.AddMediaFolder(ctx, rootB)
	if err != nil {
		t.Fatal(err)
	}

	fileA := sampleFile(filepath.Join(rootA, "movie.mp4"), "movie", "same-content", 10, 0)
	fileB := sampleFile(filepath.Join(rootB, "movie.mp4"), "movie", "same-content", 10, 0)
	video, err := db.UpsertVideo(ctx, fileA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, fileB); err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, domain.JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.LocationPath != fileA.Path {
		t.Fatalf("claimed path = %q, want %q", claimed.LocationPath, fileA.Path)
	}
	if _, err := db.SaveProgress(ctx, "same-content", domain.Progress{PositionMs: 1234}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := db.ReplaceMediaFolder(ctx, folderA.ID, folderA.Version, rootA)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Version != folderA.Version {
		t.Fatalf("same-path replacement changed version: %d", unchanged.Version)
	}
	if locations, err := db.VideoLocations(ctx, video.ID); err != nil || len(locations) != 2 {
		t.Fatalf("same-path replacement changed locations: %+v, %v", locations, err)
	}

	replaced, err := db.ReplaceMediaFolder(ctx, folderA.ID, folderA.Version, rootC)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Version != 2 {
		t.Fatalf("version = %d, want 2", replaced.Version)
	}
	locations, err := db.VideoLocations(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || locations[0].Path != fileB.Path {
		t.Fatalf("remaining locations = %+v", locations)
	}
	if _, err := db.GetVideo(ctx, video.ID); err != nil {
		t.Fatalf("video with another location was removed: %v", err)
	}
	current, err := db.JobIdentityCurrent(ctx, claimed)
	if err != nil || current {
		t.Fatalf("deleted location identity is current: %v, %v", current, err)
	}
	written, err := db.ApplyProbeForJob(ctx, claimed, domain.Probe{VideoCodec: "h264"}, domain.Playability{Playable: true})
	if err != nil || written {
		t.Fatalf("stale probe result was written: %v, %v", written, err)
	}
	if err := db.CompleteClaimedJob(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	retried, err := db.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if retried.LocationPath != fileB.Path {
		t.Fatalf("retry path = %q, want %q", retried.LocationPath, fileB.Path)
	}
	if err := db.CompleteClaimedJob(ctx, retried); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteMediaFolder(ctx, folderB.ID, folderB.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetVideo(ctx, video.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("orphan video still exists: %v", err)
	}
	progress, err := db.ProgressByContentKeys(ctx, []string{"same-content"})
	if err != nil || progress["same-content"].PositionMs != 1234 {
		t.Fatalf("playback progress was not preserved: %+v, %v", progress, err)
	}
}

func TestAddMediaFolderDoesNotTouchLibraryAndRejectsOverlap(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	file := sampleFile(filepath.Join(root, "existing.mp4"), "existing", "existing", 1, time.Second)
	video, err := db.UpsertVideo(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddMediaFolder(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetVideo(ctx, video.ID); err != nil {
		t.Fatalf("add changed the existing library: %v", err)
	}
	if _, err := db.AddMediaFolder(ctx, child); !errors.Is(err, ErrFolderConflict) {
		t.Fatalf("nested folder error = %v, want ErrFolderConflict", err)
	}
}

func TestMediaFolderMutationRejectsRunningScan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	candidate := filepath.Join(root, "candidate")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(candidate, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddMediaFolder(ctx, existing); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddMediaFolder(ctx, candidate); !errors.Is(err, ErrScanRunning) {
		t.Fatalf("error = %v, want ErrScanRunning", err)
	}
}
