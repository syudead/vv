package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 公開フラグの切り替え（specs/016-single-account-auth/contracts/guest-api.md §4）と、
// 非公開にしたときの配信の打ち切り（§6）は、本物の認証・保存層で確かめる（#301）。

func visibilityBody(public bool, ids ...int64) string {
	body := `{"videoIds":[`
	for index, id := range ids {
		if index > 0 {
			body += ","
		}
		body += strconv.FormatInt(id, 10)
	}
	return body + `],"public":` + strconv.FormatBool(public) + `}`
}

func (f *guestFixture) setVisibility(public bool, ids ...int64) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{
		method: http.MethodPut, target: "/api/video-visibility",
		body: visibilityBody(public, ids...), cookies: []*http.Cookie{f.owner},
	})
}

// 公開にするとゲストの一覧と詳細に現れ、Video.public が true になる。非公開に戻すと
// 次の取得から現れない。
func TestVideoVisibilityPublishesToGuests(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	if video := decode[gen.Video](t, env.get(f.videoPath("b", ""), f.owner)); video.Public {
		t.Fatalf("切り替え前の b が公開: %+v", video)
	}
	if video := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)); !video.Public {
		t.Fatalf("公開の a の public が false: %+v", video)
	}

	// 重複は1つとして、ライブラリに無い id は数えず、既に公開の a も誤りにしない。
	rec := f.setVisibility(true, f.ids["b"], f.ids["b"], f.ids["a"], 9999)
	if rec.Code != http.StatusOK {
		t.Fatalf("公開: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.VideoVisibilityResponse](t, rec); got.Applied != 2 {
		t.Errorf("公開: applied = %d, want 2", got.Applied)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}

	list := env.get("/api/videos")
	if got := titlesOfItems(t, rawItems(t, list.Body.Bytes(), "items")); !slices.Equal(got, []string{"a", "b", "d"}) {
		t.Errorf("公開の後のゲストの一覧 = %v, want [a b d]", got)
	}
	for _, item := range decode[gen.VideoPage](t, list).Items {
		if !item.Public {
			t.Errorf("ゲストの一覧の %s の public が false", item.Title)
		}
	}
	detail := env.get(f.videoPath("b", ""))
	if detail.Code != http.StatusOK {
		t.Fatalf("公開の後のゲストの詳細: status = %d: %s", detail.Code, detail.Body)
	}
	if video := decode[gen.Video](t, detail); !video.Public {
		t.Errorf("ゲストの詳細の public が false: %+v", video)
	}

	rec = f.setVisibility(false, f.ids["b"])
	if got := decode[gen.VideoVisibilityResponse](t, rec); rec.Code != http.StatusOK || got.Applied != 1 {
		t.Fatalf("非公開: status = %d, applied = %d", rec.Code, got.Applied)
	}
	if got := titlesOfItems(t, rawItems(t, env.get("/api/videos").Body.Bytes(), "items")); !slices.Equal(got, []string{"a", "d"}) {
		t.Errorf("非公開の後のゲストの一覧 = %v, want [a d]", got)
	}
	assertStatus(t, "非公開の後のゲストの詳細", env.get(f.videoPath("b", "")), http.StatusNotFound)
	if video := decode[gen.Video](t, env.get(f.videoPath("b", ""), f.owner)); video.Public {
		t.Errorf("非公開の後の所有者の詳細の public が true")
	}
}

// 切り替えは所有者だけで、本文の誤りは 400 になる。
func TestVideoVisibilityRejectsGuestsAndInvalidBodies(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	guest := env.serve(authRequest{method: http.MethodPut, target: "/api/video-visibility", body: visibilityBody(true, f.ids["b"])})
	assertUnauthenticated(t, "Cookie なしの切り替え", guest)
	assertStatus(t, "切り替えられていない", env.get(f.videoPath("b", "")), http.StatusNotFound)

	tooMany := make([]int64, maxVideoTagsIDs+1)
	for index := range tooMany {
		tooMany[index] = int64(index + 1)
	}
	for label, body := range map[string]string{
		"空の videoIds":   visibilityBody(true),
		"多すぎる videoIds": visibilityBody(true, tooMany...),
		"public が無い":    `{"videoIds":[1]}`,
		"知らない欄":         `{"videoIds":[1],"public":true,"extra":1}`,
	} {
		rec := env.serve(authRequest{method: http.MethodPut, target: "/api/video-visibility", body: body, cookies: []*http.Cookie{f.owner}})
		assertStatus(t, label, rec, http.StatusBadRequest)
	}
}

