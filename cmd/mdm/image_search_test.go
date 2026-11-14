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

// imageSearchFixture is an owner's library in a migrated store with one media folder, searched through the router
// with the real hashing, ffmpeg and upload area.
type imageSearchFixture struct {
	db         *store.DB
	mediaDir   string
	uploadsDir string
	handler    http.Handler
}

func newImageSearchFixture(t *testing.T) imageSearchFixture {
	t.Helper()
	dataDir := t.TempDir()
	db, err := store.OpenMigratedForTest(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(context.Background(), mediaDir); err != nil {
		t.Fatal(err)
	}
	uploadsDir := Config{DataDir: dataDir}.SearchUploadsDir()
	handler := httpapi.NewRouter(httpapi.Options{
		Videos: db.Library(), Assets: fstest.MapFS{}, Logger: slog.New(slog.DiscardHandler), Auth: ownerAuth{},
		ImageSearch: app.NewImageSearch(app.ImageSearchOptions{
			Index: db.Library(), Uploads: artifacts.NewSearchUploads(uploadsDir),
			Files:  app.MediaFiles{Folders: db.Settings(), Resolver: mediafs.New()},
			Logger: slog.New(slog.DiscardHandler),
			Media: app.ImageSearchMedia{
				HashImage: media.ImageFrameHash, Duration: media.QueryVideoDuration, Frames: media.QueryVideoFrames,
				Refine: media.RefineFrameHashes,
			},
		}),
	})
	return imageSearchFixture{db: db, mediaDir: mediaDir, uploadsDir: uploadsDir, handler: handler}
}

// indexFixtureVideo copies a fixture video under the media folder and indexes it.
func (f imageSearchFixture) indexFixtureVideo(t *testing.T, name string, durationMs int64) int64 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(mediaFixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.mediaDir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return f.indexVideo(t, path, durationMs)
}

// indexVideo registers a video already under the media folder and builds its search index with the real keyframe
// read.
func (f imageSearchFixture) indexVideo(t *testing.T, path string, durationMs int64) int64 {
	t.Helper()
	ctx := context.Background()
	db := f.db
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: name, ContentKey: "key-" + name, SizeBytes: info.Size(), MTime: time.Unix(1, 0),
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
	code, result, body := searchFile(t, handler, name, data)
	if code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", name, code, body)
	}
	return result
}

// searchFile posts one query file and returns the status, the decoded result of a 200 and the raw body.
func searchFile(t *testing.T, handler http.Handler, name string, data []byte) (int, gen.ImageSearchResult, string) {
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
	var result gen.ImageSearchResult
	if rec.Code != http.StatusOK {
		return rec.Code, result, rec.Body.String()
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return rec.Code, result, rec.Body.String()
}

// panFixture renders 30 s of a camera panning and zooming over a detailed still at 320x180, with a keyframe every
// 5 s, so frames between keyframes differ from both of them as moving footage does.
func panFixture(t *testing.T, dir string) string {
	t.Helper()
	still, video := filepath.Join(dir, "still.png"), filepath.Join(dir, "pan.mp4")
	ffmpegFixture(t, "-f", "lavfi", "-i", "mandelbrot=s=640x360:maxiter=300:start_x=-0.743643887037151:"+
		"start_y=0.131825904205330:start_scale=0.05", "-frames:v", "1", still)
	ffmpegFixture(t, "-loop", "1", "-framerate", "25", "-t", "30", "-i", still, "-vf",
		"zoompan=z='1.5+0.3*sin(on/90)':x='(iw-iw/zoom)*(0.5+0.45*sin(on/120))':y='(ih-ih/zoom)*(0.5+0.45*cos(on/150))'"+
			":d=1:s=320x180:fps=25", "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-g", "125",
		"-keyint_min", "125", "-sc_threshold", "0", "-pix_fmt", "yuv420p", video)
	return video
}

func ffmpegFixture(t *testing.T, args ...string) {
	t.Helper()
	full := append([]string{"-nostdin", "-hide_banner", "-v", "error", "-y"}, args...)
	if output, err := exec.Command("ffmpeg", full...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %v: %v: %s", args, err, output)
	}
}

// Through the real index, hashing, ffmpeg and upload area: a PNG cut at a keyframe and the video itself each find
// their video first, and the upload area is empty after each response.
func TestImageSearchFindsTheVideoAFrameOrCopyCameFrom(t *testing.T) {
	ctx := context.Background()
	fixture := newImageSearchFixture(t)
	target := fixture.indexFixtureVideo(t, "keyframes-2s.mp4", 20_000)
	fixture.indexFixtureVideo(t, "preview.mp4", 10_000)

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
		result := postSearchFile(t, fixture.handler, query.name, query.data)
		if len(result.Items) == 0 || result.Items[0].Video.Id != target || result.Items[0].Match.Kind != query.kind {
			t.Fatalf("%s: items = %+v, want video %d first as %s", query.name, result.Items, target, query.kind)
		}
		if entries, err := os.ReadDir(fixture.uploadsDir); err != nil && !os.IsNotExist(err) || len(entries) != 0 {
			t.Fatalf("%s: upload area after the response = %v, %v", query.name, entries, err)
		}
	}
}

// Footage the owner re-encoded or captured between keyframes still finds its video: a smaller, more compressed
// re-encode with other keyframes is a whole match, and a screenshot 1 s after a keyframe of a moving shot, too far
// from every keyframe to match one, is refined from the file to its own moment.
func TestImageSearchFindsMovingFootageThroughReencodesAndBetweenKeyframes(t *testing.T) {
	ctx := context.Background()
	fixture := newImageSearchFixture(t)
	video := panFixture(t, fixture.mediaDir)
	target := fixture.indexVideo(t, video, 30_000)
	fixture.indexFixtureVideo(t, "preview.mp4", 10_000)

	t.Run("downscaled re-encode", func(t *testing.T) {
		reencoded := filepath.Join(t.TempDir(), "reencoded.mp4")
		ffmpegFixture(t, "-i", video, "-vf", "scale=160:-2", "-c:v", "libx264", "-preset", "veryfast", "-crf", "32",
			"-g", "75", reencoded)
		data, err := os.ReadFile(reencoded)
		if err != nil {
			t.Fatal(err)
		}
		result := postSearchFile(t, fixture.handler, "reencoded.mp4", data)
		if len(result.Items) != 1 || result.Items[0].Video.Id != target ||
			result.Items[0].Match.Kind != gen.ImageSearchMatchKindWhole {
			t.Fatalf("items = %+v, want only video %d as a whole match", result.Items, target)
		}
	})

	t.Run("frame between keyframes", func(t *testing.T) {
		frame, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-ss", "6", "-i", video, "-frames:v", "1",
			"-f", "image2pipe", "-c:v", "png", "pipe:1").Output()
		if err != nil {
			t.Fatal(err)
		}
		result := postSearchFile(t, fixture.handler, "frame.png", frame)
		// A match at the keyframe would report 5000 ms; the refined frame is the one nearest 6 s at 4 frames a second.
		if len(result.Items) != 1 || result.Items[0].Video.Id != target ||
			result.Items[0].Match.PositionMs < 5750 || result.Items[0].Match.PositionMs > 6250 {
			t.Fatalf("items = %+v, want only video %d within 250 ms of 6000 ms", result.Items, target)
		}
	})
}
