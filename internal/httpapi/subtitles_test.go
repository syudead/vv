package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/media"
)

const subtitleSRT = "1\r\n00:00:01,000 --> 00:00:02,000\r\nこんにちは\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\nさようなら\r\n"

// subtitleFixture は一時ディレクトリに movie.mp4 を置き、本物の mediafs と字幕の変換で
// 経路を組み立てる。
func subtitleFixture(t *testing.T) (http.Handler, string) {
	t.Helper()
	mediaDir := t.TempDir()
	path := filepath.Join(mediaDir, "movie.mp4")
	writeSubtitleFile(t, mediaDir, "movie.mp4", "video")
	video := sampleVideo(1, "movie")
	video.Path = path
	handler := newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Subtitles: media.NewSubtitleConverter(),
	})
	return handler, mediaDir
}

func writeSubtitleFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func subtitleTarget(file, query string) string {
	target := "/api/videos/1/subtitles/" + url.PathEscape(file)
	if query != "" {
		target += "?" + query
	}
	return target
}

func listSubtitles(t *testing.T, handler http.Handler) []gen.SubtitleTrack {
	t.Helper()
	rec := do(t, handler, http.MethodGet, "/api/videos/1/subtitles")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q, want %q", got, cacheNoStore)
	}
	// 無いときは null ではなく空の配列であること。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["subtitles"]) == "null" {
		t.Fatalf("subtitles が null: %s", rec.Body.String())
	}
	return decode[gen.SubtitleTrackList](t, rec).Subtitles
}

func TestListVideoSubtitlesReadsTheFolderEveryTime(t *testing.T) {
	handler, mediaDir := subtitleFixture(t)

	if got := listSubtitles(t, handler); len(got) != 0 {
		t.Fatalf("字幕が無いのに %v", got)
	}

	writeSubtitleFile(t, mediaDir, "movie.srt", subtitleSRT)
	writeSubtitleFile(t, mediaDir, "movie.ja.srt", subtitleSRT)
	writeSubtitleFile(t, mediaDir, "other.srt", subtitleSRT)
	want := []gen.SubtitleTrack{
		{File: "movie.srt", Label: "", Format: gen.Srt},
		{File: "movie.ja.srt", Label: "ja", Format: gen.Srt},
	}
	assertTracks(t, "2 件", listSubtitles(t, handler), want)

	// 開いたあとに足したファイルも、呼び直せば載る（受け入れ条件 4）。
	writeSubtitleFile(t, mediaDir, "movie.en.vtt", "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nhello\n")
	want = []gen.SubtitleTrack{
		{File: "movie.srt", Label: "", Format: gen.Srt},
		{File: "movie.en.vtt", Label: "en", Format: gen.Vtt},
		{File: "movie.ja.srt", Label: "ja", Format: gen.Srt},
	}
	assertTracks(t, "3 件", listSubtitles(t, handler), want)
}

func assertTracks(t *testing.T, label string, got, want []gen.SubtitleTrack) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: [%d] = %+v, want %+v", label, i, got[i], want[i])
		}
	}
}

func TestListVideoSubtitlesWithoutTheVideoFile(t *testing.T) {
	handler, mediaDir := subtitleFixture(t)
	writeSubtitleFile(t, mediaDir, "movie.srt", subtitleSRT)
	if err := os.Remove(filepath.Join(mediaDir, "movie.mp4")); err != nil {
		t.Fatal(err)
	}
	rec := do(t, handler, http.MethodGet, "/api/videos/1/subtitles")
	assertErrorBody(t, "動画の実体が無い", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: codeNotFound, reason: reasonFileUnavailable})
	rec = do(t, handler, http.MethodGet, subtitleTarget("movie.srt", ""))
	assertErrorBody(t, "動画の実体が無いときの取得", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: codeNotFound, reason: reasonFileUnavailable})
}

