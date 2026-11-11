package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/mediafs"
	"github.com/syudead/vv/internal/store"
)

const mediaFixtures = "../../internal/media/testdata/videos"

// indexFixtureVideo registers a fixture video under mediaDir and builds its search index with the real keyframe read.
func indexFixtureVideo(t *testing.T, db *store.DB, mediaDir, name string, durationMs int64) int64 {
	t.Helper()
	ctx := context.Background()
	data, err := os.ReadFile(filepath.Join(mediaFixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mediaDir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: name, ContentKey: "key-" + name, SizeBytes: int64(len(data)), MTime: time.Unix(1, 0),
		Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: durationMs, Width: 320, Height: 240, VideoCodec: "h264"}
	if err := db.Ingest().ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteJobsForTest(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().QueueFrameIndexJobs(ctx, domain.JobPriorityBackfill); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobFrameIndex)
	if err != nil {
		t.Fatal(err)
	}
	sources := app.MediaFiles{Folders: db.Settings(), Resolver: mediafs.New()}
	if err := app.NewSearchIndexBuilder(db.Ingest(), sources, media.KeyframeHashes).Build(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	return video.ID
}

func postSearchFile(t *testing.T, handler http.Handler, name string, data []byte) gen.ImageSearchResult {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/image-search", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", name, rec.Code, rec.Body)
	}
	var result gen.ImageSearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// Through the real index, hashing, ffmpeg and upload area: a PNG cut at a keyframe and the video itself each find
// their video first, and the upload area is empty after each response.
func TestImageSearchFindsTheVideoAFrameOrCopyCameFrom(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := store.OpenMigratedForTest(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	target := indexFixtureVideo(t, db, mediaDir, "keyframes-2s.mp4", 20_000)
	indexFixtureVideo(t, db, mediaDir, "preview.mp4", 10_000)

	uploadsDir := Config{DataDir: dataDir}.SearchUploadsDir()
	handler := httpapi.NewRouter(httpapi.Options{
		Videos: db.Library(), Assets: fstest.MapFS{}, Logger: slog.New(slog.DiscardHandler), Auth: ownerAuth{},
		ImageSearch: app.NewImageSearch(app.ImageSearchOptions{
			Index: db.Library(), Uploads: artifacts.NewSearchUploads(uploadsDir),
			Media: app.ImageSearchMedia{
				HashImage: media.ImageFrameHash, Duration: media.QueryVideoDuration, Frames: media.QueryVideoFrames,
			},
		}),
	})
	video := filepath.Join(mediaFixtures, "keyframes-2s.mp4")
	frame, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-ss", "4", "-i", video, "-frames:v", "1",
		"-f", "image2pipe", "-c:v", "png", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}

	for _, query := range []struct {
		name string
		data []byte
		kind gen.ImageSearchMatchKind
	}{{"frame.png", frame, gen.ImageSearchMatchKindFrame}, {"copy.mp4", copied, gen.ImageSearchMatchKindWhole}} {
		result := postSearchFile(t, handler, query.name, query.data)
		if len(result.Items) == 0 || result.Items[0].Video.Id != target || result.Items[0].Match.Kind != query.kind {
			t.Fatalf("%s: items = %+v, want video %d first as %s", query.name, result.Items, target, query.kind)
		}
		if entries, err := os.ReadDir(uploadsDir); err != nil && !os.IsNotExist(err) || len(entries) != 0 {
			t.Fatalf("%s: upload area after the response = %v, %v", query.name, entries, err)
		}
	}
}
