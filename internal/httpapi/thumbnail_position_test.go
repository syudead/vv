package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 代表サムネイルの位置の設定・解除（specs/029-video-overrides/contracts/screen-api.md §2）は、
// 本物の認証・保存層で確かめる（#559）。画像の生成だけを fakeThumbnailPicker が代わりにする。

// fakeThumbnailPicker は internal/app の Ingest.SetThumbnailPosition の代わりである。位置を
// domain.CheckThumbnailPosition で確かめ、生成は行わず、記録は本物の保存層に任せる。
type fakeThumbnailPicker struct {
	db *store.DB

	mu sync.Mutex
	// fail があれば生成に失敗したとして何も記録しない。
	fail error
	// paths は読む元として渡された所在である。
	paths []string
}

func (p *fakeThumbnailPicker) SetThumbnailPosition(
	ctx context.Context, videoID int64, path string, positionMs *int64,
) (domain.Video, error) {
	video, err := p.db.Ingest().GetVideo(ctx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if positionMs != nil {
		if err := domain.CheckThumbnailPosition(video, *positionMs); err != nil {
			return domain.Video{}, err
		}
	}
	p.mu.Lock()
	p.paths = append(p.paths, path)
	fail := p.fail
	p.mu.Unlock()
	if fail != nil {
		return domain.Video{}, errors.Join(domain.ErrThumbnailFrameUnavailable, fail)
	}
	return p.db.Overrides().SetThumbnailPosition(ctx, videoID, positionMs)
}

func (p *fakeThumbnailPicker) setFail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fail = err
}

func (p *fakeThumbnailPicker) calledPaths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.paths)
}

func thumbnailPositionBody(positionMs *int64) string {
	body, _ := json.Marshal(map[string]*int64{"positionMs": positionMs})
	return string(body)
}

func (f *guestFixture) setThumbnailPosition(name, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{
		method: http.MethodPut, target: f.videoPath(name, "/thumbnail-position"),
		body: body, cookies: cookies,
	})
}

// thumbnailURLOf は動画の thumbnailUrl を返す。無ければ空。
func thumbnailURLOf(video gen.Video) string {
	if video.ThumbnailUrl == nil {
		return ""
	}
	return *video.ThumbnailUrl
}

// libraryThumbnailURL はライブラリの応答の動画 id の thumbnailUrl を返す。
func libraryThumbnailURL(t *testing.T, page gen.LibraryPage, id int64) string {
	t.Helper()
	for _, item := range page.Items {
		if item.Video != nil && item.Video.Id == id {
			return thumbnailURLOf(*item.Video)
		}
	}
	t.Fatalf("ライブラリに動画 %d が無い", id)
	return ""
}