func TestGetVideoSubtitleServesWebVTT(t *testing.T) {
	handler, mediaDir := subtitleFixture(t)
	writeSubtitleFile(t, mediaDir, "movie.srt", subtitleSRT)

	rec := do(t, handler, http.MethodGet, subtitleTarget("movie.srt", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/vtt; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheRevalidate {
		t.Errorf("Cache-Control = %q, want %q", got, cacheRevalidate)
	}
	body := rec.Body.String()
	for _, want := range []string{"WEBVTT", "00:00:01.000 --> 00:00:02.000\nこんにちは", "00:00:03.000 --> 00:00:04.000\nさようなら"} {
		if !strings.Contains(body, want) {
			t.Errorf("本文に %q が無い:\n%s", want, body)
		}
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag が無い")
	}

	// offsetMs だけ時刻がずれ、終わりが 0 以下の cue は落ちる。
	shifted := do(t, handler, http.MethodGet, subtitleTarget("movie.srt", "offsetMs=2500"))
	if shifted.Code != http.StatusOK {
		t.Fatalf("offsetMs: status = %d: %s", shifted.Code, shifted.Body.String())
	}
	shiftedBody := shifted.Body.String()
	if strings.Contains(shiftedBody, "こんにちは") || !strings.Contains(shiftedBody, "00:00:00.500 --> 00:00:01.500\nさようなら") {
		t.Errorf("offsetMs=2500 の本文:\n%s", shiftedBody)
	}
	if shifted.Header().Get("ETag") == etag {
		t.Error("ずらした本文の ETag が同じ")
	}

	// If-None-Match が一致すれば 304。
	req := httptest.NewRequest(http.MethodGet, subtitleTarget("movie.srt", ""), nil)
	req.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	handler.ServeHTTP(notModified, req)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Errorf("If-None-Match: status = %d, body = %q", notModified.Code, notModified.Body.String())
	}
}

func TestGetVideoSubtitleRefusesUnlistedAndBrokenFiles(t *testing.T) {
	handler, mediaDir := subtitleFixture(t)
	writeSubtitleFile(t, mediaDir, "movie.ja.srt", subtitleSRT)
	writeSubtitleFile(t, mediaDir, "movie.ja.vtt", "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nこんにちは\n")
	writeSubtitleFile(t, mediaDir, "movie.broken.srt", "\xF0\x40\xF0\x40")
	writeSubtitleFile(t, mediaDir, "other.srt", subtitleSRT)
	writeSubtitleFile(t, mediaDir, "movie.big.srt", strings.Repeat("a", int(domain.SubtitleFileLimit)+1))

	unavailable := wantError{status: http.StatusNotFound, code: codeNotFound, reason: reasonSubtitleUnavailable}
	for _, file := range []string{
		"movie.fr.srt",     // 無い
		"other.srt",        // 別の動画の字幕
		"movie.ja.srt",     // .vtt に隠れた .srt
		"movie.broken.srt", // 読めない
		"movie.big.srt",    // 上限を超える
		"../movie.ja.vtt",  // フォルダを含む名前
		"movie.mp4",        // 字幕でない
	} {
		rec := do(t, handler, http.MethodGet, subtitleTarget(file, ""))
		assertErrorBody(t, file, rec.Code, rec.Body.Bytes(), unavailable)
	}

	if rec := do(t, handler, http.MethodGet, subtitleTarget("movie.ja.vtt", "")); rec.Code != http.StatusOK {
		t.Errorf("movie.ja.vtt: status = %d: %s", rec.Code, rec.Body.String())
	}

	invalid := wantError{status: http.StatusBadRequest, code: codeInvalidRequest}
	for _, query := range []string{"offsetMs=-1", "offsetMs=1.5", "offsetMs=abc"} {
		rec := do(t, handler, http.MethodGet, subtitleTarget("movie.ja.vtt", query))
		assertErrorBody(t, query, rec.Code, rec.Body.Bytes(), invalid)
	}
}
