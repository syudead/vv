package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 外部連携 API の表示名とサムネイルの位置の一括操作
// （specs/029-video-overrides/contracts/external-api.md §0〜§2）を、本物の認証・保存層と
// Bearer の境界で確かめる。画像の生成だけを fakeThumbnailPicker が代わりにする。

type externalOverridesFixture struct {
	*guestFixture
	secret string
}

func newExternalOverridesFixture(t *testing.T) externalOverridesFixture {
	t.Helper()
	f := newGuestFixture(t, true)
	return externalOverridesFixture{guestFixture: f, secret: f.env.createAPIToken(f.owner, "scraper").Secret}
}

func (f externalOverridesFixture) post(t *testing.T, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, ok := body.(string)
	if !ok {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		raw = string(encoded)
	}
	rec := f.env.serve(authRequest{method: http.MethodPost, target: target, body: raw, header: bearer(f.secret)})
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("%s: Cache-Control = %q", target, got)
	}
	return rec
}

func (f externalOverridesFixture) lookup(t *testing.T, name string) extgen.ExternalVideo {
	t.Helper()
	rec := f.env.serve(authRequest{
		method: http.MethodGet, target: "/api/v1/videos/lookup?id=" + strconv.FormatInt(f.ids[name], 10),
		header: bearer(f.secret),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup %s: status = %d: %s", name, rec.Code, rec.Body)
	}
	return decode[extgen.ExternalVideo](t, rec)
}

func (f externalOverridesFixture) screenVideo(t *testing.T, name string) gen.Video {
	t.Helper()
	return decode[gen.Video](t, f.env.get(f.videoPath(name, ""), f.owner))
}

// wantExternalError は外部連携 API の誤りの応答に期待する値である。reason が空・index が
// -1・limit が 0 なら、その項目が無いことを期待する。
type wantExternalError struct {
	status int
	code   extgen.ErrorCode
	reason extgen.ErrorReason
	index  int
	limit  int
}

func assertExternalError(t *testing.T, label string, rec *httptest.ResponseRecorder, want wantExternalError) {
	t.Helper()
	if rec.Code != want.status {
		t.Errorf("%s: status = %d, want %d: %s", label, rec.Code, want.status, rec.Body)
		return
	}
	var got extgen.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Errorf("%s: 本文を読めない: %v: %s", label, err, rec.Body)
		return
	}
	if got.Code != want.code || got.Message == "" {
		t.Errorf("%s: code = %q, message = %q, want %q", label, got.Code, got.Message, want.code)
	}
	switch {
	case want.reason == "" && got.Reason != nil:
		t.Errorf("%s: reason = %q, want なし", label, *got.Reason)
	case want.reason != "" && (got.Reason == nil || *got.Reason != want.reason):
		t.Errorf("%s: reason = %v, want %q", label, got.Reason, want.reason)
	}
	switch {
	case want.index < 0 && got.Index != nil:
		t.Errorf("%s: index = %d, want なし", label, *got.Index)
	case want.index >= 0 && (got.Index == nil || *got.Index != want.index):
		t.Errorf("%s: index = %v, want %d", label, got.Index, want.index)
	}
	switch {
	case want.limit == 0 && got.Limit != nil:
		t.Errorf("%s: limit = %d, want なし", label, *got.Limit)
	case want.limit != 0 && (got.Limit == nil || *got.Limit != want.limit):
		t.Errorf("%s: limit = %v, want %d", label, got.Limit, want.limit)
	}
}

