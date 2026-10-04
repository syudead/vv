package httpapi

import (
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// まとめての確定・却下・削除と確認の数（specs/036-tag-admin-scale/contracts/screen-api.md §1・§3）。

// idsJSON は 1 から n までの id の JSON の配列を返す。
func idsJSON(n int) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = strconv.Itoa(i + 1)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestBatchTagsReturnsThreeArraysInRequestOrder(t *testing.T) {
	fake := &fakeTags{batchOutcome: domain.TagBatchOutcome{
		AppliedIDs:       []int64{3, 1},
		NotFoundIDs:      []int64{9},
		NotApplicableIDs: []int64{2},
	}}
	handler := newTestServer(t, Options{Tags: fake})
	rec := jsonRequest(t, handler, http.MethodPost, "/api/tags/batch", `{"action":"confirm","ids":[3,2,9,1,3]}`)
	if rec.Code != http.StatusOK || fake.operation != "batch" {
		t.Fatalf("response = %d operation=%q: %s", rec.Code, fake.operation, rec.Body)
	}
	if fake.lastBatchAction != domain.TagBatchConfirm || !reflect.DeepEqual(fake.lastIDs, []int64{3, 2, 9, 1, 3}) {
		t.Errorf("BatchTags(%q, %v)", fake.lastBatchAction, fake.lastIDs)
	}
	got := decode[gen.TagBatchResponse](t, rec)
	want := gen.TagBatchResponse{AppliedIds: []int64{3, 1}, NotFoundIds: []int64{9}, NotApplicableIds: []int64{2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("body = %+v, want %+v", got, want)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Error("batch response is cacheable")
	}
}

// 空の配列は null でなく [] で返す。
func TestBatchTagsReturnsEmptyArraysNotNull(t *testing.T) {
	fake := &fakeTags{batchOutcome: domain.TagBatchOutcome{AppliedIDs: []int64{1}}}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch", `{"action":"delete","ids":[1]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	for _, field := range []string{`"notFoundIds":[]`, `"notApplicableIds":[]`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("body = %s, want %s", rec.Body, field)
		}
	}
}

func TestBatchTagsAcceptsEveryAction(t *testing.T) {
	for _, action := range []domain.TagBatchAction{domain.TagBatchConfirm, domain.TagBatchReject, domain.TagBatchDelete} {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch",
			`{"action":"`+string(action)+`","ids":[1]}`)
		if rec.Code != http.StatusOK || fake.lastBatchAction != action {
			t.Errorf("%s: response = %d action=%q: %s", action, rec.Code, fake.lastBatchAction, rec.Body)
		}
	}
}

func TestBatchTagsRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
		want wantError
	}{
		{"ids が空", `{"action":"confirm","ids":[]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyTags, limit: domain.MaxTagBatch}},
		{"ids が 20001 件", `{"action":"confirm","ids":` + idsJSON(domain.MaxTagBatch+1) + `}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyTags, limit: domain.MaxTagBatch}},
		{"action が 3 値以外", `{"action":"merge","ids":[1]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
		{"action が無い", `{"ids":[1]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
	}
	for _, tc := range cases {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch", tc.body)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
		if fake.operation != "" {
			t.Errorf("%s: 保存層に届いた (%s)", tc.name, fake.operation)
		}
	}
}

// 20000 件ちょうどは受け付ける。
func TestBatchTagsAcceptsMaxIDs(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch",
		`{"action":"delete","ids":`+idsJSON(domain.MaxTagBatch)+`}`)
	if rec.Code != http.StatusOK || len(fake.lastIDs) != domain.MaxTagBatch {
		t.Fatalf("response = %d ids=%d: %s", rec.Code, len(fake.lastIDs), rec.Body)
	}
}

func TestBatchTagsStoreFailureIsInternalError(t *testing.T) {
	fake := &fakeTags{err: errors.New("disk full")}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/batch", `{"action":"reject","ids":[1]}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
}

func TestTagImpactReturnsCountsForAction(t *testing.T) {
	for _, action := range []domain.TagImpactAction{domain.TagImpactReject, domain.TagImpactDelete, domain.TagImpactMerge} {
		fake := &fakeTags{impact: domain.TagImpact{TagCount: 2, VideoCount: 101}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/impact",
			`{"action":"`+string(action)+`","ids":[4,5]}`)
		if rec.Code != http.StatusOK || fake.operation != "impact" {
			t.Fatalf("%s: response = %d operation=%q: %s", action, rec.Code, fake.operation, rec.Body)
		}
		if fake.lastImpactAction != action || !reflect.DeepEqual(fake.lastIDs, []int64{4, 5}) {
			t.Errorf("%s: TagImpact(%q, %v)", action, fake.lastImpactAction, fake.lastIDs)
		}
		got := decode[gen.TagImpactResponse](t, rec)
		if got != (gen.TagImpactResponse{TagCount: 2, VideoCount: 101}) {
			t.Errorf("%s: body = %+v", action, got)
		}
	}
}

func TestTagImpactRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
		want wantError
	}{
		{"action が 3 値以外", `{"action":"confirm","ids":[1]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
		{"ids が空", `{"action":"delete","ids":[]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyTags, limit: domain.MaxTagBatch}},
		{"ids が 20001 件", `{"action":"merge","ids":` + idsJSON(domain.MaxTagBatch+1) + `}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyTags, limit: domain.MaxTagBatch}},
	}
	for _, tc := range cases {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/impact", tc.body)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
		if fake.operation != "" {
			t.Errorf("%s: 保存層に届いた (%s)", tc.name, fake.operation)
		}
	}
}

// ゲストはどちらの経路も 401 で、保存層に届かない。
func TestTagBatchRoutesAreOwnerOnly(t *testing.T) {
	fake := &fakeTags{}
	env := newAuthEnv(t, t.TempDir(), Options{Tags: fake})
	env.setup()
	for _, target := range []string{"/api/tags/batch", "/api/tags/impact"} {
		rec := env.serve(authRequest{method: http.MethodPost, target: target, body: `{"action":"delete","ids":[1]}`})
		assertUnauthenticated(t, target, rec)
	}
	if fake.operation != "" {
		t.Errorf("ゲストの要求が保存層に届いた (%s)", fake.operation)
	}
}
