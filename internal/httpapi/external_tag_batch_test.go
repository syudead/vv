package httpapi

import (
	"bytes"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

func (f mcpFixture) attachTentative(t *testing.T, session *mcp.ClientSession, name string) extgen.VideoTagsResponse {
	t.Helper()
	var out extgen.VideoTagsResponse
	if callTool(t, session, "update_video_tags", map[string]any{
		"videos": []any{map[string]any{"id": f.videoA}}, "action": "add", "tags": []string{name}, "tentative": true,
	}, &out) {
		t.Fatalf("update_video_tags が誤り: %+v", out)
	}
	return out
}

func (f mcpFixture) hasTag(t *testing.T, name string) bool {
	t.Helper()
	return slices.ContainsFunc(f.getTags(t, "").Items, func(tag extgen.Tag) bool { return tag.Name == name })
}

func TestMCPBatchRejectAndRejectedNames(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	addTentativeTags(t, f, f.videoA, []string{"仮"})
	tentative, confirmed := f.tagIDByName(t, "仮"), f.createTag(t, "確定")
	session := f.connect(t)

	var batch extgen.TagBatchResponse
	if callTool(t, session, "batch_tags", map[string]any{
		"action": "reject", "ids": []int64{tentative, confirmed, 9999, tentative},
	}, &batch) {
		t.Fatalf("batch_tags が誤り: %+v", batch)
	}
	if !slices.Equal(batch.AppliedIds, []int64{tentative}) || !slices.Equal(batch.NotApplicableIds, []int64{confirmed}) ||
		!slices.Equal(batch.NotFoundIds, []int64{9999}) {
		t.Errorf("batch_tags reject = %+v", batch)
	}
	tags := f.getTags(t, "").Items
	if len(tags) != 1 || tags[0].Id != confirmed || tags[0].Tentative {
		t.Errorf("却下後の一覧 = %+v, want 確定したタグだけ", tags)
	}

	var rejected extgen.RejectedTagNameList
	if callTool(t, session, "list_rejected_tag_names", map[string]any{}, &rejected) {
		t.Fatalf("list_rejected_tag_names が誤り: %+v", rejected)
	}
	if !slices.Equal(rejected.Items, []string{"仮"}) || rejected.Total != 1 || rejected.NextCursor != nil {
		t.Errorf("list_rejected_tag_names = %+v", rejected)
	}
	if out := f.attachTentative(t, session, "仮"); !slices.Equal(out.SkippedTags, []string{"仮"}) {
		t.Errorf("却下した名前の仮の付与: skippedTags = %v", out.SkippedTags)
	}
	if f.hasTag(t, "仮") {
		t.Error("却下した名前のタグが作られた")
	}

	for i, want := range []bool{true, false} {
		var forgot extgen.RejectedTagNameForgetResult
		if callTool(t, session, "forget_rejected_tag_name", map[string]any{"name": " 仮 "}, &forgot) {
			t.Fatalf("forget_rejected_tag_name が誤り: %+v", forgot)
		}
		if forgot.Name != "仮" || forgot.Removed != want {
			t.Errorf("forget_rejected_tag_name %d 回目 = %+v, want removed %v", i+1, forgot, want)
		}
	}
	if out := f.attachTentative(t, session, "仮"); len(out.SkippedTags) != 0 {
		t.Errorf("外した名前の仮の付与: skippedTags = %v", out.SkippedTags)
	}
	if !f.hasTag(t, "仮") {
		t.Error("外した名前のタグが作られない")
	}

	var forgot extgen.RejectedTagNameForgetResult
	if callTool(t, session, "forget_rejected_tag_name", map[string]any{"name": "\t"}, &forgot) ||
		forgot.Name != "\t" || forgot.Removed {
		t.Errorf("整えられない名前 = %+v", forgot)
	}
}

func TestMCPBatchConfirmAndDelete(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	addTentativeTags(t, f, f.videoA, []string{"確定する", "仮のまま"})
	toConfirm, stays := f.tagIDByName(t, "確定する"), f.tagIDByName(t, "仮のまま")
	session := f.connect(t)

	var batch extgen.TagBatchResponse
	if callTool(t, session, "batch_tags", map[string]any{"action": "confirm", "ids": []int64{toConfirm}}, &batch) ||
		!slices.Equal(batch.AppliedIds, []int64{toConfirm}) || len(batch.NotFoundIds) != 0 || len(batch.NotApplicableIds) != 0 {
		t.Errorf("batch_tags confirm = %+v", batch)
	}
	var list extgen.TagList
	if callTool(t, session, "list_tags", map[string]any{}, &list) {
		t.Fatalf("list_tags が誤り: %+v", list)
	}
	for _, tag := range list.Items {
		if want := tag.Id == stays; tag.Tentative != want {
			t.Errorf("list_tags の %q: tentative = %v, want %v", tag.Name, tag.Tentative, want)
		}
	}

	batch = extgen.TagBatchResponse{}
	if callTool(t, session, "batch_tags", map[string]any{"action": "delete", "ids": []int64{stays, toConfirm}}, &batch) ||
		!slices.Equal(batch.AppliedIds, []int64{toConfirm}) || !slices.Equal(batch.NotApplicableIds, []int64{stays}) ||
		batch.NotFoundIds == nil || len(batch.NotFoundIds) != 0 {
		t.Errorf("batch_tags delete = %+v", batch)
	}
	if f.hasTag(t, "確定する") || !f.hasTag(t, "仮のまま") {
		t.Errorf("削除後の一覧 = %+v", f.getTags(t, "").Items)
	}
	var rejected extgen.RejectedTagNameList
	if callTool(t, session, "list_rejected_tag_names", map[string]any{}, &rejected) ||
		rejected.Items == nil || len(rejected.Items) != 0 || rejected.Total != 0 {
		t.Errorf("削除後の却下した名前 = %+v, want 空", rejected)
	}
}

func TestExternalTagBatchAndRejectedNamesErrors(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	addTentativeTags(t, f, f.videoA, []string{"仮"})
	tentative := f.tagIDByName(t, "仮")
	before := f.screenTags(t)

	invalid := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", -1, 0}
	tooMany := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TooManyTags, -1, domain.MaxTagBatch}
	for label, tc := range map[string]struct {
		body string
		want wantExternalError
	}{
		"3 値の外の action": {`{"action":"merge","ids":[` + strconv.FormatInt(tentative, 10) + `]}`, invalid},
		"空の ids":        {`{"action":"reject","ids":[]}`, tooMany},
		"上限を超える ids":    {`{"action":"reject","ids":[` + repeatIDs(tentative, domain.MaxTagBatch+1) + `]}`, tooMany},
	} {
		rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/v1/tags/batch", body: tc.body, header: bearer(f.secret)})
		assertExternalError(t, label, rec, tc.want)
	}
	if after := f.screenTags(t); !bytes.Equal(before, after) {
		t.Errorf("誤りの後に一覧が変わった:\n%s\n%s", before, after)
	}

	invalidCursor := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.InvalidCursor, -1, 0}
	for label, tc := range map[string]struct {
		method, target string
		want           wantExternalError
	}{
		"limit 0":    {http.MethodGet, "/api/v1/tags/rejected-names?limit=0", invalid},
		"limit 201":  {http.MethodGet, "/api/v1/tags/rejected-names?limit=201", invalid},
		"読めないカーソル":   {http.MethodGet, "/api/v1/tags/rejected-names?cursor=x", invalidCursor},
		"name の無い削除": {http.MethodDelete, "/api/v1/tags/rejected-names", invalid},
	} {
		rec := f.env.serve(authRequest{method: tc.method, target: tc.target, header: bearer(f.secret)})
		assertExternalError(t, label, rec, tc.want)
	}
}

