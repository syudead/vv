package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 作品情報の置き換えと読み出し（specs/047-video-work-info/contracts/screen-api.md）は、本物の
// 認証・保存層で確かめる。

func (f *guestFixture) setWorkInfo(name, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{
		method: http.MethodPut, target: f.videoPath(name, "/work-info"),
		body: body, cookies: cookies,
	})
}

func workInfoBody(t *testing.T, input gen.WorkInfoInput) string {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func ptr[T any](value T) *T { return &value }

// 所有者の PUT は 200 と保存した workInfo を返し、その後の詳細も同じものを返す。変更の後に
// その動画の video の知らせが流れ、{} の PUT は workInfo の無い動画を返す（受け入れ条件 1・2 の
// サーバー側）。一覧の項目には載らない。
func TestSetVideoWorkInfoSavesAndClears(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	events := NewEvents()
	env.db.PublishTo(eventsPublisher{events: events})
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take() // つないだ直後の送信を除く。

	rec := f.setWorkInfo("a", workInfoBody(t, gen.WorkInfoInput{
		Code:        ptr("  ABC-123 "),
		ReleaseDate: ptr("2024-02-29"),
		Maker:       nil,
		Performers:  &[]string{"Bob", " Alice ", "Bob", ""},
		Description: ptr("line one\nline two"),
		SourceUrl:   ptr("https://example.com/works/1"),
	}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("設定: status = %d: %s", rec.Code, rec.Body)
	}
	saved := decode[gen.Video](t, rec)
	want := &gen.WorkInfo{
		Code:        ptr("ABC-123"),
		ReleaseDate: ptr("2024-02-29"),
		Performers:  []string{"Bob", "Alice"},
		Description: ptr("line one\nline two"),
		SourceUrl:   ptr("https://example.com/works/1"),
	}
	if saved.Id != f.ids["a"] || !reflect.DeepEqual(saved.WorkInfo, want) {
		t.Errorf("設定の応答の workInfo = %+v, want %+v", saved.WorkInfo, want)
	}
	if saved.Location == nil || saved.Progress == nil {
		t.Errorf("設定の応答が詳細と同じ形でない: %+v", saved)
	}
	if _, videos := sub.take(); !slices.Equal(videos, []int64{f.ids["a"]}) {
		t.Errorf("/api/events の video = %v, want [%d]", videos, f.ids["a"])
	}

	detail := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	if !reflect.DeepEqual(detail.WorkInfo, want) {
		t.Errorf("詳細の workInfo = %+v, want %+v", detail.WorkInfo, want)
	}
	list := decode[gen.VideoPage](t, env.get("/api/videos", f.owner))
	for _, item := range list.Items {
		if item.WorkInfo != nil {
			t.Errorf("一覧の項目 %d に workInfo がある", item.Id)
		}
	}

	// 出演者だけの作品情報も、performers を持つ workInfo になる。
	rec = f.setWorkInfo("a", `{"performers":["Carol"],"code":null}`, f.owner)
	if got := decode[gen.Video](t, rec); got.WorkInfo == nil || !slices.Equal(got.WorkInfo.Performers, []string{"Carol"}) ||
		got.WorkInfo.Code != nil {
		t.Errorf("出演者だけ: workInfo = %+v", got.WorkInfo)
	}
	sub.take()

	rec = f.setWorkInfo("a", `{}`, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("解除: status = %d: %s", rec.Code, rec.Body)
	}
	if cleared := decode[gen.Video](t, rec); cleared.WorkInfo != nil {
		t.Errorf("解除の応答の workInfo = %+v, want なし", cleared.WorkInfo)
	}
	if got := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)); got.WorkInfo != nil {
		t.Errorf("解除の後の詳細の workInfo = %+v, want なし", got.WorkInfo)
	}
	if _, videos := sub.take(); !slices.Contains(videos, f.ids["a"]) {
		t.Errorf("解除の後の /api/events の video = %v", videos)
	}
}

