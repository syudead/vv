package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakePlayback は再生位置の保存先を差し替える。
type fakePlayback struct {
	saved map[string]domain.Progress
	// lastKey は最後に書き込んだ鍵。content_key で記録していることの確認に使う。
	lastKey string
	// plays は保存のたびに渡された視聴の値（nil を含む）である。
	plays []*domain.Play
}

func newFakePlayback() *fakePlayback {
	return &fakePlayback{saved: map[string]domain.Progress{}}
}

func (f *fakePlayback) SaveProgress(
	_ context.Context, contentKey string, progress domain.Progress, play *domain.Play,
) (domain.Progress, error) {
	f.lastKey = contentKey
	f.plays = append(f.plays, play)
	// 実際の保存先と同じく、記録した時刻を入れて返す。
	progress.UpdatedAt = time.Now()
	f.saved[contentKey] = progress
	return progress, nil
}

func (f *fakePlayback) ProgressByContentKeys(
	_ context.Context, keys []string,
) (map[string]domain.Progress, error) {
	out := map[string]domain.Progress{}
	for _, key := range keys {
		if progress, ok := f.saved[key]; ok {
			out[key] = progress
		}
	}
	return out, nil
}

// putProgress は再生位置を送る。
func putProgress(
	t *testing.T, handler http.Handler, target, contentType, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// progressServer は再生位置の経路を組み立てる。動画の尺は 10 分にして、
// 「残り 15 秒」の規則が途中の位置で成立しないようにする。
func progressServer(t *testing.T, playback *fakePlayback) http.Handler {
	t.Helper()

	video := sampleVideo(1, "京都の街並み")
	duration := int64(600_000)
	video.DurationMs = &duration

	return newTestServer(t, Options{
		Videos:   &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Playback: playback,
	})
}

// 位置を受けて、記録後の状態を返す。
func TestPutProgress(t *testing.T) {
	playback := newFakePlayback()
	handler := progressServer(t, playback)

	rec := putProgress(t, handler, "/api/videos/1/progress", "application/json", `{"positionMs": 4000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	got := decode[gen.Progress](t, rec)
	if got.PositionMs != 4000 {
		t.Errorf("positionMs = %d, want 4000", got.PositionMs)
	}
	if got.Completed {
		t.Error("completed = true")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("updatedAt が入っていない")
	}

	// 鍵は content_key（videos.id ではない）。ファイルを移動・改名しても
	// 引き継がれることの前提である。
	if playback.lastKey != "abcdef0123456789abcdef:1024" {
		t.Errorf("記録の鍵 = %q, want content_key", playback.lastKey)
	}
}

// 視聴済みの判定はサーバー側で行い、クライアントの申告は採らない。
// 契約にも completed の入力は無い。
func TestPutProgressIgnoresClientCompletionClaim(t *testing.T) {
	playback := newFakePlayback()
	handler := progressServer(t, playback)

	// 途中の位置に completed: true を添えて送る。
	rec := putProgress(t, handler, "/api/videos/1/progress", "application/json",
		`{"positionMs": 1000, "completed": true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	if got := decode[gen.Progress](t, rec); got.Completed {
		t.Error("クライアントの申告で視聴済みになった")
	}
}

// 尺の 95% 以降を送るとサーバー側で視聴済みになる（S5）。
func TestPutProgressMarksCompletion(t *testing.T) {
	playback := newFakePlayback()
	handler := progressServer(t, playback)

	rec := putProgress(t, handler, "/api/videos/1/progress", "application/json", `{"positionMs": 580000}`)
	if got := decode[gen.Progress](t, rec); !got.Completed {
		t.Error("95% 以降なのに completed = false")
	}
}

// POST/PUTはJSONだけを受理する。離脱時のkeepalive PUTも同じ契約を使う。
func TestPutProgressAcceptsJSONContentType(t *testing.T) {
	for _, contentType := range []string{
		"application/json",
		"application/json; charset=utf-8",
	} {
		playback := newFakePlayback()
		handler := progressServer(t, playback)

		rec := putProgress(t, handler, "/api/videos/1/progress", contentType, `{"positionMs": 2000}`)
		if rec.Code != http.StatusOK {
			t.Errorf("Content-Type=%q: status = %d, want 200: %s", contentType, rec.Code, rec.Body)
		}
	}
}

// 受け付けない Content-Type は誤りにする。
func TestPutProgressRejectsUnsupportedContentType(t *testing.T) {
	for _, contentType := range []string{"application/x-www-form-urlencoded", "text/plain", ""} {
		handler := progressServer(t, newFakePlayback())
		rec := putProgress(t, handler, "/api/videos/1/progress", contentType, `{"positionMs":2000}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Content-Type=%q: status = %d, want 400", contentType, rec.Code)
		}
	}
}

// 壊れた本文と負の値は 400 + invalid_request。
func TestPutProgressRejectsInvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"JSON ではない", `positionMs=1`},
		{"途中で切れている", `{"positionMs":`},
		{"空", ``},
		{"負の値", `{"positionMs": -1}`},
		{"位置が無い", `{}`},
		{"型が違う", `{"positionMs": "4000"}`},
		{"識別子の型が違う", `{"positionMs": 1, "playbackId": 1}`},
		{"識別子が短い", `{"positionMs": 1, "playbackId": "0f8fad5b-d9cb-469f-a165-70867728950"}`},
		{"識別子の区切りが違う", `{"positionMs": 1, "playbackId": "0f8fad5bd9cb-469f-a165-70867728950e1"}`},
		{"識別子が 16 進でない", `{"positionMs": 1, "playbackId": "0f8fad5b-d9cb-469f-a165-70867728950g"}`},
		{"識別子が空", `{"positionMs": 1, "playbackId": ""}`},
		{"識別子が null", `{"positionMs": 1, "playbackId": null}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := progressServer(t, newFakePlayback())

			rec := putProgress(t, handler, "/api/videos/1/progress", "application/json", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if got := decode[gen.Error](t, rec); got.Code != codeInvalidRequest {
				t.Errorf("code = %q, want %s", got.Code, codeInvalidRequest)
			}
		})
	}
}

// 存在しない id は 404。
func TestPutProgressForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}, Playback: newFakePlayback()})

	rec := putProgress(t, handler, "/api/videos/999/progress", "application/json", `{"positionMs": 1}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeNotFound {
		t.Errorf("code = %q, want %s", got.Code, codeNotFound)
	}
}

// 一覧と詳細に再生位置を載せる。一覧で視聴済みと途中まで見た動画を
// 区別できるようにするため。
func TestVideosIncludeProgress(t *testing.T) {
	playback := newFakePlayback()
	playback.saved["abcdef0123456789abcdef:1024"] = domain.Progress{
		PositionMs: 4000, Completed: false,
	}

	video := sampleVideo(1, "海辺の散歩")
	handler := newTestServer(t, Options{
		Videos: &fakeLibrary{
			videos: map[int64]domain.Video{1: video},
			page:   domain.VideoPage{Items: []domain.Video{video}, Total: 1},
		},
		Playback: playback,
	})

	page := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos"))
	if page.Items[0].Progress == nil {
		t.Fatal("一覧に progress が載っていない")
	}
	if page.Items[0].Progress.PositionMs != 4000 {
		t.Errorf("一覧の positionMs = %d, want 4000", page.Items[0].Progress.PositionMs)
	}

	detail := decode[gen.Video](t, do(t, handler, http.MethodGet, "/api/videos/1"))
	if detail.Progress == nil || detail.Progress.PositionMs != 4000 {
		t.Errorf("詳細の progress = %+v", detail.Progress)
	}
}

// 再生位置を持たない動画では省略する。0 を入れると「先頭まで戻した」と
// 「一度も見ていない」が区別できなくなる。
func TestVideosOmitProgressWhenNeverPlayed(t *testing.T) {
	video := sampleVideo(1, "海辺の散歩")
	handler := newTestServer(t, Options{
		Videos: &fakeLibrary{
			videos: map[int64]domain.Video{1: video},
			page:   domain.VideoPage{Items: []domain.Video{video}, Total: 1},
		},
		Playback: newFakePlayback(),
	})

	page := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos"))
	if page.Items[0].Progress != nil {
		t.Errorf("progress = %+v, want 省略", page.Items[0].Progress)
	}
}

// 再生位置の保存先が無い構成でも、一覧は動く。progress が載らないだけである。
func TestVideosWithoutPlayback(t *testing.T) {
	video := sampleVideo(1, "海辺の散歩")
	handler := newTestServer(t, Options{
		Videos: &fakeLibrary{page: domain.VideoPage{Items: []domain.Video{video}, Total: 1}},
	})

	rec := do(t, handler, http.MethodGet, "/api/videos")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// 視聴の識別子があれば、再生したバージョンの content_key と有効な題名で視聴履歴を渡す。
// 応答は識別子の有無で変わらない（specs/043-watch-history/contracts/screen-api.md）。
func TestPutProgressPassesPlay(t *testing.T) {
	playback := newFakePlayback()
	video := sampleVideo(1, "京都の街並み")
	// 集まりのメンバー。履歴の鍵は集まりの鍵ではなく、再生したバージョンの content_key である。
	video.UserKey = "bundle:7"
	handler := newTestServer(t, Options{
		Videos:   &fakeLibrary{videos: map[int64]domain.Video{1: video}},
		Playback: playback,
	})

	rec := putProgress(t, handler, "/api/videos/1/progress", "application/json",
		`{"positionMs": 4000, "playbackId": "0F8FAD5B-D9CB-469F-A165-70867728950E"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Progress](t, rec); got.PositionMs != 4000 {
		t.Errorf("positionMs = %d, want 4000", got.PositionMs)
	}
	if playback.lastKey != "bundle:7" {
		t.Errorf("再生位置の鍵 = %q, want bundle:7", playback.lastKey)
	}
	// 大文字で送られた識別子は小文字にそろえて渡す。大小だけ違う同じ識別子で 2 件にしない。
	want := domain.Play{
		PlaybackID: "0f8fad5b-d9cb-469f-a165-70867728950e",
		ContentKey: "abcdef0123456789abcdef:1024",
		Title:      "京都の街並み",
	}
	if len(playback.plays) != 1 || playback.plays[0] == nil || *playback.plays[0] != want {
		t.Errorf("plays = %+v, want [%+v]", playback.plays, want)
	}
}

// 識別子が無い保存と、content_key が空の動画の保存は履歴を書かない。
func TestPutProgressWithoutPlay(t *testing.T) {
	t.Run("識別子が無い", func(t *testing.T) {
		playback := newFakePlayback()
		rec := putProgress(t, progressServer(t, playback), "/api/videos/1/progress", "application/json",
			`{"positionMs": 4000}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
		}
		if len(playback.plays) != 1 || playback.plays[0] != nil {
			t.Errorf("plays = %+v, want [nil]", playback.plays)
		}
	})
	t.Run("content_key が空", func(t *testing.T) {
		playback := newFakePlayback()
		video := sampleVideo(1, "京都の街並み")
		video.ContentKey = ""
		handler := newTestServer(t, Options{
			Videos:   &fakeLibrary{videos: map[int64]domain.Video{1: video}},
			Playback: playback,
		})
		rec := putProgress(t, handler, "/api/videos/1/progress", "application/json",
			`{"positionMs": 4000, "playbackId": "0f8fad5b-d9cb-469f-a165-70867728950e"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
		}
		if len(playback.plays) != 1 || playback.plays[0] != nil {
			t.Errorf("plays = %+v, want [nil]", playback.plays)
		}
	})
}