func TestExternalRejectedTagNamesPages(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	names := []string{"猫10", "犬", "猫9"}
	addTentativeTags(t, f, f.videoA, names)
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		ids = append(ids, f.tagIDByName(t, name))
	}
	if result := f.postExternal(t, "/tags/batch", map[string]any{"action": "reject", "ids": ids}); result.status != http.StatusOK {
		t.Fatalf("POST /api/v1/tags/batch: status = %d: %s", result.status, result.body)
	}

	var got []string
	query := url.Values{"limit": {"2"}}
	for page := 0; ; page++ {
		if page > 2 {
			t.Fatal("ページが終わらない")
		}
		rec := f.env.serve(authRequest{
			method: http.MethodGet, target: "/api/v1/tags/rejected-names?" + query.Encode(), header: bearer(f.secret),
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/tags/rejected-names: status = %d: %s", rec.Code, rec.Body)
		}
		list := decode[extgen.RejectedTagNameList](t, rec)
		if list.Total != 3 {
			t.Errorf("total = %d, want 3", list.Total)
		}
		got = append(got, list.Items...)
		if list.NextCursor == nil {
			break
		}
		query.Set("cursor", *list.NextCursor)
	}
	if want := []string{"犬", "猫9", "猫10"}; !slices.Equal(got, want) {
		t.Errorf("却下した名前 = %v, want %v", got, want)
	}
}

func repeatIDs(id int64, n int) string {
	var b bytes.Buffer
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}
