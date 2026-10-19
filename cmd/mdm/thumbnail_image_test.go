package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/mediafs"
	"github.com/syudead/vv/internal/store"
)

// Verify thumbnail image application through real stores, ingestion, and routes; frameGenerator
// replaces ffmpeg.

// Fake frames encode "frame:auto" or "frame:<ms>"; applied images receive a "fitted:" prefix.
type frameGenerator struct {
	probeOnlyGenerator
}

func (frameGenerator) Thumbnail(_ context.Context, _ string, _ int64, output, poster string) (bool, error) {
	return false, writeFrames(output, poster, "auto")
}

func (frameGenerator) ThumbnailAt(_ context.Context, _ string, positionMs int64, output, poster string) error {
	return writeFrames(output, poster, strconv.FormatInt(positionMs, 10))
}

func writeFrames(output, poster, at string) error {
	if err := os.WriteFile(poster, []byte("poster:"+at), 0o600); err != nil {
		return err
	}
	return os.WriteFile(output, []byte("frame:"+at), 0o600)
}

func (frameGenerator) FitThumbnailImage(_ context.Context, image []byte, _ float64) ([]byte, error) {
	return append([]byte("fitted:"), image...), nil
}

func (frameGenerator) FitThumbnailSource(_ context.Context, image []byte, _ float64) ([]byte, error) {
	return append([]byte("source:"), image...), nil
}

type thumbnailImageFixture struct {
	ctx       context.Context
	t         *testing.T
	db        *store.DB
	mediaDir  string
	thumbDir  string
	videoID   int64
	ingest    *app.Ingest
	handler   http.Handler
	artifacts *artifacts.Store
}

const thumbnailImageKey = "cd34ef56ab:5"

func newThumbnailImageFixture(t *testing.T) *thumbnailImageFixture {
	t.Helper()
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
	logger := slog.New(slog.DiscardHandler)
	thumbDir := Config{DataDir: dataDir}.ThumbnailsDir()
	artifactStore := artifacts.New(thumbDir)
	ingest := app.NewIngest(app.IngestOptions{
		Store: db.Ingest(), Generator: frameGenerator{}, Artifacts: artifactStore, Logger: logger,
	})
	catalog := app.NewCatalog(app.CatalogOptions{
		Index: db.Library(), Ingest: db.Ingest(), Files: artifactStore, Logger: logger,
	})
	handler := httpapi.NewRouter(httpapi.Options{
		Videos: db.Library(), Playback: db.Playback(), Catalog: catalog, Artifacts: artifactStore,
		MediaFolders: db.Settings(), Files: mediafs.New(), ThumbnailPicker: ingest,
		Assets: fstest.MapFS{}, Logger: logger, Auth: ownerAuth{},
	})
	f := &thumbnailImageFixture{
		ctx: ctx, t: t, db: db, mediaDir: mediaDir, thumbDir: thumbDir,
		ingest: ingest, handler: handler, artifacts: artifactStore,
	}
	f.videoID = f.scanAndGenerate()
	if got := f.thumbnailBytes(); got != "frame:auto" {
		t.Fatalf("前提: 代表サムネイル = %q", got)
	}
	return f
}

func (f *thumbnailImageFixture) scanAndGenerate() int64 {
	f.t.Helper()
	path := filepath.Join(f.mediaDir, "movie.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	video, err := f.db.ScanIndex().UpsertVideo(f.ctx, domain.VideoFile{
		Path: path, Title: "movie", ContentKey: thumbnailImageKey, SizeBytes: 5,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		f.t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 60_000, Width: 1920, Height: 1080, VideoCodec: "h264", AudioCodec: "aac"}
	if err := f.db.Ingest().ApplyProbe(f.ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		f.t.Fatal(err)
	}
	if err := store.DeleteJobsForTest(f.ctx, f.db); err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.Ingest().EnqueueJob(f.ctx, domain.JobThumbnail, video.ID, domain.ActivityByRule); err != nil {
		f.t.Fatal(err)
	}
	job, err := f.db.Ingest().ClaimJob(f.ctx, domain.JobThumbnail)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.ingest.Thumbnail(f.ctx, job); err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.Ingest().CompleteClaimedJob(f.ctx, job); err != nil {
		f.t.Fatal(err)
	}
	return video.ID
}

