package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// 適用したサムネイルの画像（specs/045-comfyui-thumbnails/data-model.md、research.md R-6・R-10）を、
// 本物の保存層・生成物の置き場・取り込み・経路をつないで確かめる。ffmpeg の代わりに
// frameGenerator が、どのコマから作ったかを中身に書く。

// frameGenerator は代表サムネイルの中身を "frame:auto"（自動の位置）か "frame:<ms>"（指定の
// 位置）にし、適用する画像には "fitted:" を前に付ける。
type frameGenerator struct {
	probeOnlyGenerator
}

func (frameGenerator) Thumbnail(_ context.Context, _ string, _ int64, output string) (bool, error) {
	return false, os.WriteFile(output, []byte("frame:auto"), 0o600)
}

func (frameGenerator) ThumbnailAt(_ context.Context, _ string, positionMs int64, output string) error {
	return os.WriteFile(output, fmt.Appendf(nil, "frame:%d", positionMs), 0o600)
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

// newThumbnailImageFixture は、自動の位置の代表サムネイルを作り終えた動画が1本ある
// MDM_DATA_DIR を用意し、main と同じ組み立てで取り込みと経路をつなぐ。
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

// scanAndGenerate は動画を取り込み、解析を記録して代表サムネイルの仕事を1件処理する。
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
	if err := f.db.Ingest().EnqueueJob(f.ctx, domain.JobThumbnail, video.ID); err != nil {
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

// thumbnailBytes は配信される代表サムネイルの中身を返す。
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

// 適用のあと、代表サムネイルのファイルは行と同じバイト列になり、thumbnailUrl が変わる。動画の行を
// 消して同じ内容を取り込み直すと、代表サムネイルの仕事は同じバイト列を公開する。
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

	// 動画の行を消すと、参照の無くなった生成物も消える。
	if err := f.db.ScanIndex().DeleteVideos(f.ctx, []int64{f.videoID}); err != nil {
		t.Fatal(err)
	}
	f.ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{thumbnailImageKey}})
	f.ingest.Wait()
	if _, err := f.artifacts.ThumbnailFile(thumbnailImageKey); err == nil {
		t.Fatal("前提: 動画を消しても代表サムネイルが残った")
	}

	f.videoID = f.scanAndGenerate()
	// 代表サムネイルの仕事は行の image を公開し直し、元の画像は使わない。
	if got := f.thumbnailBytes(); got != "fitted:edited" {
		t.Fatalf("取り込み直したあとの代表サムネイル = %q, want 適用した画像", got)
	}
	if again := f.video(); again.ThumbnailImage == nil || thumbnailURLString(again) != thumbnailURLString(applied) {
		t.Fatalf("取り込み直したあとの thumbnailUrl = %q, want %q", thumbnailURLString(again), thumbnailURLString(applied))
	}
}

// 適用のあとに位置を設定すると画像の行が消え、ファイルはその場面のコマになる。取り消しは位置が
// あればその場面のコマへ、無ければ自動の位置のコマへ戻す。
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

	// 位置があるときの取り消しは、その場面のコマへ戻す。
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

	// 位置が無いときの取り消しは、自動の位置のコマへ戻す。
	f.decodeVideo(f.serve(http.MethodPut, f.videoPath("/thumbnail-position"), `{"positionMs":null}`))
	if _, err := f.ingest.ApplyThumbnailImage(f.ctx, f.videoID, thumbnailImageKey, []byte("third"), domain.ThumbnailImageGenerated); err != nil {
		t.Fatal(err)
	}
	f.decodeVideo(f.serve(http.MethodDelete, f.videoPath("/thumbnail-image"), ""))
	if got := f.thumbnailBytes(); got != "frame:auto" || f.storedImage() != nil {
		t.Fatalf("位置の無い取り消しの代表サムネイル = %q", got)
	}
}

// どの動画も持たない内容への適用は content_changed で、ファイルも行も変えない。
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
