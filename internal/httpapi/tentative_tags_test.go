package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 仮のタグと却下した名前の画面の API（specs/031-tentative-tags/contracts/screen-api.md）は、
// 本物の保存層と認証で確かめる（guest_test.go の guestFixture）。

// addTentativeTag は動画 video に、tentative が真の一括操作で名前 name の仮のタグを作って付ける。
func (f *guestFixture) addTentativeTag(t *testing.T, video, name string) int64 {
	t.Helper()
	outcome, err := f.env.db.Tags().ApplyVideoTags(context.Background(),
		[]domain.VideoRef{{ID: f.ids[video]}}, domain.VideoTagsAdd, []string{name}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Items) == 1 {
		for _, tag := range outcome.Items[0].Tags {
			if tag.Name == name && tag.Tentative {
				return tag.ID
			}
		}
	}
	t.Fatalf("仮のタグの付与の結果に仮のタグ %q が無い: %+v", name, outcome)
	return 0
}

func (f *guestFixture) ownerRequest(method, target, body string) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{f.owner}})
}

func tagPath(id int64, suffix string) string {
	return "/api/tags/" + strconv.FormatInt(id, 10) + suffix
}

func findTag(items []gen.Tag, id int64) (gen.Tag, bool) {
	for _, item := range items {
		if item.Id == id {
			return item, true
		}
	}
	return gen.Tag{}, false
}

func videoTagIDs(tags []gen.VideoTag) []int64 {
	ids := make([]int64, 0, len(tags))
	for _, tag := range tags {
		ids = append(ids, tag.Id)
	}
	return ids
}

// 要件 5: タグの一覧・動画のタグ・付与の応答・要約に tentative が出る。
func TestTentativeIsCarriedOnTagReads(t *testing.T) {
	f := newGuestFixture(t, true)
	tentativeID := f.addTentativeTag(t, "b", "仮のタグ")

	list := decode[gen.TagList](t, f.ownerRequest(http.MethodGet, "/api/tags", ""))
	if tag, ok := findTag(list.Items, tentativeID); !ok || !tag.Tentative {
		t.Errorf("GET /api/tags の仮のタグ = %+v (ok %v)", tag, ok)
	}
	if tag, ok := findTag(list.Items, f.tagID); !ok || tag.Tentative {
		t.Errorf("GET /api/tags の確定したタグ = %+v (ok %v)", tag, ok)
	}

	video := decode[gen.Video](t, f.ownerRequest(http.MethodGet, f.videoPath("b", ""), ""))
	for _, tag := range video.Tags {
		if want := tag.Id == tentativeID; tag.Tentative != want {
			t.Errorf("GET /api/videos/{id} の tags の %s: tentative = %v, want %v", tag.Name, tag.Tentative, want)
		}
	}
	if !slices.Contains(videoTagIDs(video.Tags), tentativeID) {
		t.Errorf("動画 b の tags に仮のタグが無い: %+v", video.Tags)
	}

	// 手での付与は、既存の仮のタグを仮のままにする。
	attach := f.ownerRequest(http.MethodPost, "/api/video-tags",
		`{"videoIds":[`+strconv.FormatInt(f.ids["c"], 10)+`],"action":"add","tag":{"id":`+strconv.FormatInt(tentativeID, 10)+`}}`)
	if attach.Code != http.StatusOK {
		t.Fatalf("POST /api/video-tags: status = %d: %s", attach.Code, attach.Body)
	}
	if got := decode[gen.VideoTagsResponse](t, attach); got.Tag.Id != tentativeID || !got.Tag.Tentative {
		t.Errorf("POST /api/video-tags の tag = %+v", got.Tag)
	}

	summary := f.ownerRequest(http.MethodPost, "/api/video-tags/summary", `{"videoIds":[`+strconv.FormatInt(f.ids["b"], 10)+`]}`)
	if summary.Code != http.StatusOK {
		t.Fatalf("POST /api/video-tags/summary: status = %d: %s", summary.Code, summary.Body)
	}
	for _, item := range decode[gen.VideoTagsSummary](t, summary).Items {
		if want := item.Tag.Id == tentativeID; item.Tag.Tentative != want {
			t.Errorf("summary の %s: tentative = %v, want %v", item.Tag.Name, item.Tag.Tentative, want)
		}
	}
}

