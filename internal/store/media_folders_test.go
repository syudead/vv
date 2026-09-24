package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

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
	claimed, err := db.ClaimJob(ctx, domain.JobProbe)
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
	retried, err := db.ClaimJob(ctx, domain.JobProbe)
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
	if _, err := db.GetVideo(ctx, video.ID); !errors.Is(err, domain.ErrNotFound) {
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
	if _, err := db.AddMediaFolder(ctx, child); !errors.Is(err, domain.ErrFolderConflict) {
		t.Fatalf("nested folder error = %v, want domain.ErrFolderConflict", err)
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
	if _, err := db.AddMediaFolder(ctx, candidate); !errors.Is(err, domain.ErrScanRunning) {
		t.Fatalf("error = %v, want domain.ErrScanRunning", err)
	}
}

func TestAddMediaFolderAllowsFilesystemRoot(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.SQL().Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	root := string(os.PathSeparator)
	if volume := filepath.VolumeName(t.TempDir()); volume != "" {
		root = volume + string(os.PathSeparator)
	}

	folder, err := db.AddMediaFolder(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Path != filepath.Clean(root) {
		t.Fatalf("path = %q, want %q", folder.Path, filepath.Clean(root))
	}

	file := sampleFile(filepath.Join(t.TempDir(), "root-visible.mp4"), "root visible", "root-content", 1, 0)
	video, err := db.UpsertVideo(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.ListVideos(ctx, domain.VideoQuery{Query: "root visible", Limit: domain.MaxLimit})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != video.ID {
		t.Fatalf("root video is not listed or searchable: %+v, %v", page, err)
	}
	if _, err := db.GetVideo(ctx, video.ID); err != nil {
		t.Fatalf("root video detail is unavailable: %v", err)
	}
	if err := db.EnqueueJob(ctx, domain.JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimJob(ctx, domain.JobProbe)
	if err != nil || job.VideoID != video.ID || job.LocationPath != file.Path {
		t.Fatalf("root video job is not claimable: %+v, %v", job, err)
	}
}

func TestAddMediaFolderRejectsRelativePath(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.AddMediaFolder(context.Background(), filepath.Join("relative", "media")); !errors.Is(err, domain.ErrInvalidMediaFolder) {
		t.Fatalf("error = %v, want domain.ErrInvalidMediaFolder", err)
	}
}

func TestAddMediaFolderRejectsWindowsCaseDuplicate(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path comparison")
	}
	db := migratedDB(t)
	root := t.TempDir()
	if _, err := db.AddMediaFolder(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddMediaFolder(context.Background(), strings.ToUpper(root)); !errors.Is(err, domain.ErrFolderConflict) {
		t.Fatalf("case-only duplicate error = %v, want domain.ErrFolderConflict", err)
	}
}

func TestAddMediaFolderPreservesFilesystemUnicodePath(t *testing.T) {
	db := migratedDB(t)
	decomposed := norm.NFD.String("Café")
	path := filepath.Join(t.TempDir(), decomposed)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	folder, err := db.AddMediaFolder(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Path != path {
		t.Fatalf("path = %q, want exact filesystem path %q", folder.Path, path)
	}
}

func TestDeletingRepresentativeLocationRecomputesContainerAndPlayability(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := t.TempDir()
	rootA := filepath.Join(root, "a")
	rootB := filepath.Join(root, "b")
	for _, path := range []string{rootA, rootB} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	folderA, err := db.AddMediaFolder(ctx, rootA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddMediaFolder(ctx, rootB); err != nil {
		t.Fatal(err)
	}
	video, err := db.UpsertVideo(ctx, sampleFile(filepath.Join(rootA, "movie.mkv"), "movie", "same", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile(filepath.Join(rootB, "movie.mp4"), "movie", "same", 1, 0)); err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mkv", probe)); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteMediaFolder(ctx, folderA.ID, folderA.Version); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != filepath.Join(rootB, "movie.mp4") || got.Container != "mp4" {
		t.Fatalf("representative = %q (%s)", got.Path, got.Container)
	}
	if !got.Playable || got.UnplayableReason != "" {
		t.Fatalf("playability was not recomputed: playable=%v reason=%q", got.Playable, got.UnplayableReason)
	}
}

func TestReplacingFolderSynchronizesLocationsEnabledByNewRoot(t *testing.T) {
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
	if _, err := db.AddMediaFolder(ctx, rootC); err != nil {
		t.Fatal(err)
	}
	video, err := db.UpsertVideo(ctx, sampleFile(filepath.Join(rootC, "z.mkv"), "z", "same", 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile(filepath.Join(rootB, "a.mp4"), "a", "same", 1, 0)); err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mkv", probe)); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ReplaceMediaFolder(ctx, folderA.ID, folderA.Version, rootB); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != filepath.Join(rootB, "a.mp4") || got.Container != "mp4" || !got.Playable {
		t.Fatalf("new-root representative was not synchronized: %+v", got)
	}
}
