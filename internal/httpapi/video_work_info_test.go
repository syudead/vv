package httpapi

import (
	"context"
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

// 作品情報の読み書き（specs/047-video-work-info/contracts/screen-api.md「PUT /api/videos/{id}/work-info」
// 「PUT /api/videos/{id}/details」「Video changes」）は、本物の認証・保存層・経路で確かめる。

func (f *guestFixture) putVideo(name, suffix, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodPut, target: f.videoPath(name, suffix), body: body, cookies: cookies})
}

// createWorkInfoPerson は保存層で人物を作り、その id を返す。
func (f *guestFixture) createWorkInfoPerson(t *testing.T, name string) int64 {
	t.Helper()
	credit, err := f.env.db.Credits().Create(context.Background(), domain.CreditPerson, domain.CreditInput{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return credit.ID
}

// subscribeVideoEvents は保存層の発行を /api/events へつなぎ、つないだ直後の送信を除いた購読を返す。
func (f *guestFixture) subscribeVideoEvents(t *testing.T) *eventSubscriber {
	t.Helper()
	events := NewEvents()
	f.env.db.PublishTo(eventsPublisher{events: events})
	sub, unsubscribe := events.subscribe()
	t.Cleanup(unsubscribe)
	sub.take()
	return sub
}

// rawVideo は応答の動画を JSON のまま返す。省いた欄が無いことを確かめるのに使う。
func rawVideo(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSON を読めない: %v: %s", err, rec.Body)
	}
	return raw
}

// 所有者の PUT は 200 で保存した作品情報（人物は名前付き）を返し、その後の GET が同じものを返す。
// 変更の後にその id の video が流れ、{} の PUT はすべてを解除する。ゲストは workInfo を受け取らない。
func TestSetVideoWorkInfoSavesAndReads(t *testing.T) {
	f := newGuestFixture(t, true)
	sub := f.subscribeVideoEvents(t)
	performer := f.createWorkInfoPerson(t, "出演者")

	body := jsonBody(map[string]any{
		"code":        " ABC-123 ",
		"description": "1 行目\n2 行目",
		"dates":       []map[string]string{{"kind": "release", "date": "2024-02-29"}},
		"people":      []map[string]any{{"role": "performer", "person": map[string]int64{"id": performer}}},
		"urls":        []string{"https://example.com/a"},
		"externalIds": []map[string]string{{"source": "s", "value": "v"}},
	})
	rec := f.putVideo("a", "/work-info", body, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("保存: status = %d: %s", rec.Code, rec.Body)
	}
	saved := decode[gen.Video](t, rec)
	description := "1 行目\n2 行目"
	code := "ABC-123"
	want := gen.WorkInfo{
		Code: &code, Description: &description,
		Dates:         []gen.WorkDate{{Kind: gen.Release, Date: "2024-02-29"}},
		People:        []gen.WorkPerson{{Role: gen.Performer, Person: gen.CreditRef{Id: performer, Name: "出演者"}}},
		Organizations: []gen.WorkOrganization{},
		Urls:          []string{"https://example.com/a"},
		ExternalIds:   []gen.ExternalId{{Source: "s", Value: "v"}},
	}
	if saved.WorkInfo == nil || !reflect.DeepEqual(*saved.WorkInfo, want) {
		t.Errorf("保存の応答の workInfo = %+v, want %+v", saved.WorkInfo, want)
	}
	if saved.Location == nil || len(saved.Tags) != 1 {
		t.Errorf("保存の応答が詳細と同じ形でない: %+v", saved)
	}
	if _, videos := sub.take(); !slices.Equal(videos, []int64{f.ids["a"]}) {
		t.Errorf("/api/events の video = %v, want [%d]", videos, f.ids["a"])
	}

	detail := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner))
	if detail.WorkInfo == nil || !reflect.DeepEqual(*detail.WorkInfo, want) {
		t.Errorf("詳細の workInfo = %+v, want %+v", detail.WorkInfo, want)
	}
	// 一覧の項目には載せない。
	for _, item := range rawItems(t, f.env.get("/api/videos", f.owner).Body.Bytes(), "items") {
		if value, ok := item["workInfo"]; ok {
			t.Errorf("一覧の項目に workInfo = %s", value)
		}
	}
	// ゲストの公開動画の詳細には workInfo が無い（R-10）。
	guest := f.env.get(f.videoPath("a", ""))
	if guest.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", guest.Code, guest.Body)
	}
	assertGuestVideo(t, "ゲストの詳細", rawVideo(t, guest))

	// {} はすべてを解除し、応答に workInfo が無い。
	rec = f.putVideo("a", "/work-info", `{}`, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("解除: status = %d: %s", rec.Code, rec.Body)
	}
	if value, ok := rawVideo(t, rec)["workInfo"]; ok {
		t.Errorf("解除の応答に workInfo = %s", value)
	}
	if _, videos := sub.take(); !slices.Contains(videos, f.ids["a"]) {
		t.Errorf("解除の後の /api/events の video = %v", videos)
	}
	if value, ok := rawVideo(t, f.env.get(f.videoPath("a", ""), f.owner))["workInfo"]; ok {
		t.Errorf("解除の後の詳細に workInfo = %s", value)
	}
}