// 確定は 200 と tentative が偽の Tag を返し、既に確定していても 200 を返す。
func TestConfirmTentativeTag(t *testing.T) {
	f := newGuestFixture(t, true)
	id := f.addTentativeTag(t, "b", "仮のタグ")

	for _, label := range []string{"仮のタグ", "確定済みのタグ"} {
		rec := f.ownerRequest(http.MethodPost, tagPath(id, "/confirm"), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", label, rec.Code, rec.Body)
		}
		if got := decode[gen.Tag](t, rec); got.Id != id || got.Name != "仮のタグ" || got.Tentative || got.VideoCount != 1 {
			t.Errorf("%s: Tag = %+v", label, got)
		}
	}
	video := decode[gen.Video](t, f.ownerRequest(http.MethodGet, f.videoPath("b", ""), ""))
	if !slices.Contains(videoTagIDs(video.Tags), id) {
		t.Errorf("確定したタグが動画から外れた: %+v", video.Tags)
	}

	if rec := f.ownerRequest(http.MethodPost, tagPath(999999, "/confirm"), ""); rec.Code != http.StatusNotFound || decodeError(t, rec) != "tag_not_found" {
		t.Errorf("無いタグの確定: status = %d: %s", rec.Code, rec.Body)
	}
}

// 受け入れ条件 8: 却下したタグは一覧と動画から消え、名前が却下した名前の一覧に出る。
// 確定したタグの却下は 409 tag_not_tentative で何も変えない。
func TestRejectTentativeTag(t *testing.T) {
	f := newGuestFixture(t, true)
	id := f.addTentativeTag(t, "b", "仮のタグ")

	rec := f.ownerRequest(http.MethodPost, tagPath(id, "/reject"), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("却下: status = %d: %s", rec.Code, rec.Body)
	}
	if _, ok := findTag(decode[gen.TagList](t, f.ownerRequest(http.MethodGet, "/api/tags", "")).Items, id); ok {
		t.Error("却下したタグが GET /api/tags に残っている")
	}
	video := decode[gen.Video](t, f.ownerRequest(http.MethodGet, f.videoPath("b", ""), ""))
	if slices.Contains(videoTagIDs(video.Tags), id) {
		t.Errorf("却下したタグが動画 b に残っている: %+v", video.Tags)
	}
	names := decode[gen.RejectedTagNameList](t, f.ownerRequest(http.MethodGet, "/api/tags/rejected-names", ""))
	if !slices.Equal(names.Items, []string{"仮のタグ"}) {
		t.Errorf("却下した名前 = %v, want [仮のタグ]", names.Items)
	}

	if rec := f.ownerRequest(http.MethodPost, tagPath(id, "/reject"), ""); rec.Code != http.StatusNotFound || decodeError(t, rec) != "tag_not_found" {
		t.Errorf("消えたタグの却下: status = %d: %s", rec.Code, rec.Body)
	}

	confirmed := f.ownerRequest(http.MethodPost, tagPath(f.tagID, "/reject"), "")
	if confirmed.Code != http.StatusConflict || decodeError(t, confirmed) != "tag_not_tentative" {
		t.Fatalf("確定したタグの却下: status = %d: %s", confirmed.Code, confirmed.Body)
	}
	if tag, ok := findTag(decode[gen.TagList](t, f.ownerRequest(http.MethodGet, "/api/tags", "")).Items, f.tagID); !ok || tag.VideoCount != 2 {
		t.Errorf("確定したタグが変わった: %+v (ok %v)", tag, ok)
	}
	names = decode[gen.RejectedTagNameList](t, f.ownerRequest(http.MethodGet, "/api/tags/rejected-names", ""))
	if !slices.Equal(names.Items, []string{"仮のタグ"}) {
		t.Errorf("確定したタグの却下のあとの却下した名前 = %v", names.Items)
	}
}