func (f *thumbnailImageFixture) serve(method, target, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f *thumbnailImageFixture) videoPath(suffix string) string {
	return "/api/videos/" + strconv.FormatInt(f.videoID, 10) + suffix
}

func (f *thumbnailImageFixture) video() gen.Video {
	f.t.Helper()
	return f.decodeVideo(f.serve(http.MethodGet, f.videoPath(""), ""))
}

func (f *thumbnailImageFixture) decodeVideo(rec *httptest.ResponseRecorder) gen.Video {
	f.t.Helper()
	if rec.Code != http.StatusOK {
		f.t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var video gen.Video
	if err := json.Unmarshal(rec.Body.Bytes(), &video); err != nil {
		f.t.Fatal(err)
	}
	return video
}

func (f *thumbnailImageFixture) thumbnailBytes() string {
	f.t.Helper()
	file, err := f.artifacts.ThumbnailFile(thumbnailImageKey)
	if err != nil {
		f.t.Fatalf("代表サムネイルが無い: %v", err)
	}
	defer func() { _ = file.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(file); err != nil {
		f.t.Fatal(err)
	}
	return buf.String()
}

func (f *thumbnailImageFixture) storedImage() []byte {
	f.t.Helper()
	image, err := f.db.Ingest().ThumbnailImage(f.ctx, thumbnailImageKey)
	if err != nil {
		f.t.Fatal(err)
	}
	return image
}

func (f *thumbnailImageFixture) storedSource() []byte {
	f.t.Helper()
	source, err := f.db.Ingest().ThumbnailImageSource(f.ctx, thumbnailImageKey)
	if err != nil {
		f.t.Fatal(err)
	}
	return source
}

func thumbnailURLString(video gen.Video) string {
	if video.ThumbnailUrl == nil {
		return ""
	}
	return *video.ThumbnailUrl
}

// Reimporting identical content must republish the applied bytes and retain the changed thumbnail
// URL.
func TestAppliedThumbnailImageSurvivesRescan(t *testing.T) {
	f := newThumbnailImageFixture(t)
	before := thumbnailURLString(f.video())

	saved, err := f.ingest.ApplyThumbnailImage(f.ctx, f.videoID, thumbnailImageKey, []byte("edited"), domain.ThumbnailImageGenerated)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ThumbnailImageRevision == 0 {
		t.Fatal("改版番号が付かない")
	}
	if got := f.thumbnailBytes(); got != "fitted:edited" || !bytes.Equal(f.storedImage(), []byte(got)) {
		t.Fatalf("ファイル = %q, 行 = %q", got, f.storedImage())
	}
	if got := f.storedSource(); string(got) != "source:edited" {
		t.Fatalf("元の画像 = %q", got)
	}
	applied := f.video()
	if after := thumbnailURLString(applied); after == before || after == "" {
		t.Fatalf("thumbnailUrl = %q, 前 = %q", after, before)
	}
	if applied.ThumbnailImage == nil || *applied.ThumbnailImage != gen.Generated {
		t.Fatalf("thumbnailImage = %v", applied.ThumbnailImage)
	}

	if err := f.db.ScanIndex().DeleteVideos(f.ctx, []int64{f.videoID}); err != nil {
		t.Fatal(err)
	}
	f.ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{thumbnailImageKey}})
	f.ingest.Wait()
	if _, err := f.artifacts.ThumbnailFile(thumbnailImageKey); err == nil {
		t.Fatal("前提: 動画を消しても代表サムネイルが残った")
	}

	f.videoID = f.scanAndGenerate()
	// Republish the stored image rather than regenerating the source frame.
	if got := f.thumbnailBytes(); got != "fitted:edited" {
		t.Fatalf("取り込み直したあとの代表サムネイル = %q, want 適用した画像", got)
	}
	if again := f.video(); again.ThumbnailImage == nil || thumbnailURLString(again) != thumbnailURLString(applied) {
		t.Fatalf("取り込み直したあとの thumbnailUrl = %q, want %q", thumbnailURLString(again), thumbnailURLString(applied))
	}
}