// ゲストの PUT は 401 で、公開の動画の詳細にも workInfo は無い（research.md R-7）。
func TestSetVideoWorkInfoGuest(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	body := workInfoBody(t, gen.WorkInfoInput{Code: ptr("ABC-123")})
	assertUnauthenticated(t, "Cookie なしの設定", f.setWorkInfo("a", body))
	if got := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)); got.WorkInfo != nil {
		t.Errorf("ゲストの設定で作品情報が付いた: %+v", got.WorkInfo)
	}

	if rec := f.setWorkInfo("a", body, f.owner); rec.Code != http.StatusOK {
		t.Fatalf("所有者の設定: status = %d: %s", rec.Code, rec.Body)
	}
	guest := env.get(f.videoPath("a", ""))
	if guest.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", guest.Code, guest.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(guest.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "ゲストの詳細", raw)
}

// 規則に合わない項目は 400 と reason・field（上限のある理由は limit も）で、何も書かない。
// 無い動画は 404、本文の形の誤りは 400 になる。
func TestSetVideoWorkInfoRejects(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	limits := domain.WorkInfoLimits

	tooManyPerformers := make([]string, limits.PerformersMaxCount+1)
	for i := range tooManyPerformers {
		tooManyPerformers[i] = "p" + strings.Repeat("x", i)
	}
	cases := []struct {
		label  string
		input  gen.WorkInfoInput
		reason gen.ErrorReason
		field  domain.WorkInfoField
		limit  int
	}{
		{"説明が長すぎる", gen.WorkInfoInput{Description: ptr(strings.Repeat("あ", limits.DescriptionMaxLength+1))},
			reasonWorkInfoTooLong, domain.WorkInfoFieldDescription, limits.DescriptionMaxLength},
		{"暦に無い日", gen.WorkInfoInput{ReleaseDate: ptr("2024-02-30")},
			reasonWorkInfoInvalidDate, domain.WorkInfoFieldReleaseDate, 0},
		{"http でない URL", gen.WorkInfoInput{SourceUrl: ptr("ftp://example.com/a")},
			reasonWorkInfoInvalidURL, domain.WorkInfoFieldSourceURL, 0},
		{"品番の改行", gen.WorkInfoInput{Code: ptr("a\nb")},
			reasonWorkInfoControlCharacters, domain.WorkInfoFieldCode, 0},
		{"出演者が多すぎる", gen.WorkInfoInput{Performers: &tooManyPerformers},
			reasonTooManyPerformers, domain.WorkInfoFieldPerformers, limits.PerformersMaxCount},
		// 最初に規則に合わない項目を WorkInfo の順で返す。
		{"2 つの誤り", gen.WorkInfoInput{Maker: ptr("a\tb"), ReleaseDate: ptr("x")},
			reasonWorkInfoInvalidDate, domain.WorkInfoFieldReleaseDate, 0},
	}
	for _, tc := range cases {
		rec := f.setWorkInfo("a", workInfoBody(t, tc.input), f.owner)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: tc.reason, limit: tc.limit,
		})
		var got gen.Error
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Field == nil || string(*got.Field) != string(tc.field) {
			t.Errorf("%s: field = %v, want %q", tc.label, got.Field, tc.field)
		}
	}
	if got := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)); got.WorkInfo != nil {
		t.Errorf("拒んだ要求で作品情報が付いた: %+v", got.WorkInfo)
	}

	for label, body := range map[string]string{
		"知らない欄": `{"code":"x","extra":1}`,
		"型が違う":  `{"code":1}`,
		"出演者の型": `{"performers":"x"}`,
	} {
		assertStatus(t, label, f.setWorkInfo("a", body, f.owner), http.StatusBadRequest)
	}

	missing := env.serve(authRequest{
		method: http.MethodPut, target: "/api/videos/9999/work-info",
		body: `{"code":"x"}`, cookies: []*http.Cookie{f.owner},
	})
	assertErrorBody(t, "無い動画", missing.Code, missing.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound,
	})
}
