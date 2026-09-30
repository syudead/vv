package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 表示名の設定・解除（specs/029-video-overrides/contracts/screen-api.md §0・§1）は、本物の
// 認証・保存層で確かめる（#557）。

func displayNameBody(name string) string {
	body, _ := json.Marshal(map[string]string{"displayName": name})
	return string(body)
}

func (f *guestFixture) setDisplayName(name, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{
		method: http.MethodPut, target: f.videoPath(name, "/display-name"),
		body: body, cookies: cookies,
	})
}

// eventsPublisher は保存層が確定後に発行する変化を画面への知らせへ渡す（cmd/mdm の配線と同じ向き）。
type eventsPublisher struct{ events *Events }

func (p eventsPublisher) Publish(events ...domain.Event) {
	for _, event := range events {
		p.events.Handle(event)
	}
}

// libraryVideoTitles はライブラリの応答の動画の項目の題名を並びのまま返す。
func libraryVideoTitles(page gen.LibraryPage) []string {
	var titles []string
	for _, item := range page.Items {
		if item.Video != nil {
			titles = append(titles, item.Video.Title)
		}
	}
	return titles
}

// 所有者の PUT は 200 と反映済みの Video を返し、詳細・一覧・ライブラリ・関連動画・ゲストの
// 詳細の title が表示名になる（受け入れ条件 1）。空文字で元の題名に戻る（受け入れ条件 2）。
// 確定後に /api/events の video が流れる。
func TestSetVideoDisplayNameAppliesAndClears(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	events := NewEvents()
	env.db.PublishTo(eventsPublisher{events: events})
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take() // つないだ直後の送信を除く。

	rec := f.setDisplayName("a", displayNameBody("  My Movie  "), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("設定: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	saved := decode[gen.Video](t, rec)
	if saved.Id != f.ids["a"] || saved.Title != "My Movie" {
		t.Errorf("設定の応答: id = %d, title = %q", saved.Id, saved.Title)
	}
	if saved.FileTitle == nil || *saved.FileTitle != "a" {
		t.Errorf("設定の応答の fileTitle = %v, want a", saved.FileTitle)
	}
	if saved.DisplayName == nil || *saved.DisplayName != "My Movie" {
		t.Errorf("設定の応答の displayName = %v, want My Movie", saved.DisplayName)
	}
	// GET /api/videos/{id} と同じ形（所在・タグ・再生位置を含む）。
	if saved.Location == nil || saved.Progress == nil || len(saved.Tags) != 1 {
		t.Errorf("設定の応答が詳細と同じ形でない: %+v", saved)
	}
	if _, videos := sub.take(); !slices.Equal(videos, []int64{f.ids["a"]}) {
		t.Errorf("/api/events の video = %v, want [%d]", videos, f.ids["a"])
	}

	detail := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	if detail.Title != "My Movie" || detail.DisplayName == nil || detail.FileTitle == nil {
		t.Errorf("所有者の詳細: %+v", detail)
	}
	list := decode[gen.VideoPage](t, env.get("/api/videos", f.owner))
	if !slices.ContainsFunc(list.Items, func(v gen.Video) bool { return v.Id == f.ids["a"] && v.Title == "My Movie" }) {
		t.Errorf("所有者の一覧に表示名が無い: %v", titlesOf(list.Items))
	}
	library := decode[gen.LibraryPage](t, env.get("/api/library?query=my%20movie", f.owner))
	if got := libraryVideoTitles(library); !slices.Equal(got, []string{"My Movie"}) {
		t.Errorf("所有者のライブラリの検索 = %v, want [My Movie]", got)
	}
	related := decode[gen.RelatedVideos](t, env.get(f.videoPath("b", "/related"), f.owner))
	if !slices.ContainsFunc(related.Items, func(v gen.Video) bool { return v.Id == f.ids["a"] && v.Title == "My Movie" }) {
		t.Errorf("関連動画に表示名が無い: %v", titlesOf(related.Items))
	}

	// ゲストは title で表示名を受け取り、上書きの項目を受け取らない（受け入れ条件 9）。
	guest := env.get(f.videoPath("a", ""))
	if guest.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", guest.Code, guest.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(guest.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "ゲストの詳細", raw)
	if got := string(raw["title"]); got != `"My Movie"` {
		t.Errorf("ゲストの詳細の title = %s, want \"My Movie\"", got)
	}

	// 他の動画と同じ表示名も受け付ける。
	if rec := f.setDisplayName("b", displayNameBody("My Movie"), f.owner); rec.Code != http.StatusOK {
		t.Errorf("同じ表示名: status = %d: %s", rec.Code, rec.Body)
	}

	// 空白だけは解除で、元の題名に戻る。
	rec = f.setDisplayName("a", displayNameBody(" 　 "), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("解除: status = %d: %s", rec.Code, rec.Body)
	}
	cleared := decode[gen.Video](t, rec)
	if cleared.Title != "a" || cleared.DisplayName != nil || cleared.FileTitle == nil || *cleared.FileTitle != "a" {
		t.Errorf("解除の応答: title = %q, displayName = %v, fileTitle = %v", cleared.Title, cleared.DisplayName, cleared.FileTitle)
	}
	if got := decode[gen.Video](t, env.get(f.videoPath("a", ""))); got.Title != "a" {
		t.Errorf("解除の後のゲストの詳細の title = %q, want a", got.Title)
	}
	if _, videos := sub.take(); !slices.Contains(videos, f.ids["a"]) {
		t.Errorf("解除の後の /api/events の video = %v", videos)
	}
}

// ゲストの PUT は 401、規則に合わない名前は 400 と reason、無い動画は 404 になる。
func TestSetVideoDisplayNameRejects(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	assertUnauthenticated(t, "Cookie なしの設定", f.setDisplayName("a", displayNameBody("x")))
	if got := decode[gen.Video](t, env.get(f.videoPath("a", ""))); got.Title != "a" {
		t.Errorf("ゲストの設定で題名が変わった: %q", got.Title)
	}

	cases := []struct {
		label string
		body  string
		want  wantError
	}{
		{"制御文字", displayNameBody("a\nb"), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonDisplayNameControlCharacters,
		}},
		{"長すぎる", displayNameBody(strings.Repeat("あ", domain.DisplayNameMaxLength+1)), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonDisplayNameTooLong, limit: domain.DisplayNameMaxLength,
		}},
	}
	for _, tc := range cases {
		rec := f.setDisplayName("a", tc.body, f.owner)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
	}
	if rec := f.setDisplayName("a", displayNameBody(strings.Repeat("あ", domain.DisplayNameMaxLength)), f.owner); rec.Code != http.StatusOK {
		t.Errorf("上限ちょうど: status = %d: %s", rec.Code, rec.Body)
	}

	for label, body := range map[string]string{
		"displayName が無い": `{}`,
		"知らない欄":           `{"displayName":"x","extra":1}`,
		"型が違う":            `{"displayName":1}`,
	} {
		assertStatus(t, label, f.setDisplayName("a", body, f.owner), http.StatusBadRequest)
	}

	missing := env.serve(authRequest{
		method: http.MethodPut, target: "/api/videos/9999/display-name",
		body: displayNameBody("x"), cookies: []*http.Cookie{f.owner},
	})
	assertErrorBody(t, "無い動画", missing.Code, missing.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound,
	})
}

func titlesOf(videos []gen.Video) []string {
	titles := make([]string, 0, len(videos))
	for _, video := range videos {
		titles = append(titles, video.Title)
	}
	return titles
}
