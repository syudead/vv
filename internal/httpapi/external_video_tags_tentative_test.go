package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 外部連携 API の仮の付与（specs/031-tentative-tags/contracts/external-api.md §1）を、本物の
// 保存先と Bearer の境界で確かめる。

func tentativeTagsBody(videos []map[string]any, action string, tags []string, tentative any) string {
	body := map[string]any{"videos": videos, "action": action, "tags": tags}
	if tentative != nil {
		body["tentative"] = tentative
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

// externalTagStates は GET /api/v1/tags の名前ごとの仮かどうかである。
func (f externalTagsFixture) externalTagStates(t *testing.T) map[string]bool {
	t.Helper()
	rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(f.secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/tags: %d %s", rec.Code, rec.Body)
	}
	var list extgen.TagList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, tag := range list.Items {
		out[tag.Name] = tag.Tentative
	}
	return out
}

// screenTagStates は画面の GET /api/tags の名前ごとの仮かどうかである（持ち主のセッション）。
func (f externalTagsFixture) screenTagStates(t *testing.T) map[string]bool {
	t.Helper()
	rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/tags", cookies: []*http.Cookie{f.owner}})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tags: %d %s", rec.Code, rec.Body)
	}
	out := map[string]bool{}
	for _, tag := range decode[gen.TagList](t, rec).Items {
		out[tag.Name] = tag.Tentative
	}
	return out
}