// 受け入れ条件 8: トークン付きの POST /api/v1/video-display-names が 200 を返し、画面の
// GET /api/videos/{id} の title と、lookup の title・displayName・fileTitle に出る。null と
// 空白だけの名前で戻る。
func TestExternalVideoDisplayNamesApplyAndClear(t *testing.T) {
	f := newExternalOverridesFixture(t)
	aPath := filepath.Join(f.mediaDir, "pub", "a.mp4")

	rec := f.post(t, "/api/v1/video-display-names", map[string]any{"items": []any{
		map[string]any{"video": map[string]any{"path": aPath}, "displayName": "  旅行 2024 夏  "},
		map[string]any{"video": map[string]any{"id": f.ids["b"]}, "displayName": "B の名前"},
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("設定: status = %d: %s", rec.Code, rec.Body)
	}
	out := decode[extgen.VideoDisplayNamesResponse](t, rec)
	if len(out.Items) != 2 {
		t.Fatalf("items = %+v", out.Items)
	}
	first := out.Items[0]
	if first.Video.Id != f.ids["a"] || first.Video.ContentKey != "content-key-a" || first.Title != "旅行 2024 夏" ||
		first.FileTitle != "a" || first.DisplayName == nil || *first.DisplayName != "旅行 2024 夏" {
		t.Errorf("items[0] = %+v", first)
	}
	if second := out.Items[1]; second.Video.Id != f.ids["b"] || second.Title != "B の名前" {
		t.Errorf("items[1] = %+v", second)
	}

	if got := f.screenVideo(t, "a").Title; got != "旅行 2024 夏" {
		t.Errorf("画面の title = %q", got)
	}
	looked := f.lookup(t, "a")
	if looked.Title != "旅行 2024 夏" || looked.FileTitle != "a" ||
		looked.DisplayName == nil || *looked.DisplayName != "旅行 2024 夏" || looked.ThumbnailPositionMs != nil {
		t.Errorf("lookup = title %q, fileTitle %q, displayName %v, thumbnailPositionMs %v",
			looked.Title, looked.FileTitle, looked.DisplayName, looked.ThumbnailPositionMs)
	}

	rec = f.post(t, "/api/v1/video-display-names", map[string]any{"items": []any{
		map[string]any{"video": map[string]any{"contentKey": "content-key-a"}, "displayName": nil},
		map[string]any{"video": map[string]any{"id": f.ids["b"]}, "displayName": "   "},
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("解除: status = %d: %s", rec.Code, rec.Body)
	}
	out = decode[extgen.VideoDisplayNamesResponse](t, rec)
	for i, item := range out.Items {
		if item.DisplayName != nil || item.Title != item.FileTitle {
			t.Errorf("解除 items[%d] = %+v", i, item)
		}
	}
	if got := f.screenVideo(t, "a").Title; got != "a" {
		t.Errorf("解除後の画面の title = %q", got)
	}
	if looked := f.lookup(t, "b"); looked.Title != "b" || looked.DisplayName != nil {
		t.Errorf("解除後の lookup = %+v", looked)
	}
}

// 引けない動画・規則に合わない名前・形の誤りは index 付きの誤りで、何も反映しない。
func TestExternalVideoDisplayNamesRejects(t *testing.T) {
	f := newExternalOverridesFixture(t)
	valid := map[string]any{"video": map[string]any{"id": f.ids["a"]}, "displayName": "変わらない"}
	item := func(ref map[string]any, name any) map[string]any {
		return map[string]any{"video": ref, "displayName": name}
	}

	cases := []struct {
		label string
		body  any
		want  wantExternalError
	}{
		{"引けない動画", map[string]any{"items": []any{valid, item(map[string]any{"contentKey": "missing"}, "x")}},
			wantExternalError{http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.VideoNotFound, 1, 0}},
		{"制御文字", map[string]any{"items": []any{valid, item(map[string]any{"id": f.ids["b"]}, "a\tb")}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.DisplayNameControlCharacters, 1, 0}},
		{"長すぎる", map[string]any{"items": []any{item(map[string]any{"id": f.ids["b"]}, strings.Repeat("あ", 201)), valid}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.DisplayNameTooLong, 0, 200}},
		{"0 件", map[string]any{"items": []any{}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TooManyVideos, -1, 20000}},
		{"items が無い", `{}`,
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TooManyVideos, -1, 20000}},
		{"指定が 2 つ", map[string]any{"items": []any{valid, item(map[string]any{"id": f.ids["b"], "contentKey": "content-key-b"}, "x")}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", 1, 0}},
		{"displayName が無い", map[string]any{"items": []any{valid, map[string]any{"video": map[string]any{"id": f.ids["b"]}}}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", 1, 0}},
		{"displayName の型", map[string]any{"items": []any{item(map[string]any{"id": f.ids["b"]}, 1)}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", 0, 0}},
	}
	for _, tc := range cases {
		assertExternalError(t, tc.label, f.post(t, "/api/v1/video-display-names", tc.body), tc.want)
	}
	if rec := f.post(t, "/api/v1/video-display-names", `{"items":[],"extra":1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("知らない欄: status = %d", rec.Code)
	}
	for _, name := range []string{"a", "b"} {
		if looked := f.lookup(t, name); looked.DisplayName != nil || looked.Title != name {
			t.Errorf("誤りの後に %s が変わった: %+v", name, looked)
		}
	}

	over := make([]any, 20001)
	for i := range over {
		over[i] = valid
	}
	assertExternalError(t, "20001 件", f.post(t, "/api/v1/video-display-names", map[string]any{"items": over}),
		wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TooManyVideos, -1, 20000})

	assertBearerUnauthenticated(t, "トークン無し", f.env.serve(authRequest{
		method: http.MethodPost, target: "/api/v1/video-display-names", body: `{"items":[]}`,
		cookies: []*http.Cookie{f.owner},
	}))
}

func thumbnailItemBody(ref map[string]any, positionMs any) map[string]any {
	return map[string]any{"video": ref, "positionMs": positionMs}
}

// POST /api/v1/video-thumbnails が 200 を返し、画面の thumbnailUrl が変わる。null で戻る。
func TestExternalVideoThumbnailsApplyAndClear(t *testing.T) {
	f := newExternalOverridesFixture(t)
	beforeA, beforeB := thumbnailURLOf(f.screenVideo(t, "a")), thumbnailURLOf(f.screenVideo(t, "b"))

	rec := f.post(t, "/api/v1/video-thumbnails", map[string]any{"items": []any{
		thumbnailItemBody(map[string]any{"path": filepath.Join(f.mediaDir, "pub", "a.mp4")}, 12_500),
		thumbnailItemBody(map[string]any{"contentKey": "content-key-b"}, 0),
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("設定: status = %d: %s", rec.Code, rec.Body)
	}
	out := decode[extgen.VideoThumbnailsResponse](t, rec)
	if len(out.Items) != 2 || out.Items[0].Video.Id != f.ids["a"] || out.Items[0].Video.ContentKey != "content-key-a" ||
		out.Items[0].ThumbnailPositionMs == nil || *out.Items[0].ThumbnailPositionMs != 12_500 ||
		out.Items[1].Video.Id != f.ids["b"] || out.Items[1].ThumbnailPositionMs == nil || *out.Items[1].ThumbnailPositionMs != 0 {
		t.Fatalf("items = %+v", out.Items)
	}
	a := f.screenVideo(t, "a")
	if got := thumbnailURLOf(a); got == "" || got == beforeA {
		t.Errorf("a の thumbnailUrl = %q, 前 = %q", got, beforeA)
	}
	if got := thumbnailURLOf(f.screenVideo(t, "b")); got == "" || got == beforeB {
		t.Errorf("b の thumbnailUrl = %q, 前 = %q", got, beforeB)
	}
	if looked := f.lookup(t, "a"); looked.ThumbnailPositionMs == nil || *looked.ThumbnailPositionMs != 12_500 {
		t.Errorf("lookup の thumbnailPositionMs = %v", looked.ThumbnailPositionMs)
	}
	if got := f.thumbnails.calledPaths(); len(got) != 2 {
		t.Errorf("生成の呼び出し = %v", got)
	}

	rec = f.post(t, "/api/v1/video-thumbnails", map[string]any{"items": []any{
		thumbnailItemBody(map[string]any{"id": f.ids["a"]}, nil),
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("解除: status = %d: %s", rec.Code, rec.Body)
	}
	if out := decode[extgen.VideoThumbnailsResponse](t, rec); len(out.Items) != 1 || out.Items[0].ThumbnailPositionMs != nil {
		t.Errorf("解除 items = %+v", out.Items)
	}
	if got := thumbnailURLOf(f.screenVideo(t, "a")); got != beforeA {
		t.Errorf("解除後の thumbnailUrl = %q, want %q", got, beforeA)
	}
}

// 検証の誤りは何も生成せず index で返し、途中の生成の失敗は 409 と index で、それより前の
// 項目だけが反映される。
func TestExternalVideoThumbnailsRejects(t *testing.T) {
	f := newExternalOverridesFixture(t)
	valid := thumbnailItemBody(map[string]any{"id": f.ids["a"]}, 1_000)

	over := make([]any, 21)
	for i := range over {
		over[i] = valid
	}
	cases := []struct {
		label string
		body  any
		want  wantExternalError
	}{
		{"21 件", map[string]any{"items": over},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TooManyVideos, -1, 20}},
		{"0 件", map[string]any{"items": []any{}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TooManyVideos, -1, 20}},
		{"尺ちょうど", map[string]any{"items": []any{valid, thumbnailItemBody(map[string]any{"id": f.ids["b"]}, 60_000)}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.ThumbnailPositionOutOfRange, 1, 60_000}},
		{"負の位置", map[string]any{"items": []any{thumbnailItemBody(map[string]any{"id": f.ids["b"]}, -1)}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.ThumbnailPositionOutOfRange, 0, 60_000}},
		// d は解析に失敗していて尺が無い。
		{"解析前", map[string]any{"items": []any{valid, thumbnailItemBody(map[string]any{"id": f.ids["d"]}, 1_000)}},
			wantExternalError{http.StatusConflict, extgen.ErrorCodeConflict, extgen.DurationUnknown, 1, 0}},
		{"引けない動画", map[string]any{"items": []any{valid, thumbnailItemBody(map[string]any{"path": "/nowhere.mp4"}, 1)}},
			wantExternalError{http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.VideoNotFound, 1, 0}},
		{"positionMs が無い", map[string]any{"items": []any{map[string]any{"video": map[string]any{"id": f.ids["a"]}}}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", 0, 0}},
		{"小数", map[string]any{"items": []any{valid, thumbnailItemBody(map[string]any{"id": f.ids["b"]}, 1.5)}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", 1, 0}},
		{"指定が無い", map[string]any{"items": []any{thumbnailItemBody(map[string]any{}, 1)}},
			wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", 0, 0}},
	}
	for _, tc := range cases {
		assertExternalError(t, tc.label, f.post(t, "/api/v1/video-thumbnails", tc.body), tc.want)
	}

	// どの所在も開けない。
	if err := os.Remove(filepath.Join(f.mediaDir, "pub", "b.mp4")); err != nil {
		t.Fatal(err)
	}
	assertExternalError(t, "開けない所在",
		f.post(t, "/api/v1/video-thumbnails", map[string]any{"items": []any{valid, thumbnailItemBody(map[string]any{"id": f.ids["b"]}, 1)}}),
		wantExternalError{http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.FileUnavailable, 1, 0})
	if got := f.thumbnails.calledPaths(); len(got) != 0 {
		t.Errorf("検証の誤りなのに生成を呼んだ: %v", got)
	}
	if looked := f.lookup(t, "a"); looked.ThumbnailPositionMs != nil {
		t.Errorf("検証の誤りの後に a の位置が変わった: %v", *looked.ThumbnailPositionMs)
	}

	// 途中（index 1 の c）で生成に失敗すると、a だけが反映され、c と e は反映されない。
	f.thumbnails.failVideo(f.ids["c"])
	rec := f.post(t, "/api/v1/video-thumbnails", map[string]any{"items": []any{
		valid,
		thumbnailItemBody(map[string]any{"id": f.ids["c"]}, 2_000),
		thumbnailItemBody(map[string]any{"id": f.ids["e"]}, 3_000),
	}})
	assertExternalError(t, "途中の生成の失敗", rec,
		wantExternalError{http.StatusConflict, extgen.ErrorCodeConflict, extgen.ThumbnailFrameUnavailable, 1, 0})
	if strings.Contains(rec.Body.String(), "ffmpeg") {
		t.Errorf("生成の失敗の理由が応答に出た: %s", rec.Body)
	}
	if looked := f.lookup(t, "a"); looked.ThumbnailPositionMs == nil || *looked.ThumbnailPositionMs != 1_000 {
		t.Errorf("失敗より前の a が反映されていない: %v", looked.ThumbnailPositionMs)
	}
	for _, name := range []string{"c", "e"} {
		if looked := f.lookup(t, name); looked.ThumbnailPositionMs != nil {
			t.Errorf("失敗した項目とその後の %s が反映された: %v", name, *looked.ThumbnailPositionMs)
		}
	}
	if got := f.thumbnails.calledPaths(); len(got) != 2 {
		t.Errorf("失敗の後も生成を続けた: %v", got)
	}

	assertBearerUnauthenticated(t, "トークン無し", f.env.serve(authRequest{
		method: http.MethodPost, target: "/api/v1/video-thumbnails", body: `{"items":[]}`,
		cookies: []*http.Cookie{f.owner},
	}))
}

// 前の項目を生成している間に後の項目の所在が開けなくなったら、検証のときに決めた所在を
// 使わずに生成の直前に決め直し、file_unavailable と index で止まる。文言はそれより前の
// 項目が反映済みであることを伝える。
func TestExternalVideoThumbnailsResolveEachSourceBeforeGenerating(t *testing.T) {
	f := newExternalOverridesFixture(t)
	f.thumbnails.setAfterSet(func(videoID int64) {
		if videoID != f.ids["a"] {
			return
		}
		if err := os.Remove(filepath.Join(f.mediaDir, "pub", "b.mp4")); err != nil {
			t.Error(err)
		}
	})

	rec := f.post(t, "/api/v1/video-thumbnails", map[string]any{"items": []any{
		thumbnailItemBody(map[string]any{"id": f.ids["a"]}, 1_000),
		thumbnailItemBody(map[string]any{"id": f.ids["b"]}, 2_000),
	}})
	assertExternalError(t, "反映の途中で開けなくなった所在", rec,
		wantExternalError{http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.FileUnavailable, 1, 0})
	var got extgen.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Message, "Nothing was changed") || !strings.Contains(got.Message, "items before it were changed") {
		t.Errorf("message = %q", got.Message)
	}
	if got := f.thumbnails.calledPaths(); len(got) != 1 {
		t.Errorf("開けなくなった所在で生成を呼んだ: %v", got)
	}
	if looked := f.lookup(t, "a"); looked.ThumbnailPositionMs == nil || *looked.ThumbnailPositionMs != 1_000 {
		t.Errorf("前の a が反映されていない: %v", looked.ThumbnailPositionMs)
	}
	if looked := f.lookup(t, "b"); looked.ThumbnailPositionMs != nil {
		t.Errorf("b が反映された: %v", *looked.ThumbnailPositionMs)
	}
}

// 所在を決めたあとに走査がそれを別の内容へ付け替えたら（internal/app が生成の錠の中で
// domain.ErrMediaFileUnavailable を返す）、file_unavailable と index で止まり、その項目は
// 反映しない。
func TestExternalVideoThumbnailsStopAtReassignedSource(t *testing.T) {
	f := newExternalOverridesFixture(t)
	f.thumbnails.setStale(true)

	rec := f.post(t, "/api/v1/video-thumbnails", map[string]any{"items": []any{
		thumbnailItemBody(map[string]any{"id": f.ids["a"]}, 1_000),
	}})
	assertExternalError(t, "付け替わった所在", rec,
		wantExternalError{http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.FileUnavailable, 0, 0})
	if looked := f.lookup(t, "a"); looked.ThumbnailPositionMs != nil {
		t.Errorf("a が反映された: %v", *looked.ThumbnailPositionMs)
	}
}
