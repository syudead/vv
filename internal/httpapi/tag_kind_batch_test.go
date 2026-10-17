package httpapi

import (
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 種類のまとめての付け外しと、タグの一覧の kind。

func TestBatchTagsPassesKindActionsToStore(t *testing.T) {
	cases := []struct {
		body       string
		wantAction domain.TagBatchAction
		wantKindID int64
	}{
		{`{"action":"setKind","kindId":3,"ids":[1,2]}`, domain.TagBatchSetKind, 3},
		{`{"action":"clearKind","ids":[1,2]}`, domain.TagBatchClearKind, 0},
	}
	for _, tc := range cases {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch", tc.body)
		if rec.Code != http.StatusOK || fake.operation != "batch" {
			t.Fatalf("%s: response = %d operation=%q: %s", tc.body, rec.Code, fake.operation, rec.Body)
		}
		if fake.lastBatchAction != tc.wantAction || fake.lastBatchKindID != tc.wantKindID {
			t.Errorf("%s: BatchTags(%q, kind %d)", tc.body, fake.lastBatchAction, fake.lastBatchKindID)
		}
	}
}

func TestBatchTagsRejectsKindIDMismatch(t *testing.T) {
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	cases := []struct {
		name string
		body string
	}{
		{"kindId の無い setKind", `{"action":"setKind","ids":[1]}`},
		{"kindId が 0 の setKind", `{"action":"setKind","kindId":0,"ids":[1]}`},
		{"kindId を付けた clearKind", `{"action":"clearKind","kindId":1,"ids":[1]}`},
		{"kindId を付けた delete", `{"action":"delete","kindId":1,"ids":[1]}`},
		{"kindId が null の delete", `{"action":"delete","kindId":null,"ids":[1]}`},
		{"kindId が null の setKind", `{"action":"setKind","kindId":null,"ids":[1]}`},
	}
	for _, tc := range cases {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch", tc.body)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), invalid)
		if fake.operation != "" {
			t.Errorf("%s: 保存層に届いた (%s)", tc.name, fake.operation)
		}
	}
}

func TestBatchTagsSetKindWithMissingKindIsNotFound(t *testing.T) {
	fake := &fakeTags{err: domain.ErrTagKindNotFound}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch",
		`{"action":"setKind","kindId":999,"ids":[1]}`)
	assertErrorBody(t, "無い種類", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonTagKindNotFound})
}

// 確認の数は種類の付け外しを受け付けない（変えても何も消えないので画面は確認をとらない）。
func TestTagImpactRejectsKindActions(t *testing.T) {
	for _, action := range []string{"setKind", "clearKind"} {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/impact",
			`{"action":"`+action+`","ids":[1]}`)
		assertErrorBody(t, action, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
		if fake.operation != "" {
			t.Errorf("%s: 保存層に届いた (%s)", action, fake.operation)
		}
	}
}

func TestListTagsPassesKindToStore(t *testing.T) {
	cases := []struct {
		query string
		want  domain.TagKindFilter
	}{
		{"kind=7", domain.TagKindFilter{ID: 7}},
		{"kind=none", domain.TagKindFilter{None: true}},
	}
	for _, tc := range cases {
		fake := &fakeTags{}
		rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodGet, "/api/tags?"+tc.query+"&tentative=true")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", tc.query, rec.Code, rec.Body)
		}
		want := domain.TagListQuery{Kind: tc.want, TentativeOnly: true, Sort: domain.TagSortName}
		if fake.lastListQuery != want {
			t.Errorf("%s: ListTags(%+v), want %+v", tc.query, fake.lastListQuery, want)
		}
	}
}

func TestListTagsRejectsInvalidKind(t *testing.T) {
	for _, value := range []string{"abc", "0", "-1", "07", "1.5", "NONE", "", "99999999999999999999"} {
		fake := &fakeTags{}
		rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodGet, "/api/tags?kind="+value)
		assertErrorBody(t, "kind="+value, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
		if fake.operation != "" {
			t.Errorf("kind=%s: 保存層に届いた (%s)", value, fake.operation)
		}
	}
}