// 所有者の PUT は 200 と thumbnailPositionMs 付きの Video を返し、thumbnailUrl の版が変わり、
// ライブラリの同じ動画も同じ URL になる（受け入れ条件 5）。同じ位置の指定し直しでも版が変わり、
// ゲストの thumbnailUrl は所有者と同じで位置を含まない。null で位置が消え URL が元に戻る
// （受け入れ条件 6）。確定後に /api/events の video が流れる。
func TestSetVideoThumbnailPositionAppliesAndClears(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	events := NewEvents()
	env.db.PublishTo(eventsPublisher{events: events})
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take() // つないだ直後の送信を除く。

	original := thumbnailURLOf(decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)))
	if original == "" {
		t.Fatal("前提: 動画 a に thumbnailUrl が無い")
	}

	position := int64(12_345)
	rec := f.setThumbnailPosition("a", thumbnailPositionBody(&position), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("設定: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	saved := decode[gen.Video](t, rec)
	if saved.Id != f.ids["a"] || saved.ThumbnailState != gen.VideoThumbnailStateDone {
		t.Errorf("設定の応答: id = %d, thumbnailState = %q", saved.Id, saved.ThumbnailState)
	}
	if saved.ThumbnailPositionMs == nil || *saved.ThumbnailPositionMs != position {
		t.Errorf("設定の応答の thumbnailPositionMs = %v, want %d", saved.ThumbnailPositionMs, position)
	}
	first := thumbnailURLOf(saved)
	if first == "" || first == original {
		t.Errorf("設定の応答の thumbnailUrl = %q, 前 = %q", first, original)
	}
	// GET /api/videos/{id} と同じ形（所在・タグ・再生位置を含む）。
	if saved.Location == nil || saved.Progress == nil || len(saved.Tags) != 1 {
		t.Errorf("設定の応答が詳細と同じ形でない: %+v", saved)
	}
	// 読む元は配信と同じ規則で開けた所在である。
	if got := f.thumbnails.calledPaths(); !slices.Equal(got, []string{filepath.Join(f.mediaDir, "pub", "a.mp4")}) {
		t.Errorf("読む元 = %v", got)
	}
	if _, videos := sub.take(); !slices.Equal(videos, []int64{f.ids["a"]}) {
		t.Errorf("/api/events の video = %v, want [%d]", videos, f.ids["a"])
	}

	library := decode[gen.LibraryPage](t, env.get("/api/library", f.owner))
	if got := libraryThumbnailURL(t, library, f.ids["a"]); got != first {
		t.Errorf("ライブラリの thumbnailUrl = %q, want %q", got, first)
	}

	// 同じ位置の指定し直しでも版が変わる。
	again := decode[gen.Video](t, f.setThumbnailPosition("a", thumbnailPositionBody(&position), f.owner))
	second := thumbnailURLOf(again)
	if second == "" || second == first {
		t.Errorf("指定し直しの thumbnailUrl = %q, 前 = %q", second, first)
	}

	// ゲストは所有者と同じ URL を受け取り、位置の値を受け取らない（受け入れ条件 9）。
	guest := env.get(f.videoPath("a", ""))
	if guest.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", guest.Code, guest.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(guest.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "ゲストの詳細", raw)
	var guestURL string
	if err := json.Unmarshal(raw["thumbnailUrl"], &guestURL); err != nil {
		t.Fatal(err)
	}
	if guestURL != second || strings.Contains(guestURL, strconv.FormatInt(position, 10)) {
		t.Errorf("ゲストの thumbnailUrl = %q, 所有者 = %q", guestURL, second)
	}

	// null で位置が消え、URL が元に戻る。
	rec = f.setThumbnailPosition("a", `{"positionMs":null}`, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("解除: status = %d: %s", rec.Code, rec.Body)
	}
	cleared := decode[gen.Video](t, rec)
	if cleared.ThumbnailPositionMs != nil || thumbnailURLOf(cleared) != original {
		t.Errorf("解除の応答: thumbnailPositionMs = %v, thumbnailUrl = %q, want %q",
			cleared.ThumbnailPositionMs, thumbnailURLOf(cleared), original)
	}
	if _, videos := sub.take(); !slices.Contains(videos, f.ids["a"]) {
		t.Errorf("解除の後の /api/events の video = %v", videos)
	}
}

// ゲストの PUT は 401、解析前は 409 duration_unknown、尺の外は 400 と limit、生成の失敗は
// 409 で前の thumbnailUrl のまま、開けない所在は 404 file_unavailable、無い動画は 404 になる。
func TestSetVideoThumbnailPositionRejects(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	position := int64(1_000)

	assertUnauthenticated(t, "Cookie なしの設定", f.setThumbnailPosition("a", thumbnailPositionBody(&position)))

	const durationMs = 60_000
	outOfRange := wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
		reason: reasonThumbnailPositionOutOfRange, limit: durationMs,
	}
	cases := []struct {
		label string
		video string
		body  string
		want  wantError
	}{
		{"尺ちょうど", "a", `{"positionMs":60000}`, outOfRange},
		{"負の位置", "a", `{"positionMs":-1}`, outOfRange},
		// d は解析に失敗していて尺が無い。
		{"解析前", "d", `{"positionMs":1000}`, wantError{
			status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonDurationUnknown,
		}},
	}
	for _, tc := range cases {
		rec := f.setThumbnailPosition(tc.video, tc.body, f.owner)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
	}
	if got := f.thumbnails.calledPaths(); len(got) != 0 {
		t.Errorf("位置が使えないのに生成を呼んだ: %v", got)
	}
	if rec := f.setThumbnailPosition("a", `{"positionMs":59999}`, f.owner); rec.Code != http.StatusOK {
		t.Errorf("尺の直前: status = %d: %s", rec.Code, rec.Body)
	}

	// 生成の失敗では前の画像と位置が残る。
	before := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	f.thumbnails.setFail(errors.New("ffmpeg: no frame"))
	rec := f.setThumbnailPosition("a", thumbnailPositionBody(&position), f.owner)
	assertErrorBody(t, "生成の失敗", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailFrameUnavailable,
	})
	if strings.Contains(rec.Body.String(), "ffmpeg") {
		t.Errorf("生成の失敗の理由が応答に出た: %s", rec.Body)
	}
	after := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	if thumbnailURLOf(after) != thumbnailURLOf(before) ||
		after.ThumbnailPositionMs == nil || *after.ThumbnailPositionMs != 59_999 {
		t.Errorf("生成の失敗の後: thumbnailUrl = %q (前 %q), thumbnailPositionMs = %v",
			thumbnailURLOf(after), thumbnailURLOf(before), after.ThumbnailPositionMs)
	}
	f.thumbnails.setFail(nil)

	for label, body := range map[string]string{
		"positionMs が無い": `{}`,
		"知らない欄":          `{"positionMs":1,"extra":1}`,
		"型が違う":           `{"positionMs":"1"}`,
		"小数":             `{"positionMs":1.5}`,
	} {
		assertStatus(t, label, f.setThumbnailPosition("a", body, f.owner), http.StatusBadRequest)
	}

	// どの所在も開けない。
	if err := os.Remove(filepath.Join(f.mediaDir, "pub", "b.mp4")); err != nil {
		t.Fatal(err)
	}
	rec = f.setThumbnailPosition("b", thumbnailPositionBody(&position), f.owner)
	assertErrorBody(t, "開けない所在", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonFileUnavailable,
	})

	missing := env.serve(authRequest{
		method: http.MethodPut, target: "/api/videos/9999/thumbnail-position",
		body: thumbnailPositionBody(&position), cookies: []*http.Cookie{f.owner},
	})
	assertErrorBody(t, "無い動画", missing.Code, missing.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound,
	})
}