// 規則に合わない項目は field 付きの誤りで、何も書かない。ゲストは 401、無い動画は 404 である。
func TestSetVideoWorkInfoRejects(t *testing.T) {
	f := newGuestFixture(t, true)
	performer := f.createWorkInfoPerson(t, "出演者")
	invalid := func(reason gen.ErrorReason, limit int) wantError {
		return wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reason, limit: limit}
	}
	samePerson := map[string]any{"role": "performer", "person": map[string]int64{"id": performer}}
	cases := []struct {
		label string
		body  any
		want  wantError
		field string
	}{
		{"長すぎる説明", map[string]any{"description": strings.Repeat("あ", domain.WorkInfoLimits.DescriptionMaxLength+1)},
			invalid(gen.ErrorReasonWorkInfoTooLong, domain.WorkInfoLimits.DescriptionMaxLength), "description"},
		{"暦に無い日", map[string]any{"dates": []map[string]string{{"kind": "release", "date": "2024-02-30"}}},
			invalid(gen.ErrorReasonWorkInfoInvalidDate, 0), "dates[0].date"},
		{"同じ出演者を 2 回", map[string]any{"people": []any{samePerson, samePerson}},
			invalid(gen.ErrorReasonWorkInfoDuplicate, 0), "people[1].person.id"},
		{"無い人物", map[string]any{"people": []any{map[string]any{"role": "director", "person": map[string]int64{"id": 99999}}}},
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}, "people[0].person.id"},
	}
	for _, tc := range cases {
		rec := f.putVideo("a", "/work-info", jsonBody(tc.body), f.owner)
		assertFieldError(t, tc.label, rec, tc.want, tc.field)
	}
	if value, ok := rawVideo(t, f.env.get(f.videoPath("a", ""), f.owner))["workInfo"]; ok {
		t.Errorf("断った後の詳細に workInfo = %s", value)
	}

	for label, body := range map[string]string{
		"種類が列挙に無い": `{"dates":[{"kind":"birthday","date":"2024"}]}`,
		"役割が列挙に無い": `{"people":[{"role":"actor","person":{"id":1}}]}`,
		"知らない欄":    `{"title":"x"}`,
		"本文が null": `null`,
	} {
		assertStatus(t, label, f.putVideo("a", "/work-info", body, f.owner), http.StatusBadRequest)
	}

	assertUnauthenticated(t, "ゲストの保存", f.putVideo("a", "/work-info", `{"code":"X"}`))
	missing := f.env.serve(authRequest{
		method: http.MethodPut, target: "/api/videos/9999/work-info", body: `{}`, cookies: []*http.Cookie{f.owner},
	})
	assertErrorBody(t, "無い動画", missing.Code, missing.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound,
	})
}

// details は表示名と作品情報を 1 回で保存し、どちらかが規則に合わなければどちらも書かない。
func TestSetVideoDetails(t *testing.T) {
	f := newGuestFixture(t, true)
	sub := f.subscribeVideoEvents(t)
	details := func(name string, info any) string {
		return jsonBody(map[string]any{"displayName": name, "workInfo": info})
	}

	rec := f.putVideo("a", "/details", details(" 新しい題名 ", map[string]any{"code": "ABC-1"}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("保存: status = %d: %s", rec.Code, rec.Body)
	}
	saved := decode[gen.Video](t, rec)
	if saved.Title != "新しい題名" || saved.DisplayName == nil || *saved.DisplayName != "新しい題名" {
		t.Errorf("保存の応答: title = %q, displayName = %v", saved.Title, saved.DisplayName)
	}
	if saved.WorkInfo == nil || saved.WorkInfo.Code == nil || *saved.WorkInfo.Code != "ABC-1" {
		t.Errorf("保存の応答の workInfo = %+v", saved.WorkInfo)
	}
	if _, videos := sub.take(); !slices.Contains(videos, f.ids["a"]) {
		t.Errorf("/api/events の video = %v", videos)
	}
	detail := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner))
	if detail.Title != "新しい題名" || detail.WorkInfo == nil || detail.WorkInfo.Code == nil || *detail.WorkInfo.Code != "ABC-1" {
		t.Errorf("詳細: title = %q, workInfo = %+v", detail.Title, detail.WorkInfo)
	}

	// 作品情報の誤りは workInfo の中の位置を field にし、表示名も書かない。
	rec = f.putVideo("b", "/details", details("別の題名", map[string]any{
		"dates": []map[string]string{{"kind": "release", "date": "2024-02-30"}},
	}), f.owner)
	assertFieldError(t, "暦に無い日", rec, wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonWorkInfoInvalidDate,
	}, "dates[0].date")
	if got := decode[gen.Video](t, f.env.get(f.videoPath("b", ""), f.owner)); got.Title != "b" {
		t.Errorf("断った後の題名 = %q, want b", got.Title)
	}

	// 表示名の誤りは field: displayName で、作品情報も書かない。
	rec = f.putVideo("b", "/details", details(strings.Repeat("あ", domain.DisplayNameMaxLength+1), map[string]any{"code": "X"}), f.owner)
	assertFieldError(t, "長すぎる表示名", rec, wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonDisplayNameTooLong, limit: domain.DisplayNameMaxLength,
	}, "displayName")
	if value, ok := rawVideo(t, f.env.get(f.videoPath("b", ""), f.owner))["workInfo"]; ok {
		t.Errorf("断った後の詳細に workInfo = %s", value)
	}

	for label, body := range map[string]string{
		"displayName が無い": `{"workInfo":{}}`,
		"workInfo が無い":    `{"displayName":"x"}`,
		"workInfo が null": `{"displayName":"x","workInfo":null}`,
	} {
		rec := f.putVideo("b", "/details", body, f.owner)
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}

	assertUnauthenticated(t, "ゲストの保存", f.putVideo("a", "/details", details("x", map[string]any{})))
}
