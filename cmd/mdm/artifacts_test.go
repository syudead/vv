package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/eventbus"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 生成物の置き場（internal/artifacts）を、本物の保存層・アプリケーション層・経路と
// つないで確かめる。置き場の単体テストは経路を通らず、httpapi と app の単体テストは
// 置き場を差し替えるので、既存の MDM_DATA_DIR がそのまま使えることはここで見る。

// existingKey は実際の形（"<16進>:<サイズ>"）の content key である。
const existingKey = "ab12cd34ef:5678"

type existingArtifacts struct {
	thumbnail, frame0, frame1, preview, manifest string
}

// placeExistingArtifacts は、この変更より前の版が作ったのと同じ場所に生成物を置く。
// パスは規則の実装を通さず文字列で書く。
func placeExistingArtifacts(t *testing.T, dataDir string) existingArtifacts {
	t.Helper()
	root := filepath.Join(dataDir, "thumbnails")
	files := existingArtifacts{
		thumbnail: filepath.Join(root, "ab", "ab12cd34ef_5678.jpg"),
		frame0:    filepath.Join(root, "seek", "ab", "ab12cd34ef_5678", "000000.jpg"),
		frame1:    filepath.Join(root, "seek", "ab", "ab12cd34ef_5678", "000001.jpg"),
		preview:   filepath.Join(root, "preview", "ab", "ab12cd34ef_5678.mp4"),
		manifest:  filepath.Join(root, "preview", "ab", "ab12cd34ef_5678.mp4.sha256"),
	}
	payload := []byte("preview-mp4")
	digest := sha256.Sum256(payload)
	for path, data := range map[string][]byte{
		files.thumbnail: []byte("thumbnail-jpeg"),
		files.frame0:    []byte("frame-0"),
		files.frame1:    []byte("frame-1"),
		files.preview:   payload,
		files.manifest:  fmt.Appendf(nil, `{"version":1,"size":%d,"sha256":%q}`+"\n", len(payload), hex.EncodeToString(digest[:])),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

type artifactsFixture struct {
	ctx     context.Context
	db      *store.DB
	videoID int64
	files   existingArtifacts
	handler http.Handler
	ingest  *app.Ingest
}

// newArtifactsFixture は、3種類の生成物を作り終えた動画が1本ある MDM_DATA_DIR を
// 用意し、main と同じ組み立てで経路と取り込みをつなぐ。
func newArtifactsFixture(t *testing.T) artifactsFixture {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mediaDir, "movie.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: "movie", ContentKey: existingKey, SizeBytes: 5,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 60_000, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, video.ID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().SetThumbnailState(ctx, video.ID, domain.ThumbnailStateDone); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().SetPreviewState(ctx, video.ID, domain.PreviewStateDone); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().ExecContext(ctx, `delete from jobs`); err != nil {
		t.Fatal(err)
	}
	files := placeExistingArtifacts(t, dataDir)

	logger := slog.New(slog.DiscardHandler)
	artifactStore := artifacts.New(Config{DataDir: dataDir}.ThumbnailsDir())
	catalog := app.NewCatalog(app.CatalogOptions{
		Index: db.Library(), Ingest: db.Ingest(), Files: artifactStore, Logger: logger,
	})
	handler := httpapi.NewRouter(httpapi.Options{
		Videos: db.Library(), Playback: db.Playback(), Catalog: catalog, Artifacts: artifactStore,
		Assets: fstest.MapFS{}, Logger: logger,
	})
	ingest := app.NewIngest(app.IngestOptions{Store: db.Ingest(), Artifacts: artifactStore, Logger: logger})
	return artifactsFixture{ctx: ctx, db: db, videoID: video.ID, files: files, handler: handler, ingest: ingest}
}

func (f artifactsFixture) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func (f artifactsFixture) video(t *testing.T) gen.Video {
	t.Helper()
	rec := f.get(t, "/api/videos/"+strconv.FormatInt(f.videoID, 10))
	if rec.Code != http.StatusOK {
		t.Fatalf("詳細: status = %d: %s", rec.Code, rec.Body)
	}
	var video gen.Video
	if err := json.Unmarshal(rec.Body.Bytes(), &video); err != nil {
		t.Fatal(err)
	}
	return video
}

func (f artifactsFixture) listed(t *testing.T) gen.Video {
	t.Helper()
	rec := f.get(t, "/api/videos")
	if rec.Code != http.StatusOK {
		t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	var page gen.VideoPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("一覧 = %d 件", len(page.Items))
	}
	return page.Items[0]
}

func (f artifactsFixture) previewJobs(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.db.SQL().QueryRowContext(f.ctx,
		`select count(*) from jobs where kind = 'preview' and video_id = ?`, f.videoID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// この変更より前に作った生成物がある MDM_DATA_DIR では、作り直しを積まずに
// 一覧のサムネイル・シークバーのプレビュー・ホバープレビューを出す。
func TestExistingArtifactsAreServedWithoutRegeneration(t *testing.T) {
	f := newArtifactsFixture(t)

	listed := f.listed(t)
	if listed.ThumbnailUrl == nil || listed.PreviewUrl == nil || listed.SeekThumbnailUrl == nil {
		t.Fatalf("一覧の URL が欠けている: %+v", listed)
	}
	video := f.video(t)
	if video.PreviewUrl == nil || video.SeekThumbnailState == nil || *video.SeekThumbnailState != gen.VideoSeekThumbnailStateDone {
		t.Fatalf("詳細 = previewUrl %v, seekThumbnailState %v", video.PreviewUrl, video.SeekThumbnailState)
	}

	id := strconv.FormatInt(f.videoID, 10)
	for target, want := range map[string]string{
		*listed.ThumbnailUrl: "thumbnail-jpeg",
		*listed.PreviewUrl:   "preview-mp4",
		"/api/videos/" + id + "/seek-thumbnail?positionMs=0":    "frame-0",
		"/api/videos/" + id + "/seek-thumbnail?positionMs=5000": "frame-1",
	} {
		rec := f.get(t, target)
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Errorf("%s = %d %q, want %q", target, rec.Code, rec.Body.String(), want)
		}
	}

	var jobs int
	if err := f.db.SQL().QueryRowContext(f.ctx, `select count(*) from jobs`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("ジョブが %d 件積まれた", jobs)
	}
}

// 完了の記録があるのに、プレビューが無い・manifest と一致しないときは、並行した
// 要求でも作り直しを1回だけ積み、previewUrl を出さない。
func TestIncompletePreviewIsRequeuedOnce(t *testing.T) {
	for name, damage := range map[string]func(files existingArtifacts) error{
		"MP4 が無い":      func(files existingArtifacts) error { return os.Remove(files.preview) },
		"manifest が無い": func(files existingArtifacts) error { return os.Remove(files.manifest) },
		"大きさが一致しない": func(files existingArtifacts) error {
			return os.WriteFile(files.preview, []byte("truncated"), 0o644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newArtifactsFixture(t)
			if err := damage(f.files); err != nil {
				t.Fatal(err)
			}

			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					rec := httptest.NewRecorder()
					f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos", nil))
				})
			}
			wg.Wait()

			if jobs := f.previewJobs(t); jobs != 1 {
				t.Fatalf("積んだ作り直し = %d, want 1", jobs)
			}
			listed := f.listed(t)
			if listed.PreviewUrl != nil || listed.PreviewState != gen.VideoPreviewStatePending {
				t.Fatalf("previewUrl = %v, previewState = %s", listed.PreviewUrl, listed.PreviewState)
			}
			rec := f.get(t, "/api/videos/"+strconv.FormatInt(f.videoID, 10)+"/preview")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("配信 = %d, want 404", rec.Code)
			}
		})
	}
}