// rejectByName は名前 name のタグを仮として作ってから却下し、却下した名前の一覧に入れる。
func (f externalTagsFixture) rejectByName(t *testing.T, name string) {
	t.Helper()
	out := f.apply(t, tentativeTagsBody([]map[string]any{{"id": f.ids[1]}}, "add", []string{name}, true))
	var id int64
	for _, tag := range out.Items[0].Tags {
		if tag.Name == name {
			id = tag.Id
		}
	}
	if id == 0 {
		t.Fatalf("%s が付かなかった: %+v", name, out)
	}
	if _, err := f.env.db.Tags().RejectTag(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

// 受け入れ条件 1: tentative: true で無い名前を送ると 200 で、その名前が仮のタグとして付き、
// GET /api/v1/tags でも画面の GET /api/tags でも仮として出る。既存のタグは状態を変えずに付き、複数の動画に付く名前も
// 作るのは 1 つ。
func TestExternalVideoTagsTentativeCreatesTentativeTags(t *testing.T) {
	f := newExternalTagsFixture(t)
	if _, err := f.env.db.Tags().CreateTag(context.Background(), "猫"); err != nil {
		t.Fatal(err)
	}
	out := f.apply(t, tentativeTagsBody([]map[string]any{{"id": f.ids[0]}, {"id": f.ids[1]}}, "add", []string{"犬", "猫"}, true))
	if out.SkippedTags == nil || len(out.SkippedTags) != 0 {
		t.Errorf("skippedTags = %#v, want 空の配列", out.SkippedTags)
	}
	for i, item := range out.Items {
		states := map[string]bool{}
		for _, tag := range item.Tags {
			states[tag.Name] = tag.Tentative
		}
		if len(states) != 2 || !states["犬"] || states["猫"] {
			t.Errorf("item %d の tags = %+v", i, item.Tags)
		}
	}
	if got := f.externalTagStates(t); len(got) != 2 || !got["犬"] || got["猫"] {
		t.Errorf("GET /api/v1/tags = %v（犬を 1 つだけ仮で作る）", got)
	}
	if got := f.screenTagStates(t); len(got) != 2 || !got["犬"] || got["猫"] {
		t.Errorf("GET /api/tags = %v（犬を 1 つだけ仮で作る）", got)
	}
}

// 受け入れ条件 2・15: tentative を省くと確定したタグとして作り、その名前が却下した名前なら
// 一覧から消える。
func TestExternalVideoTagsWithoutTentativeCreatesConfirmedTags(t *testing.T) {
	f := newExternalTagsFixture(t)
	f.rejectByName(t, "高画質")
	out := f.apply(t, tentativeTagsBody([]map[string]any{{"id": f.ids[0]}}, "add", []string{"高画質", "犬"}, nil))
	if out.SkippedTags == nil || len(out.SkippedTags) != 0 {
		t.Errorf("skippedTags = %#v, want 空の配列", out.SkippedTags)
	}
	for _, tag := range out.Items[0].Tags {
		if tag.Tentative {
			t.Errorf("tentative を省いたのに仮のタグ: %+v", tag)
		}
	}
	if got := f.externalTagStates(t); len(got) != 2 || got["高画質"] || got["犬"] {
		t.Errorf("GET /api/v1/tags = %v", got)
	}
	rejected, err := f.env.db.Tags().ListRejectedTagNames(context.Background(), "", 0)
	if err != nil || len(rejected.Items) != 0 || rejected.Total != 0 {
		t.Errorf("却下した名前 = %v, err=%v（確定で作った名前は一覧から消える）", rejected, err)
	}
}

// 受け入れ条件 9: 却下した名前と新しい名前を一緒に送ると 200 で、新しい名前だけが付き、
// skippedTags に却下した名前が整えた形で 1 回出る。replace では置き換え後の集合に入らない。
// remove では skippedTags が空。
func TestExternalVideoTagsTentativeSkipsRejectedNames(t *testing.T) {
	f := newExternalTagsFixture(t)
	f.rejectByName(t, "高画質")
	videos := []map[string]any{{"id": f.ids[0]}}

	out := f.apply(t, tentativeTagsBody(videos, "add", []string{" 高画質 ", "猫", "高画質"}, true))
	if !slices.Equal(out.SkippedTags, []string{"高画質"}) {
		t.Errorf("add の skippedTags = %#v", out.SkippedTags)
	}
	if got := externalTagLabels(out.Items[0].Tags); strings.Join(got, ",") != "猫:m" {
		t.Errorf("add の後 = %v", got)
	}

	out = f.apply(t, tentativeTagsBody(videos, "replace", []string{"高画質", "犬"}, true))
	if !slices.Equal(out.SkippedTags, []string{"高画質"}) {
		t.Errorf("replace の skippedTags = %#v", out.SkippedTags)
	}
	if got := externalTagLabels(out.Items[0].Tags); strings.Join(got, ",") != "犬:m" {
		t.Errorf("replace の後 = %v", got)
	}

	out = f.apply(t, tentativeTagsBody(videos, "remove", []string{"高画質", "犬"}, true))
	if out.SkippedTags == nil || len(out.SkippedTags) != 0 {
		t.Errorf("remove の skippedTags = %#v, want 空の配列", out.SkippedTags)
	}
	if len(out.Items[0].Tags) != 0 {
		t.Errorf("remove の後 = %+v", out.Items[0].Tags)
	}
	if _, ok := f.externalTagStates(t)["高画質"]; ok {
		t.Errorf("却下した名前のタグが作られた")
	}
}

// tentative が真偽値でなければ本文の形の誤りとして 400 invalid_request。明示の null も
// 省略とは扱わず、確定したタグを作らずに断る。
func TestExternalVideoTagsRejectsNonBooleanTentative(t *testing.T) {
	f := newExternalTagsFixture(t)
	for _, value := range []any{"true", 1, []any{}, json.RawMessage("null")} {
		status, raw := f.post(t, tentativeTagsBody([]map[string]any{{"id": f.ids[0]}}, "add", []string{"猫"}, value))
		var e extgen.Error
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("%s: %v: %s", value, err, raw)
		}
		if status != http.StatusBadRequest || e.Code != extgen.ErrorCodeInvalidRequest || e.Reason != nil {
			t.Errorf("tentative=%s: status = %d: %s", value, status, raw)
		}
	}
	if tags, err := f.env.db.Tags().ListTags(context.Background()); err != nil || len(tags) != 0 {
		t.Errorf("誤りの要求がタグを作った: %+v, err=%v", tags, err)
	}
}
