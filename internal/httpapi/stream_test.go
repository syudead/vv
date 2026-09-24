package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// streamFixture は MediaDir の内側に実体を持つ動画を1件用意する。
func streamFixture(t *testing.T, name string, size int) (string, domain.Video, []byte) {
	t.Helper()

	mediaDir := t.TempDir()
	path := filepath.Join(mediaDir, name)

	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 251)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	video := sampleVideo(1, "海辺の散歩")
	video.Path = path
	video.SizeBytes = int64(size)
	video.Container = domain.ContainerFromPath(path)

	return mediaDir, video, content
}

// streamServer は配信の経路だけを組み立てる。
func streamServer(t *testing.T, mediaDir string, video domain.Video) http.Handler {
	t.Helper()

	return newTestServer(t, Options{
		Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
	})
}

// rangeRequest は Range 付きの要求を投げる。
func rangeRequest(t *testing.T, handler http.Handler, target, rangeHeader string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// Range の解釈は http.ServeContent に任せる。ここで確かめるのは標準実装の
// 再テストではなく、ハンドラが正しい io.ReadSeeker と ModTime を渡している
// ことである（技術選定文書 3.3）。
func TestStreamServesRanges(t *testing.T) {
	const size = 4096
	mediaDir, video, content := streamFixture(t, "a.mp4", size)
	handler := streamServer(t, mediaDir, video)

	tests := []struct {
		name      string
		header    string
		wantStart int
		wantEnd   int
	}{
		{"先頭", "bytes=0-1023", 0, 1023},
		{"途中", "bytes=1024-2047", 1024, 2047},
		{"末尾", fmt.Sprintf("bytes=%d-", size-512), size - 512, size - 1},
		{"末尾からの相対指定", "bytes=-512", size - 512, size - 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := rangeRequest(t, handler, "/api/videos/1/stream", tc.header)

			if rec.Code != http.StatusPartialContent {
				t.Fatalf("status = %d, want 206: %s", rec.Code, rec.Body)
			}
			wantRange := fmt.Sprintf("bytes %d-%d/%d", tc.wantStart, tc.wantEnd, size)
			if got := rec.Header().Get("Content-Range"); got != wantRange {
				t.Errorf("Content-Range = %q, want %q", got, wantRange)
			}
			if got := rec.Body.Bytes(); string(got) != string(content[tc.wantStart:tc.wantEnd+1]) {
				t.Errorf("本文が要求した範囲と違う（%d バイト）", len(got))
			}
		})
	}
}

// Range 無しなら全体を返し、Range に対応していることを示す。
func TestStreamServesWholeFile(t *testing.T) {
	const size = 2048
	mediaDir, video, content := streamFixture(t, "a.mp4", size)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes", got)
	}
	if rec.Body.Len() != len(content) {
		t.Errorf("本文 = %d バイト, want %d", rec.Body.Len(), len(content))
	}
}

// 範囲外の要求は 416。シーク先が尺を越えたときにブラウザがこれを見る。
func TestStreamRejectsUnsatisfiableRange(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 1024)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "bytes=99999999999-")
	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("status = %d, want 416", rec.Code)
	}
	// ServeContent は成功用のヘッダを設定したあとで 416 を返すので、
	// 失敗応答に配信用の Cache-Control が残らないことを固定する。
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("416 の Cache-Control = %q, want %q", got, cacheNoStore)
	}
}

// If-Range が一致すれば部分応答、一致しなければ全体を返す。シーク中に
// ファイルが差し替わった場合に、混ざった内容を返さないための仕組みである。
func TestStreamHonorsIfRange(t *testing.T) {
	const size = 2048
	mediaDir, video, _ := streamFixture(t, "a.mp4", size)
	handler := streamServer(t, mediaDir, video)

	// まず Last-Modified を得る。
	first := rangeRequest(t, handler, "/api/videos/1/stream", "")
	lastModified := first.Header().Get("Last-Modified")
	if lastModified == "" {
		t.Fatal("Last-Modified が付いていない（ModTime を渡していない）")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/videos/1/stream", nil)
	req.Header.Set("Range", "bytes=0-99")
	req.Header.Set("If-Range", lastModified)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Errorf("一致する If-Range で status = %d, want 206", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/videos/1/stream", nil)
	req.Header.Set("Range", "bytes=0-99")
	req.Header.Set("If-Range", "Sun, 01 Jan 2006 00:00:00 GMT")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("一致しない If-Range で status = %d, want 200（全体）", rec.Code)
	}
}

// Content-Type は拡張子から決める。ffprobe の format_name は mp4 と mov を
// 区別できないので、判定と配信で基準を揃える。
func TestStreamContentType(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"a.mp4", "video/mp4"},
		{"a.m4v", "video/mp4"},
		{"a.webm", "video/webm"},
		{"a.mkv", "application/octet-stream"},
		{"a.avi", "application/octet-stream"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mediaDir, video, _ := streamFixture(t, tc.name, 512)
			handler := streamServer(t, mediaDir, video)

			rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
			if got := rec.Header().Get("Content-Type"); got != tc.want {
				t.Errorf("Content-Type = %q, want %q", got, tc.want)
			}
		})
	}
}

