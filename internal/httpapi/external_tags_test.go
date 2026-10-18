package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

func (f mcpFixture) createTag(t *testing.T, name string) int64 {
	t.Helper()
	tag, err := f.env.db.Tags().CreateTag(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return tag.ID
}

func (f mcpFixture) tagIDByName(t *testing.T, name string) int64 {
	t.Helper()
	for _, tag := range f.getTags(t, "").Items {
		if tag.Name == name {
			return tag.Id
		}
	}
	t.Fatalf("タグ %q が無い", name)
	return 0
}

func (f mcpFixture) screenTags(t *testing.T) []byte {
	t.Helper()
	rec := f.env.get("/api/tags", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tags: status = %d: %s", rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

func (f mcpFixture) postExternal(t *testing.T, path string, body any) *httpResult {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := f.env.serve(authRequest{
		method: http.MethodPost, target: "/api/v1" + path, body: string(encoded), header: bearer(f.secret),
	})
	return &httpResult{status: rec.Code, body: rec.Body.Bytes()}
}

type httpResult struct {
	status int
	body   []byte
}

func TestMCPMergeTagsMovesTentativeSource(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	addTentativeTags(t, f, f.videoA, []string{"selfie", "自撮"})
	target, source := f.tagIDByName(t, "自撮"), f.tagIDByName(t, "selfie")
	session := f.connect(t)

	var merged extgen.TagMergeResponse
	if callTool(t, session, "merge_tags", map[string]any{"targetId": target, "sourceIds": []int64{source}}, &merged) {
		t.Fatalf("merge_tags が誤り: %+v", merged)
	}
	if merged.Tag.Id != target || merged.Tag.Tentative || !slices.Equal(merged.Tag.Synonyms, []string{"selfie"}) ||
		merged.NotFoundIds == nil || len(merged.NotFoundIds) != 0 {
		t.Errorf("merge_tags = %+v", merged)
	}

	var screen struct{ Items []struct{ ID int64 } }
	if err := json.Unmarshal(f.screenTags(t), &screen); err != nil {
		t.Fatal(err)
	}
	if len(screen.Items) != 1 || screen.Items[0].ID != target {
		t.Errorf("画面の一覧 = %+v, want 統合先だけ", screen.Items)
	}
	rec := f.env.get("/api/videos/"+strconv.FormatInt(f.videoA, 10), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/videos/{id}: status = %d: %s", rec.Code, rec.Body)
	}
	var video struct {
		Tags []struct {
			ID   int64
			Name string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &video); err != nil {
		t.Fatal(err)
	}
	if len(video.Tags) != 1 || video.Tags[0].ID != target || video.Tags[0].Name != "自撮" {
		t.Errorf("動画のタグ = %+v, want 統合先だけ", video.Tags)
	}
}

func TestExternalMergeTagsErrorsAndMissingSources(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	target, source := f.createTag(t, "猫"), f.createTag(t, "ねこ")
	before := f.screenTags(t)

	sameTag := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.MergeSameTag, -1, 0}
	notFound := wantExternalError{http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.TagNotFound, -1, 0}
	tooMany := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TooManyTags, -1, domain.MaxTagBatch}
	overLimit := make([]int64, domain.MaxTagBatch+1)
	for label, tc := range map[string]struct {
		body any
		want wantExternalError
	}{
		"統合元に統合先":   {map[string]any{"targetId": target, "sourceIds": []int64{source, target}}, sameTag},
		"無い統合先":     {map[string]any{"targetId": 9999, "sourceIds": []int64{source}}, notFound},
		"空の統合元":     {map[string]any{"targetId": target, "sourceIds": []int64{}}, tooMany},
		"上限を超える統合元": {map[string]any{"targetId": target, "sourceIds": overLimit}, tooMany},
	} {
		encoded, err := json.Marshal(tc.body)
		if err != nil {
			t.Fatal(err)
		}
		rec := f.env.serve(authRequest{
			method: http.MethodPost, target: "/api/v1/tags/merge", body: string(encoded), header: bearer(f.secret),
		})
		assertExternalError(t, label, rec, tc.want)
	}
	if after := f.screenTags(t); !bytes.Equal(before, after) {
		t.Errorf("誤りの後に一覧が変わった:\n%s\n%s", before, after)
	}

	session := f.connect(t)
	var merged extgen.TagMergeResponse
	if callTool(t, session, "merge_tags", map[string]any{"targetId": target, "sourceIds": []int64{source, 9999, 9999}}, &merged) {
		t.Fatalf("merge_tags が誤り: %+v", merged)
	}
	if !slices.Equal(merged.NotFoundIds, []int64{9999}) || !slices.Equal(merged.Tag.Synonyms, []string{"ねこ"}) {
		t.Errorf("一部が無い統合 = %+v", merged)
	}
	if tags := f.getTags(t, "").Items; len(tags) != 1 || tags[0].Id != target {
		t.Errorf("統合後の一覧 = %+v", tags)
	}
}

func TestMCPRenameTag(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	ctx := context.Background()
	owner, renamed := f.createTag(t, "旅行"), f.createTag(t, "旅")
	if _, err := f.env.db.Tags().AddSynonym(ctx, owner, "trip", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.db.Tags().AddSynonym(ctx, renamed, "journey", nil); err != nil {
		t.Fatal(err)
	}
	before := f.screenTags(t)
	session := f.connect(t)

	for label, tc := range map[string]struct {
		name  string
		tagID int64
		owner string
	}{
		"別のタグのシノニム": {"trip", owner, "旅行"},
		"別のタグの名前":   {"旅行", owner, "旅行"},
		"自分のシノニム":   {"journey", renamed, "旅"},
	} {
		var body extgen.Error
		if !callTool(t, session, "rename_tag", map[string]any{"id": renamed, "name": tc.name}, &body) {
			t.Errorf("%s: 誤りにならない: %+v", label, body)
			continue
		}
		if body.Code != extgen.ErrorCodeConflict || body.Reason == nil || *body.Reason != extgen.TagNameTaken ||
			body.TagId == nil || *body.TagId != tc.tagID || body.TagName == nil || *body.TagName != tc.owner {
			t.Errorf("%s: %+v", label, body)
		}
	}
	if after := f.screenTags(t); !bytes.Equal(before, after) {
		t.Errorf("衝突の後に一覧が変わった:\n%s\n%s", before, after)
	}

	var body extgen.Error
	if !callTool(t, session, "rename_tag", map[string]any{"id": 9999, "name": "x"}, &body) ||
		body.Code != extgen.ErrorCodeNotFound || body.Reason == nil || *body.Reason != extgen.TagNotFound {
		t.Errorf("無いタグ: %+v", body)
	}
	body = extgen.Error{}
	if !callTool(t, session, "rename_tag", map[string]any{"id": renamed, "name": " "}, &body) ||
		body.Reason == nil || *body.Reason != extgen.TagNameEmpty || body.TagId != nil {
		t.Errorf("空の名前: %+v", body)
	}
	body = extgen.Error{}
	if !callTool(t, session, "rename_tag", map[string]any{"id": renamed, "name": strings.Repeat("a", domain.TagNameMaxLength+1)}, &body) ||
		body.Reason == nil || *body.Reason != extgen.TagNameTooLong || body.Limit == nil || *body.Limit != domain.TagNameMaxLength {
		t.Errorf("長すぎる名前: %+v", body)
	}

	addTentativeTags(t, f, f.videoA, []string{"仮"})
	tentative := f.tagIDByName(t, "仮")
	var tag extgen.Tag
	if callTool(t, session, "rename_tag", map[string]any{"id": tentative, "name": "本"}, &tag) ||
		tag.Id != tentative || tag.Name != "本" || tag.Tentative || tag.VideoCount != 1 {
		t.Errorf("仮のタグの改名 = %+v", tag)
	}
}

func TestMCPUpdateTagSynonyms(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	target := f.createTag(t, "自撮り")
	addTentativeTags(t, f, f.videoA, []string{"自己撮影"})
	other := f.tagIDByName(t, "自己撮影")
	before := f.screenTags(t)
	session := f.connect(t)

	add := map[string]any{"id": target, "action": "add", "name": "自己撮影"}
	var body extgen.Error
	if !callTool(t, session, "update_tag_synonyms", add, &body) ||
		body.Code != extgen.ErrorCodeConflict || body.Reason == nil || *body.Reason != extgen.TagMergeRequired ||
		body.TagId == nil || *body.TagId != other || body.TagName == nil || *body.TagName != "自己撮影" {
		t.Errorf("mergeTagId 無しの add: %+v", body)
	}
	body = extgen.Error{}
	if !callTool(t, session, "update_tag_synonyms", map[string]any{
		"id": target, "action": "add", "name": "自己撮影", "mergeTagId": target,
	}, &body) || body.Reason == nil || *body.Reason != extgen.TagMergeRequired {
		t.Errorf("違う mergeTagId の add: %+v", body)
	}
	if after := f.screenTags(t); !bytes.Equal(before, after) {
		t.Errorf("tag_merge_required の後に一覧が変わった:\n%s\n%s", before, after)
	}
	body = extgen.Error{}
	if !callTool(t, session, "update_tag_synonyms", map[string]any{"id": target, "action": "add", "name": "自撮り"}, &body) ||
		body.Reason == nil || *body.Reason != extgen.TagNameTaken || body.TagId == nil || *body.TagId != target {
		t.Errorf("自分の名前の add: %+v", body)
	}

	add["mergeTagId"] = other
	var tag extgen.Tag
	if callTool(t, session, "update_tag_synonyms", add, &tag) {
		t.Fatalf("mergeTagId 付きの add が誤り: %+v", tag)
	}
	if tag.Id != target || !slices.Equal(tag.Synonyms, []string{"自己撮影"}) || tag.VideoCount != 1 {
		t.Errorf("mergeTagId 付きの add = %+v", tag)
	}
	if tags := f.getTags(t, "").Items; len(tags) != 1 || tags[0].Id != target {
		t.Errorf("統合後の一覧 = %+v", tags)
	}

	for range 2 {
		tag = extgen.Tag{}
		if callTool(t, session, "update_tag_synonyms", map[string]any{"id": target, "action": "add", "name": "selfie"}, &tag) ||
			!slices.Equal(tag.Synonyms, []string{"selfie", "自己撮影"}) {
			t.Errorf("新しい名前の add = %+v", tag)
		}
	}
	tag = extgen.Tag{}
	if callTool(t, session, "update_tag_synonyms", map[string]any{"id": target, "action": "remove", "name": " selfie "}, &tag) ||
		tag.Id != target || !slices.Equal(tag.Synonyms, []string{"自己撮影"}) {
		t.Errorf("remove = %+v", tag)
	}
	tag = extgen.Tag{}
	if callTool(t, session, "update_tag_synonyms", map[string]any{"id": target, "action": "remove", "name": "無い"}, &tag) ||
		!slices.Equal(tag.Synonyms, []string{"自己撮影"}) {
		t.Errorf("シノニムでない名前の remove = %+v", tag)
	}

	body = extgen.Error{}
	if !callTool(t, session, "update_tag_synonyms", map[string]any{"id": 9999, "action": "remove", "name": "x"}, &body) ||
		body.Reason == nil || *body.Reason != extgen.TagNotFound {
		t.Errorf("無いタグ: %+v", body)
	}
	result := f.postExternal(t, "/tags/synonyms", map[string]any{"id": target, "action": "replace", "name": "x"})
	if result.status != http.StatusBadRequest || !strings.Contains(string(result.body), `"invalid_request"`) {
		t.Errorf("action が 2 値の外: %d %s", result.status, result.body)
	}
}

// Ignore fixture creation timestamps when comparing UI and external-operation results.
func TestExternalTagEditsMatchScreen(t *testing.T) {
	type step struct {
		screenMethod, screenPath, screenBody string
		externalPath                         string
		externalBody                         any
	}
	build := func(t *testing.T) (mcpFixture, map[string]int64) {
		t.Helper()
		f := newMCPFixture(t, Options{Scans: &fakeScans{}})
		addTentativeTags(t, f, f.videoA, []string{"猫", "ねこ", "ネコ", "犬", "いぬ"})
		ids := map[string]int64{}
		for _, tag := range f.getTags(t, "").Items {
			ids[tag.Name] = tag.Id
		}
		return f, ids
	}
	steps := func(ids map[string]int64) []step {
		cat, kana, katakana, dog, inu := ids["猫"], ids["ねこ"], ids["ネコ"], ids["犬"], ids["いぬ"]
		id := strconv.FormatInt
		return []step{
			{
				http.MethodPost, "/api/tags/" + id(cat, 10) + "/merge", `{"sourceIds":[` + id(kana, 10) + `]}`,
				"/tags/merge", map[string]any{"targetId": cat, "sourceIds": []int64{kana}},
			},
			{
				http.MethodPatch, "/api/tags/" + id(dog, 10), `{"name":"イヌ"}`,
				"/tags/rename", map[string]any{"id": dog, "name": "イヌ"},
			},
			{
				http.MethodPost, "/api/tags/" + id(cat, 10) + "/synonyms", `{"name":"cat"}`,
				"/tags/synonyms", map[string]any{"id": cat, "action": "add", "name": "cat"},
			},
			{
				http.MethodPost, "/api/tags/" + id(cat, 10) + "/synonyms", `{"name":"ネコ","mergeTagId":` + id(katakana, 10) + `}`,
				"/tags/synonyms", map[string]any{"id": cat, "action": "add", "name": "ネコ", "mergeTagId": katakana},
			},
			{
				http.MethodPost, "/api/tags/" + id(inu, 10) + "/synonyms", `{"name":"dog"}`,
				"/tags/synonyms", map[string]any{"id": inu, "action": "add", "name": "dog"},
			},
		}
	}

	screen, screenIDs := build(t)
	for _, s := range steps(screenIDs) {
		rec := screen.env.serve(authRequest{
			method: s.screenMethod, target: s.screenPath, body: s.screenBody, cookies: []*http.Cookie{screen.owner},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("画面 %s %s: status = %d: %s", s.screenMethod, s.screenPath, rec.Code, rec.Body)
		}
	}
	external, externalIDs := build(t)
	for _, s := range steps(externalIDs) {
		if result := external.postExternal(t, s.externalPath, s.externalBody); result.status != http.StatusOK {
			t.Fatalf("外部 %s: status = %d: %s", s.externalPath, result.status, result.body)
		}
	}

	want, got := withoutCreatedAt(t, screen.screenTags(t)), withoutCreatedAt(t, external.screenTags(t))
	if want != got {
		t.Errorf("GET /api/tags が一致しない:\n画面 %s\n外部 %s", want, got)
	}
	if !strings.Contains(got, `"name":"猫"`) || !strings.Contains(got, `"name":"イヌ"`) {
		t.Errorf("操作が反映されていない: %s", got)
	}
}

func withoutCreatedAt(t *testing.T, body []byte) string {
	t.Helper()
	var list map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	items, _ := list["items"].([]any)
	for _, item := range items {
		if tag, ok := item.(map[string]any); ok {
			delete(tag, "createdAt")
		}
	}
	out, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