// 動画の行が消えたとき、同じ content key を参照する動画が無ければ3種類の
// 生成物を消し、あれば残す。
func TestDeletedVideoReleasesOnlyUnreferencedArtifacts(t *testing.T) {
	t.Run("参照が無い", func(t *testing.T) {
		f := newArtifactsFixture(t)
		bus := eventbus.New(slog.New(slog.DiscardHandler))
		f.db.PublishTo(bus)
		subscribeEvents(bus, eventSubscribers{ReleaseArtifacts: f.ingest.ReleaseArtifacts})
		if err := f.db.ScanIndex().DeleteVideos(f.ctx, []int64{f.videoID}); err != nil {
			t.Fatal(err)
		}
		bus.Close()
		f.ingest.Wait()
		for _, path := range []string{f.files.thumbnail, filepath.Dir(f.files.frame0), f.files.preview, f.files.manifest} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("%s が残っている (err=%v)", path, err)
			}
		}
	})
	t.Run("参照が残っている", func(t *testing.T) {
		f := newArtifactsFixture(t)
		// 行が消えたという知らせが、同じ内容の動画が取り込み直された後に届いた場合。
		f.ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{existingKey}})
		f.ingest.Wait()
		for _, path := range []string{f.files.thumbnail, f.files.frame1, f.files.preview, f.files.manifest} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("参照のある %s が消えた: %v", path, err)
			}
		}
	})
}