// Position changes clear the applied image; reset restores the selected frame, or the automatic
// frame when no position is set.
func TestThumbnailPositionAndRevertReplaceAppliedImage(t *testing.T) {
	f := newThumbnailImageFixture(t)
	if _, err := f.ingest.ApplyThumbnailImage(f.ctx, f.videoID, thumbnailImageKey, []byte("edited"), domain.ThumbnailImageGenerated); err != nil {
		t.Fatal(err)
	}

	moved := f.decodeVideo(f.serve(http.MethodPut, f.videoPath("/thumbnail-position"), `{"positionMs":12000}`))
	if moved.ThumbnailImage != nil || f.storedImage() != nil || f.storedSource() != nil {
		t.Fatalf("位置の設定で画像が外れない: thumbnailImage = %v", moved.ThumbnailImage)
	}
	if got := f.thumbnailBytes(); got != "frame:12000" {
		t.Fatalf("位置の設定のあとの代表サムネイル = %q", got)
	}

	if _, err := f.ingest.ApplyThumbnailImage(f.ctx, f.videoID, thumbnailImageKey, []byte("again"), domain.ThumbnailImageGenerated); err != nil {
		t.Fatal(err)
	}
	reverted := f.decodeVideo(f.serve(http.MethodDelete, f.videoPath("/thumbnail-image"), ""))
	if reverted.ThumbnailImage != nil || f.storedImage() != nil || f.storedSource() != nil || f.thumbnailBytes() != "frame:12000" {
		t.Fatalf("取り消し: thumbnailImage = %v, 代表サムネイル = %q", reverted.ThumbnailImage, f.thumbnailBytes())
	}
	if reverted.ThumbnailPositionMs == nil || *reverted.ThumbnailPositionMs != 12_000 {
		t.Fatalf("取り消しで位置が消えた: %v", reverted.ThumbnailPositionMs)
	}

	f.decodeVideo(f.serve(http.MethodPut, f.videoPath("/thumbnail-position"), `{"positionMs":null}`))
	if _, err := f.ingest.ApplyThumbnailImage(f.ctx, f.videoID, thumbnailImageKey, []byte("third"), domain.ThumbnailImageGenerated); err != nil {
		t.Fatal(err)
	}
	f.decodeVideo(f.serve(http.MethodDelete, f.videoPath("/thumbnail-image"), ""))
	if got := f.thumbnailBytes(); got != "frame:auto" || f.storedImage() != nil {
		t.Fatalf("位置の無い取り消しの代表サムネイル = %q", got)
	}
}

// The player's large thumbnail follows every kind of change to the thumbnail.
func TestLargeThumbnailFollowsTheThumbnail(t *testing.T) {
	f := newThumbnailImageFixture(t)
	large := func() string {
		t.Helper()
		rec := f.serve(http.MethodGet, f.videoPath("/thumbnail?size=large"), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	if got := large(); got != "poster:auto" {
		t.Fatalf("取り込み直後 = %q", got)
	}
	if _, err := f.ingest.ApplyThumbnailImage(f.ctx, f.videoID, thumbnailImageKey, []byte("edited"), domain.ThumbnailImageUploaded); err != nil {
		t.Fatal(err)
	}
	if got := large(); got != "source:edited" {
		t.Fatalf("画像の適用のあと = %q", got)
	}
	f.decodeVideo(f.serve(http.MethodPut, f.videoPath("/thumbnail-position"), `{"positionMs":12000}`))
	if got := large(); got != "poster:12000" {
		t.Fatalf("位置の設定のあと = %q", got)
	}
}

func TestApplyThumbnailImageToGoneContentWritesNothing(t *testing.T) {
	f := newThumbnailImageFixture(t)
	before := thumbnailURLString(f.video())

	_, err := f.ingest.ApplyThumbnailImage(f.ctx, f.videoID, "ffffffffff:9", []byte("edited"), domain.ThumbnailImageGenerated)
	if !errors.Is(err, domain.ErrContentChanged) {
		t.Fatalf("err = %v, want ErrContentChanged", err)
	}
	if f.storedImage() != nil || f.thumbnailBytes() != "frame:auto" || thumbnailURLString(f.video()) != before {
		t.Fatalf("書いた: 行 %q, 代表サムネイル %q", f.storedImage(), f.thumbnailBytes())
	}
}