// endlessTranscoder は止められるまで変換の出力を返し続ける。
type endlessTranscoder struct{}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func (endlessTranscoder) Start(context.Context, string, int64, bool, time.Time) (io.ReadCloser, func() error, func(), error) {
	return io.NopCloser(zeroReader{}), func() error { return nil }, func() {}, nil
}

// 非公開に戻すと、ゲストとして処理中のその動画の Range 応答とライブ変換が終わり、
// 次の要求は 404 になる。所有者として処理中の応答は終わらない。
func TestVideoVisibilityEndsGuestResponsesInFlight(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()
	const size = 512 << 20
	moviePath := filepath.Join(mediaDir, "long.mp4")
	file, err := os.Create(moviePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		library := db.Library()
		return Options{
			Videos:     library,
			Folders:    library,
			Playback:   db.Playback(),
			Tags:       db.Tags(),
			Visibility: db.Visibility(),
			Catalog:    app.NewCatalog(app.CatalogOptions{Index: library, Ingest: db.Ingest(), Files: guestArtifactFiles{}}),
			Transcoder: endlessTranscoder{},
		}
	})
	owner := env.setup()
	db := env.db
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	result, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: moviePath, Title: "long", ContentKey: "content-key-long", SizeBytes: size,
		MTime: time.Now(), AddedAt: time.Now(), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 60_000, Width: 640, Height: 360, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(ctx, result.ID, probe, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{result.ID}, true); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(env.handler)
	t.Cleanup(server.Close)
	base := "/api/videos/" + strconv.FormatInt(result.ID, 10)

	open := func(target string, cookie *http.Cookie, header map[string]string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, server.URL+target, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		for name, value := range header {
			req.Header.Set(name, value)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		if _, err := io.ReadFull(resp.Body, make([]byte, 1024)); err != nil {
			t.Fatalf("%s: 始めを読めない (status = %d): %v", target, resp.StatusCode, err)
		}
		return resp
	}
	rangeHeader := map[string]string{"Range": "bytes=0-"}
	guestStream := open(base+"/stream", nil, rangeHeader)
	guestTranscode := open(base+"/transcode.mp4", nil, nil)
	ownerStream := open(base+"/stream", owner, rangeHeader)
	if guestStream.StatusCode != http.StatusPartialContent || ownerStream.StatusCode != http.StatusPartialContent {
		t.Fatalf("Range: status = %d, %d", guestStream.StatusCode, ownerStream.StatusCode)
	}
	if guestStream.Header.Get(audienceHeader) != "guest" || ownerStream.Header.Get(audienceHeader) != "owner" {
		t.Fatalf("見る人 = %q, %q", guestStream.Header.Get(audienceHeader), ownerStream.Header.Get(audienceHeader))
	}

	rec := env.serve(authRequest{
		method: http.MethodPut, target: "/api/video-visibility",
		body: visibilityBody(false, result.ID), cookies: []*http.Cookie{owner},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("非公開: status = %d: %s", rec.Code, rec.Body)
	}

	ended := func(label string, body io.Reader, limit int64) {
		t.Helper()
		done := make(chan int64, 1)
		go func() {
			n, _ := io.Copy(io.Discard, io.LimitReader(body, limit))
			done <- n
		}()
		select {
		case n := <-done:
			if n >= limit {
				t.Errorf("%s が終わらずに %d バイト届いた", label, n)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s が終わらない", label)
		}
	}
	ended("ゲストの Range 応答", guestStream.Body, size)
	ended("ゲストのライブ変換", guestTranscode.Body, size)

	// 所有者の応答は続いている。
	if _, err := io.ReadFull(ownerStream.Body, make([]byte, 4<<20)); err != nil {
		t.Errorf("所有者の Range 応答が終わった: %v", err)
	}

	for _, suffix := range []string{"/stream", "/transcode.mp4"} {
		assertStatus(t, "非公開の後のゲストの "+suffix, env.get(base+suffix), http.StatusNotFound)
	}
}

// 動画を引いてから台帳に載るまでに打ち切りが挟まったら、載せずに引き直させる。
func TestGuestLedgerRejectsTrackingAcrossRevocation(t *testing.T) {
	ledger := newGuestLedger()
	since := ledger.generation()
	ledger.revoke([]string{"other"})
	if _, _, ok := ledger.track(context.Background(), httptest.NewRecorder(), "key", since); ok {
		t.Fatal("打ち切りを挟んだ要求が台帳に載った")
	}

	ctx, release, ok := ledger.track(context.Background(), httptest.NewRecorder(), "key", ledger.generation())
	if !ok {
		t.Fatal("台帳に載らない")
	}
	ledger.revoke([]string{"other"})
	if ctx.Err() != nil {
		t.Fatal("別の content_key の打ち切りで終わった")
	}
	ledger.revoke([]string{"key"})
	if ctx.Err() == nil {
		t.Fatal("打ち切っても context が終わらない")
	}
	release()
	if len(ledger.requests) != 0 {
		t.Errorf("release の後に台帳に残った: %v", ledger.requests)
	}
}

// orderedVisibility は1回目（非公開）の反映の中で、2回目（再公開）の切り替えが
// 先の切り替えを待つ（contended）か、待たずに反映へ入ってくる（secondEntered）まで
// 返さない。どちらが先かは時間ではなく出来事で決まる。
type orderedVisibility struct {
	ledger        *guestLedger
	entered       chan struct{}
	contended     chan struct{}
	secondEntered chan struct{}
	calls         atomic.Int32
	overlapped    atomic.Bool
	secondGen     atomic.Uint64
}

func (v *orderedVisibility) SetVideosPublic(_ context.Context, _ []int64, public bool) ([]string, error) {
	v.calls.Add(1)
	if !public {
		close(v.entered)
		select {
		case <-v.contended:
		case <-v.secondEntered:
			// 並べていないと、再公開が非公開の確定から打ち切りまでの間に反映される。
			v.overlapped.Store(true)
		}
		return []string{"key"}, nil
	}
	v.secondGen.Store(v.ledger.generation())
	close(v.secondEntered)
	return []string{"key"}, nil
}

// 非公開の確定から打ち切りまでの間に再公開が割り込まない。割り込むと、再公開の後に
// 始まったゲストの配信を先の非公開の打ち切りが止めてしまう。
func TestVideoVisibilitySwitchesRunOneAtATime(t *testing.T) {
	ledger := newGuestLedger()
	visibility := &orderedVisibility{
		ledger:        ledger,
		entered:       make(chan struct{}),
		contended:     make(chan struct{}),
		secondEntered: make(chan struct{}),
	}
	var waits atomic.Int32
	ledger.onSwitchWait = func() {
		// 待つのは、非公開の反映の中にいる間に来た再公開の1回だけである。
		if waits.Add(1) == 1 {
			close(visibility.contended)
		}
	}
	srv := &server{
		visibility: visibility, guests: ledger,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	put := func(public bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/video-visibility",
			strings.NewReader(visibilityBody(public, 7)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.UpdateVideoVisibility(rec, req)
		return rec
	}
	before := ledger.generation()

	hidden := make(chan *httptest.ResponseRecorder, 1)
	go func() { hidden <- put(false) }()
	<-visibility.entered
	// 非公開の反映が確定して打ち切りの前にいる間に、再公開を送る。
	published := make(chan *httptest.ResponseRecorder, 1)
	go func() { published <- put(true) }()

	for name, ch := range map[string]chan *httptest.ResponseRecorder{"非公開": hidden, "再公開": published} {
		if rec := <-ch; rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", name, rec.Code, rec.Body)
		}
	}
	if n := visibility.calls.Load(); n != 2 {
		t.Fatalf("反映の回数 = %d", n)
	}
	if visibility.overlapped.Load() {
		t.Fatal("再公開が先の非公開の確定から打ち切りまでの間に反映された")
	}
	if visibility.secondGen.Load() == before {
		t.Error("再公開が先の非公開の打ち切りより前に反映された")
	}
}