// 内容が同じでも、別の利用者に共有キャッシュさせない。
func TestStreamCacheControl(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if got := rec.Header().Get("Cache-Control"); got != cacheStream {
		t.Errorf("Cache-Control = %q, want %q", got, cacheStream)
	}
}

// 再生できない形式でも配信自体は行う。ブラウザが再生できるかどうかと、
// ファイルを取得できるかは別の話である。
func TestStreamServesUnplayableFormats(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mkv", 512)
	video.Playable = false
	video.UnplayableReason = domain.ReasonContainer
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200（再生可否と配信は別）", rec.Code)
	}
}

// DB に入っているパスをそのまま開かない。登録rootの外を指す行は 404
// にする。403 にしないのは、存在そのものを漏らさないためである。
func TestStreamRefusesPathsOutsideMediaDir(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	outside := filepath.Join(t.TempDir(), "秘密.mp4")
	if err := os.WriteFile(outside, []byte("外のファイル"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{"絶対パスで外を指す", outside},
		{"相対で外へ出る", filepath.Join(mediaDir, "..", filepath.Base(outside))},
		{"接頭辞が似ているだけの別ディレクトリ", mediaDir + "-other/a.mp4"},
		{"MediaDir そのもの", mediaDir},
		{"空", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outsideVideo := video
			outsideVideo.Path = tc.path
			handler := streamServer(t, mediaDir, outsideVideo)

			rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404（存在を漏らさない）", rec.Code)
			}
			if rec.Code == http.StatusForbidden {
				t.Error("403 は存在を漏らす")
			}
		})
	}
}

// シンボリックリンクで外へ出る行も 404。Clean だけでは辿り着けるため、
// EvalSymlinks のあとにも同じ検証を行う。
func TestStreamRefusesSymlinkEscape(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "秘密.mp4")
	if err := os.WriteFile(outside, []byte("外のファイル"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(mediaDir, "link.mp4")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("シンボリックリンクを作れない: %v", err)
	}

	video.Path = link
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// MediaDir の内側を指すシンボリックリンクは配信してよい。
func TestStreamAllowsSymlinkInsideMediaDir(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	link := filepath.Join(mediaDir, "別名.mp4")
	if err := os.Symlink(filepath.Join(mediaDir, "a.mp4"), link); err != nil {
		t.Skipf("シンボリックリンクを作れない: %v", err)
	}

	video.Path = link
	handler := streamServer(t, mediaDir, video)

	if rec := rangeRequest(t, handler, "/api/videos/1/stream", ""); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// 通常ファイルでないものは開かない（ディレクトリ・デバイスファイル）。
func TestStreamRefusesNonRegularFiles(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)

	dir := filepath.Join(mediaDir, "ディレクトリ.mp4")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	video.Path = dir
	handler := streamServer(t, mediaDir, video)

	if rec := rangeRequest(t, handler, "/api/videos/1/stream", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// 実体が無くなっていても 404 にする。再生中にファイルが失われた場合、
// アプリケーション全体は使い続けられなければならない。
func TestStreamHandlesMissingFile(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)
	if err := os.Remove(video.Path); err != nil {
		t.Fatal(err)
	}
	handler := streamServer(t, mediaDir, video)

	if rec := rangeRequest(t, handler, "/api/videos/1/stream", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// 存在しない id は 404。
func TestStreamForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{roots: []string{t.TempDir()}}})

	if rec := rangeRequest(t, handler, "/api/videos/999/stream", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// MediaDir が設定されていない構成では、何も開かない。
func TestStreamWithoutMediaDir(t *testing.T) {
	_, video, _ := streamFixture(t, "a.mp4", 512)
	handler := newTestServer(t, Options{
		Videos: &fakeLibrary{videos: map[int64]domain.Video{1: video}},
	})

	rec := rangeRequest(t, handler, "/api/videos/1/stream", "")
	if rec.Code == http.StatusOK {
		t.Error("MediaDir が無いのに配信した")
	}
}

// 経路が /api/ の外へ漏れていないこと。SPA のフォールバックが動画本体を
// index.html で返してしまうと、再生の失敗が「JSON でない」形で現れる。
func TestStreamPathStaysUnderAPI(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mp4", 512)
	handler := streamServer(t, mediaDir, video)

	rec := rangeRequest(t, handler, "/api/videos/1/stream/extra", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<html") {
		t.Error("/api/ 配下で HTML が返った")
	}
}

// MediaFiles をつなぎ忘れたら、実体が無いこと（404）と見分けられるよう 500 を返す。
func TestStreamWithoutMediaFilesIsInternalError(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "movie.mp4", 16)
	handler := NewRouter(Options{
		Videos: &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Assets: emptyAssets{},
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/stream"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body)
	}
}