// 却下した名前の取り外しは 204 で一覧から消え、無い名前や整えられない入力でも 204 を返す。
func TestForgetRejectedTagName(t *testing.T) {
	f := newGuestFixture(t, true)
	for _, name := range []string{"Beta", "Alpha"} {
		id := f.addTentativeTag(t, "b", name)
		if rec := f.ownerRequest(http.MethodPost, tagPath(id, "/reject"), ""); rec.Code != http.StatusNoContent {
			t.Fatalf("却下 %s: status = %d: %s", name, rec.Code, rec.Body)
		}
	}
	names := decode[gen.RejectedTagNameList](t, f.ownerRequest(http.MethodGet, "/api/tags/rejected-names", ""))
	if !slices.Equal(names.Items, []string{"Alpha", "Beta"}) {
		t.Fatalf("却下した名前 = %v, want [Alpha Beta]", names.Items)
	}

	// 登録時と同じく整えてから照合する。
	for _, name := range []string{"  Beta  ", "無い名前", ""} {
		rec := f.ownerRequest(http.MethodDelete, "/api/tags/rejected-names?name="+url.QueryEscape(name), "")
		if rec.Code != http.StatusNoContent {
			t.Errorf("取り外し %q: status = %d: %s", name, rec.Code, rec.Body)
		}
	}
	names = decode[gen.RejectedTagNameList](t, f.ownerRequest(http.MethodGet, "/api/tags/rejected-names", ""))
	if !slices.Equal(names.Items, []string{"Alpha"}) {
		t.Errorf("取り外しのあとの却下した名前 = %v, want [Alpha]", names.Items)
	}

	if rec := f.ownerRequest(http.MethodDelete, "/api/tags/rejected-names", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("name の無い取り外し: status = %d: %s", rec.Code, rec.Body)
	}
}

// 受け入れ条件 11: 改名・シノニムの追加・統合先では、仮のタグが確定したタグになって返る。
func TestManualEditsReturnConfirmedTag(t *testing.T) {
	f := newGuestFixture(t, true)
	cases := []struct {
		label  string
		method string
		suffix string
		body   func(source int64) string
	}{
		{"改名", http.MethodPatch, "", func(int64) string { return `{"name":"新しい名前"}` }},
		{"シノニムの追加", http.MethodPost, "/synonyms", func(int64) string { return `{"name":"別名"}` }},
		{"統合先", http.MethodPost, "/merge", func(source int64) string { return `{"sourceId":` + strconv.FormatInt(source, 10) + `}` }},
	}
	for index, tc := range cases {
		id := f.addTentativeTag(t, "b", "仮"+strconv.Itoa(index))
		source := f.addTentativeTag(t, "c", "統合元"+strconv.Itoa(index))
		rec := f.ownerRequest(tc.method, tagPath(id, tc.suffix), tc.body(source))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", tc.label, rec.Code, rec.Body)
		}
		if got := decode[gen.Tag](t, rec); got.Id != id || got.Tentative {
			t.Errorf("%s: Tag = %+v", tc.label, got)
		}
	}
}

// 受け入れ条件 4: ゲストは 4 つの経路とも 401 で、ゲストの Video.tags は仮のタグがあっても空のまま。
func TestTentativeTagRoutesAreOwnerOnly(t *testing.T) {
	f := newGuestFixture(t, true)
	id := f.addTentativeTag(t, "a", "仮のタグ")

	for _, req := range []authRequest{
		{method: http.MethodPost, target: tagPath(id, "/confirm")},
		{method: http.MethodPost, target: tagPath(id, "/reject")},
		{method: http.MethodGet, target: "/api/tags/rejected-names"},
		{method: http.MethodDelete, target: "/api/tags/rejected-names?name=x"},
	} {
		assertUnauthenticated(t, "ゲストの "+req.method+" "+req.target, f.env.serve(req))
	}
	if tag, ok := findTag(decode[gen.TagList](t, f.ownerRequest(http.MethodGet, "/api/tags", "")).Items, id); !ok || !tag.Tentative {
		t.Errorf("ゲストの要求のあと仮のタグが変わった: %+v (ok %v)", tag, ok)
	}

	detail := f.env.get(f.videoPath("a", ""))
	if detail.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", detail.Code, detail.Body)
	}
	var video map[string]json.RawMessage
	if err := json.Unmarshal(detail.Body.Bytes(), &video); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "仮のタグを持つ動画のゲストの詳細", video)
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return string(decode[gen.Error](t, rec).Code)
}
