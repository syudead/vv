package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// shortTranscodeStartTimeout は報告の経路の待ちの上限をテストの間だけ縮める。
func shortTranscodeStartTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()
	saved := transcodeStartupTimeout
	transcodeStartupTimeout = timeout
	t.Cleanup(func() { transcodeStartupTimeout = saved })
}

func transcodeStartOf(t *testing.T, rec *httptest.ResponseRecorder) int64 {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("transcode-start status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	var body struct {
		StartMs int64 `json:"startMs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("本文を読めない: %v: %s", err, rec.Body)
	}
	return body.StartMs
}

func TestTranscodeStartReportsActualStartAfterTranscode(t *testing.T) {
	fake := &fakeTranscoder{body: "fragmented-mp4", actualStartMs: 5_000}
	handler := transcodeServer(t, false, fake)

	rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?startMs=7000&attempt=a-1_B")
	if rec.Code != http.StatusOK || fake.startMs != 7_000 {
		t.Fatalf("transcode = %d startMs=%d", rec.Code, fake.startMs)
	}
	// 変換の要求が終わってもしばらく引ける（契約 §2）。
	if got := transcodeStartOf(t, do(t, handler, http.MethodGet, "/api/videos/1/transcode-start?attempt=a-1_B")); got != 5_000 {
		t.Errorf("startMs = %d, want 5000", got)
	}
}

// 報告の要求が変換の要求より先に届いても、載るまで待って値を返す。
func TestTranscodeStartWaitsForAttemptToAppear(t *testing.T) {
	fake := &fakeTranscoder{body: "fragmented-mp4", actualStartMs: 4_000}
	handler := transcodeServer(t, false, fake)

	report := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/1/transcode-start?attempt=early", nil))
		report <- rec
	}()
	time.Sleep(20 * time.Millisecond)
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?startMs=6000&attempt=early"); rec.Code != http.StatusOK {
		t.Fatalf("transcode = %d", rec.Code)
	}
	select {
	case rec := <-report:
		if got := transcodeStartOf(t, rec); got != 4_000 {
			t.Errorf("startMs = %d, want 4000", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("報告の要求が戻らない")
	}
}

// 載っていて未決の間は待ち、最初のデータとともに決まった値を返す。
func TestTranscodeStartWaitsUntilStartIsResolved(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	fake := &fakeTranscoder{body: "fragmented-mp4", actualStartMs: 3_000}
	fake.beforeReturn = func(domain.LiveTranscodeRequest) {
		close(entered)
		<-release
	}
	handler := transcodeServer(t, false, fake)

	transcoded := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/1/transcode.mp4?startMs=5000&attempt=pending", nil))
		transcoded <- rec.Code
	}()
	<-entered

	report := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/1/transcode-start?attempt=pending", nil))
		report <- rec
	}()
	select {
	case rec := <-report:
		t.Fatalf("開始位置が決まる前に戻った: %d %s", rec.Code, rec.Body)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if got := transcodeStartOf(t, <-report); got != 3_000 {
		t.Errorf("startMs = %d, want 3000", got)
	}
	if code := <-transcoded; code != http.StatusOK {
		t.Errorf("transcode = %d", code)
	}
}

// 変換が最初のデータを出せずに終わったら、上限を待たずに 404 を返す。
func TestTranscodeStartNotFoundWhenTranscodeFails(t *testing.T) {
	for _, tc := range []struct {
		name string
		fake *fakeTranscoder
	}{
		{"start failure", &fakeTranscoder{err: errors.New("start failed")}},
		{"no initial data", &fakeTranscoder{waitErr: errors.New("exit 1")}},
		{"unprocessable", &fakeTranscoder{err: domain.ErrUnprocessableMedia}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := transcodeServer(t, false, tc.fake)
			if rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?startMs=1000&attempt=failed"); rec.Code < 400 {
				t.Fatalf("transcode = %d", rec.Code)
			}
			started := time.Now()
			rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode-start?attempt=failed")
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
				t.Fatalf("transcode-start = %d: %s", rec.Code, rec.Body)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Errorf("失敗した変換の報告に %s かかった", elapsed)
			}
		})
	}
}

// 上限までに attempt が現れなければ 404。別の動画の経路からは同じ attempt を引けない。
func TestTranscodeStartNotFoundForUnknownAttempt(t *testing.T) {
	shortTranscodeStartTimeout(t, 30*time.Millisecond)
	mediaDir, first, _ := streamFixture(t, "a.mkv", 128)
	second := first
	second.ID = 2
	handler := newTestServer(t, Options{
		Videos:     &fakeLibrary{videos: map[int64]domain.Video{1: first, 2: second}, roots: []string{mediaDir}},
		Transcoder: &fakeTranscoder{body: "fragmented-mp4", actualStartMs: 1_000},
	})

	if rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode-start?attempt=never"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown attempt = %d", rec.Code)
	}
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?startMs=2000&attempt=shared"); rec.Code != http.StatusOK {
		t.Fatalf("transcode = %d", rec.Code)
	}
	if rec := do(t, handler, http.MethodGet, "/api/videos/2/transcode-start?attempt=shared"); rec.Code != http.StatusNotFound {
		t.Errorf("other video = %d", rec.Code)
	}
	if rec := do(t, handler, http.MethodGet, "/api/videos/999/transcode-start?attempt=shared"); rec.Code != http.StatusNotFound {
		t.Errorf("missing video = %d", rec.Code)
	}
}

func TestTranscodeStartRejectsMalformedAttempt(t *testing.T) {
	fake := &fakeTranscoder{body: "fragmented-mp4"}
	handler := transcodeServer(t, false, fake)
	for _, attempt := range []string{"", "has%20space", "a.b", strings.Repeat("a", 65)} {
		for _, target := range []string{
			"/api/videos/1/transcode.mp4?startMs=1000&attempt=" + attempt,
			"/api/videos/1/transcode-start?attempt=" + attempt,
		} {
			rec := do(t, handler, http.MethodGet, target)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_request"`) {
				t.Errorf("%s = %d: %s", target, rec.Code, rec.Body)
			}
		}
	}
	if fake.starts != 0 {
		t.Errorf("形式の違う attempt で変換を %d 回始めた", fake.starts)
	}
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode-start"); rec.Code != http.StatusBadRequest {
		t.Errorf("attempt なし = %d", rec.Code)
	}
}

