package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/scanner"
	"github.com/syudead/vv/internal/store"
)

// 解析と取り込みの失敗理由のコードが、失敗を作る adapter から保存層を通って API まで
// 届くことを、本物の保存層・解析・走査と経路をつないで確かめる
// （specs/023-english-i18n/contracts/error-api.md §2・§3）。

// japaneseText は日本語の固定文言が混ざっていないかを見る。
var japaneseText = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`)

func newFailureCodesDB(t *testing.T) (context.Context, string, *store.DB) {
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
	return ctx, dataDir, db
}

func failureCodesRouter(db *store.DB, scans httpapi.Scans) http.Handler {
	return httpapi.NewRouter(httpapi.Options{
		Videos: db.Library(), Playback: db.Playback(), MediaFolders: db.Settings(),
		Folders: db.Library(), Library: db.Library(), Tags: db.Tags(), Scans: scans,
		Assets: fstest.MapFS{}, Auth: ownerAuth{},
	})
}

func getJSON[T any](t *testing.T, handler http.Handler, target string) T {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", target, rec.Code, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s の JSON を読めない: %v: %s", target, err, rec.Body.String())
	}
	return out
}

// 動画でないファイルの解析が上限まで失敗すると、動画の応答が probe_failed のコードと
// 英語の理由を返す。
func TestProbeFailureOfNonVideoFileReturnsCode(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is unavailable")
	}
	ctx, dataDir, db := newFailureCodesDB(t)
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mediaDir, "not-a-video.mp4")
	content := []byte("this is plain text, not a video\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: "not-a-video", ContentKey: "not-a-video", SizeBytes: info.Size(),
		MTime: info.ModTime(), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	job := claimLastAttempt(t, ctx, db, domain.JobProbe, video.ID)
	handleErr := newTestIngest(db, dataDir).Handler(domain.JobProbe)(ctx, job)
	if handleErr == nil {
		t.Fatal("probing a non-video file unexpectedly succeeded")
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, handleErr); err != nil {
		t.Fatal(err)
	}

	got := getJSON[gen.Video](t, failureCodesRouter(db, nil), "/api/videos/"+strconv.FormatInt(video.ID, 10))
	if got.ProbeState != gen.VideoProbeStateFailed {
		t.Fatalf("probeState = %q, want failed", got.ProbeState)
	}
	if got.ProbeErrorCode == nil || *got.ProbeErrorCode != gen.ProbeErrorCodeProbeFailed {
		t.Fatalf("probeErrorCode = %v, want probe_failed", got.ProbeErrorCode)
	}
	if got.ProbeError == nil || !strings.HasPrefix(*got.ProbeError, "ffprobe failed (") ||
		japaneseText.MatchString(strings.ReplaceAll(*got.ProbeError, path, "")) {
		t.Fatalf("probeError = %v, want an English reason", got.ProbeError)
	}
}

// 読めないメディアフォルダの取り込みは、取り込みの応答が media_folder_unreadable の
// コードとそのフォルダのパスを返す。
func TestScanOfUnreadableMediaFolderReturnsCodeAndPath(t *testing.T) {
	ctx, _, db := newFailureCodesDB(t)
	mediaDir := filepath.Join(t.TempDir(), "media")
	if err := os.Mkdir(mediaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	folder, err := db.Settings().AddMediaFolder(ctx, mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	// 登録後にフォルダが消えた（外付けのディスクを外したなど）。
	if err := os.Remove(mediaDir); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.DiscardHandler)
	scans := app.NewScans(app.ScansOptions{
		Store: db.Scans(), Jobs: db.Ingest(), FolderIndex: db.ScanIndex(),
		NewScanner: func(reporter app.ScanReporter) app.Scanner {
			return scanner.New(scanner.Options{
				Index: db.ScanIndex(), Queue: db.Ingest(), Reporter: reporter, Logger: logger,
			})
		},
		Logger: logger,
	})
	if _, err := scans.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	scans.Wait()

	got := getJSON[gen.Scan](t, failureCodesRouter(db, scans), "/api/scans/current")
	if got.State != gen.ScanStateFailed {
		t.Fatalf("state = %q, want failed", got.State)
	}
	if got.ErrorCode == nil || *got.ErrorCode != gen.ScanErrorCodeMediaFolderUnreadable {
		t.Fatalf("errorCode = %v, want media_folder_unreadable", got.ErrorCode)
	}
	if got.ErrorPath == nil || *got.ErrorPath != folder.Path {
		t.Fatalf("errorPath = %v, want %q", got.ErrorPath, folder.Path)
	}
	if got.Error == nil || japaneseText.MatchString(*got.Error) {
		t.Fatalf("error = %v, want an English reason", got.Error)
	}
}

// 停止で打ち切った取り込みと、起動時に閉じた中断の取り込みは interrupted になる。
func TestInterruptedScanReturnsCode(t *testing.T) {
	ctx, _, db := newFailureCodesDB(t)
	if _, err := db.Settings().AddMediaFolder(ctx, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Scans().StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	scans := app.NewScans(app.ScansOptions{
		Store: db.Scans(), Jobs: db.Ingest(), Logger: slog.New(slog.DiscardHandler),
	})
	if err := scans.RecoverInterrupted(ctx); err != nil {
		t.Fatal(err)
	}

	got := getJSON[gen.Scan](t, failureCodesRouter(db, scans), "/api/scans/current")
	if got.State != gen.ScanStateFailed || got.ErrorCode == nil || *got.ErrorCode != gen.ScanErrorCodeInterrupted {
		t.Fatalf("scan = %+v, want failed with interrupted", got)
	}
	if got.ErrorPath != nil {
		t.Fatalf("errorPath = %q, want none", *got.ErrorPath)
	}
}