// 同じ attempt で 2 度来たら後の値で上書きする。
func TestTranscodeStartKeepsLatestValue(t *testing.T) {
	fake := &fakeTranscoder{body: "fragmented-mp4", actualStartMs: 4_000}
	handler := transcodeServer(t, false, fake)
	do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?startMs=5000&attempt=twice")
	fake.actualStartMs = 4_500
	do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?startMs=5000&attempt=twice")
	if got := transcodeStartOf(t, do(t, handler, http.MethodGet, "/api/videos/1/transcode-start?attempt=twice")); got != 4_500 {
		t.Errorf("startMs = %d, want 4500", got)
	}
}

// 変換の要求が終わってから保持時間が過ぎた行は消える。attempt の無い要求は載らない。
func TestTranscodeStartForgetsAfterRetention(t *testing.T) {
	shortTranscodeStartTimeout(t, 30*time.Millisecond)
	saved := transcodeStartRetention
	transcodeStartRetention = 20 * time.Millisecond
	t.Cleanup(func() { transcodeStartRetention = saved })

	fake := &fakeTranscoder{body: "fragmented-mp4", actualStartMs: 1_000}
	handler := transcodeServer(t, false, fake)
	do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?startMs=2000&attempt=old")
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode-start?attempt=old")
		if rec.Code == http.StatusNotFound {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("保持時間を過ぎても残っている: %d", rec.Code)
		}
		time.Sleep(10 * time.Millisecond)
	}

}

// 決まらずに終わった要求は失敗として返り、報告の要求だけが作った行は待ち手が居なく
// なれば消える。
func TestTranscodeStartLedgerFailureAndPlaceholders(t *testing.T) {
	ledger := newTranscodeStarts()
	failed := transcodeStartKey{videoID: 1, attempt: "x"}
	_, end := ledger.begin(failed)
	end()
	if _, ok := ledger.wait(t.Context(), failed, time.Second); ok {
		t.Error("決まらずに終わった要求の値が返った")
	}

	absent := transcodeStartKey{videoID: 1, attempt: "absent"}
	if _, ok := ledger.wait(t.Context(), absent, 10*time.Millisecond); ok {
		t.Error("載っていない attempt の値が返った")
	}
	ledger.mu.Lock()
	_, leaked := ledger.slots[absent]
	_, kept := ledger.slots[failed]
	ledger.mu.Unlock()
	if leaked {
		t.Error("報告の要求だけが作った行が残っている")
	}
	if !kept {
		t.Error("終わった変換の行が保持時間の前に消えた")
	}
}
