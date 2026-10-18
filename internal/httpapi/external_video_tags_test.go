package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

type videoTagsCall struct {
	refs      []domain.VideoRef
	action    domain.VideoTagsAction
	names     []string
	tentative bool
}

type externalVideoTagsStore struct {
	Tags
	calls   []videoTagsCall
	outcome domain.VideoTagsOutcome
	err     error
}

func (s *externalVideoTagsStore) ApplyVideoTags(_ context.Context, refs []domain.VideoRef, action domain.VideoTagsAction, names []string, tentative bool) (domain.VideoTagsOutcome, error) {
	s.calls = append(s.calls, videoTagsCall{refs, action, names, tentative})
	return s.outcome, s.err
}

func externalVideoTagsHandler(store *externalVideoTagsStore) *externalServer {
	return &externalServer{s: &server{tags: store, logger: discardLogger()}}
}

func postExternalVideoTags(t *testing.T, e *externalServer, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/video-tags", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.UpdateVideoTags(w, r)
	if got := w.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	return w
}

func TestExternalVideoTagsForwardsRequestAndMapsResponse(t *testing.T) {
	for _, tc := range []struct {
		action        domain.VideoTagsAction
		tentative     string
		wantTentative bool
	}{
		{domain.VideoTagsAdd, "", false}, {domain.VideoTagsRemove, `,"tentative":false`, false}, {domain.VideoTagsReplace, `,"tentative":true`, true},
	} {
		t.Run(string(tc.action), func(t *testing.T) {
			store := &externalVideoTagsStore{outcome: domain.VideoTagsOutcome{
				Items: []domain.VideoTagsResult{
					{VideoID: 7, ContentKey: "key-a", Tags: []domain.VideoTag{{TagRef: domain.TagRef{ID: 5, Name: "cat", Tentative: true}, Manual: true, FromFolder: true}}},
					{VideoID: 9, ContentKey: "key-b"},
				},
				SkippedNames: []string{"rejected"},
			}}
			body := fmt.Sprintf(`{"videos":[{"id":7},{"contentKey":"key-b"},{"path":"/media/c.mp4"}],"action":%q,"tags":[" cat ","cat"]%s}`, tc.action, tc.tentative)
			w := postExternalVideoTags(t, externalVideoTagsHandler(store), body)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body)
			}
			wantCall := videoTagsCall{[]domain.VideoRef{{ID: 7}, {ContentKey: "key-b"}, {Path: "/media/c.mp4"}}, tc.action, []string{" cat ", "cat"}, tc.wantTentative}
			if !reflect.DeepEqual(store.calls, []videoTagsCall{wantCall}) {
				t.Fatalf("calls = %+v, want %+v", store.calls, wantCall)
			}
			var got extgen.VideoTagsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			want := extgen.VideoTagsResponse{Items: []extgen.VideoTagsItem{
				{Video: extgen.VideoTagsVideo{Id: 7, ContentKey: "key-a"}, Tags: []extgen.ExternalVideoTag{{Id: 5, Name: "cat", Tentative: true, Manual: true, FromFolder: true}}},
				{Video: extgen.VideoTagsVideo{Id: 9, ContentKey: "key-b"}, Tags: []extgen.ExternalVideoTag{}},
			}, SkippedTags: []string{"rejected"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response = %+v, want %+v", got, want)
			}
		})
	}
}

func TestExternalVideoTagsEmptyReplacement(t *testing.T) {
	store := &externalVideoTagsStore{}
	w := postExternalVideoTags(t, externalVideoTagsHandler(store), `{"videos":[{"id":1}],"action":"replace","tags":[]}`)
	if w.Code != http.StatusOK || len(store.calls) != 1 || store.calls[0].action != domain.VideoTagsReplace || store.calls[0].names == nil || len(store.calls[0].names) != 0 {
		t.Fatalf("status=%d calls=%+v: %s", w.Code, store.calls, w.Body)
	}
	var got extgen.VideoTagsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Items == nil || len(got.Items) != 0 || got.SkippedTags == nil || len(got.SkippedTags) != 0 {
		t.Fatalf("empty response = %#v", got)
	}
}

func TestExternalVideoTagsRejectsBadRequests(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		reason     extgen.ErrorReason
		index      *int
		limit      int
	}{
		{name: "invalid action", body: `{"videos":[{"id":1}],"action":"toggle","tags":["cat"]}`},
		{name: "unknown field", body: `{"videos":[{"id":1}],"action":"add","tags":["cat"],"extra":1}`},
		{name: "missing tags", body: `{"videos":[{"id":1}],"action":"replace"}`},
		{name: "null tags", body: `{"videos":[{"id":1}],"action":"replace","tags":null}`},
		{name: "multiple selectors", body: `{"videos":[{"id":1},{"id":2,"contentKey":"a"}],"action":"add","tags":["cat"]}`, index: new(1)},
		{name: "no selector", body: `{"videos":[{}],"action":"add","tags":["cat"]}`, index: new(0)},
		{name: "empty path", body: `{"videos":[{"path":""}],"action":"add","tags":["cat"]}`, index: new(0)},
		{name: "empty content key", body: `{"videos":[{"contentKey":""}],"action":"add","tags":["cat"]}`, index: new(0)},
		{name: "no videos", body: `{"videos":[],"action":"add","tags":["cat"]}`, reason: extgen.TooManyVideos, limit: maxVideoTagsIDs},
		{name: "empty add", body: `{"videos":[{"id":1}],"action":"add","tags":[]}`, reason: extgen.TooManyTags, limit: domain.ExternalVideoTagsMaxNames},
		{name: "empty remove", body: `{"videos":[{"id":1}],"action":"remove","tags":[]}`, reason: extgen.TooManyTags, limit: domain.ExternalVideoTagsMaxNames},
		{name: "trailing JSON", body: `{"videos":[{"id":1}],"action":"add","tags":["cat"]}{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &externalVideoTagsStore{}
			w := postExternalVideoTags(t, externalVideoTagsHandler(store), tc.body)
			checkExternalVideoTagsError(t, w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, tc.reason, tc.index, tc.limit)
			if len(store.calls) != 0 {
				t.Fatalf("rejected request called storage: %+v", store.calls)
			}
		})
	}
}

func TestExternalVideoTagsRejectsNonBooleanTentative(t *testing.T) {
	for _, value := range []string{`"true"`, `1`, `[]`, `null`} {
		store := &externalVideoTagsStore{}
		body := `{"videos":[{"id":1}],"action":"add","tags":["cat"],"tentative":` + value + `}`
		w := postExternalVideoTags(t, externalVideoTagsHandler(store), body)
		checkExternalVideoTagsError(t, w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", nil, 0)
		if len(store.calls) != 0 {
			t.Fatalf("tentative=%s called storage", value)
		}
	}
}

func TestExternalVideoTagsCountLimits(t *testing.T) {
	// These are the API's actual count boundaries, not database or load fixtures.
	for _, tc := range []struct {
		name         string
		videos, tags int
		reason       extgen.ErrorReason
		limit        int
	}{
		{"video limit", maxVideoTagsIDs, 1, "", 0},
		{"too many videos", maxVideoTagsIDs + 1, 1, extgen.TooManyVideos, maxVideoTagsIDs},
		{"tag limit", 1, domain.ExternalVideoTagsMaxNames, "", 0},
		{"too many tags", 1, domain.ExternalVideoTagsMaxNames + 1, extgen.TooManyTags, domain.ExternalVideoTagsMaxNames},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &externalVideoTagsStore{}
			videos := strings.TrimSuffix(strings.Repeat(`{"id":1},`, tc.videos), ",")
			tags := strings.TrimSuffix(strings.Repeat(`"cat",`, tc.tags), ",")
			w := postExternalVideoTags(t, externalVideoTagsHandler(store), `{"videos":[`+videos+`],"action":"add","tags":[`+tags+`]}`)
			if tc.reason != "" {
				checkExternalVideoTagsError(t, w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, tc.reason, nil, tc.limit)
				if len(store.calls) != 0 {
					t.Fatal("over-limit request called storage")
				}
			} else if w.Code != http.StatusOK || len(store.calls) != 1 || len(store.calls[0].refs) != tc.videos || len(store.calls[0].names) != tc.tags {
				t.Fatalf("status=%d calls=%d: %s", w.Code, len(store.calls), w.Body)
			}
		})
	}
}

func TestExternalVideoTagsMapsStorageErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   extgen.ErrorCode
		reason extgen.ErrorReason
		index  *int
		limit  int
	}{
		{"missing video", &domain.VideoRefNotFoundError{Index: 1}, http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.VideoNotFound, new(1), 0},
		{"empty name", &domain.TagNameAtError{Index: 1, Err: &domain.InvalidTagNameError{Problem: domain.TagNameEmpty}}, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TagNameEmpty, new(1), 0},
		{"control characters", &domain.TagNameAtError{Index: 0, Err: &domain.InvalidTagNameError{Problem: domain.TagNameControlCharacters}}, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TagNameControlCharacters, new(0), 0},
		{"long name", &domain.TagNameAtError{Index: 2, Err: &domain.InvalidTagNameError{Problem: domain.TagNameTooLong}}, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TagNameTooLong, new(2), domain.TagNameMaxLength},
		{"storage failure", errors.New("private storage detail"), http.StatusInternalServerError, extgen.ErrorCodeInternal, "", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &externalVideoTagsStore{err: tc.err}
			w := postExternalVideoTags(t, externalVideoTagsHandler(store), `{"videos":[{"id":1}],"action":"add","tags":["cat"]}`)
			checkExternalVideoTagsError(t, w, tc.status, tc.code, tc.reason, tc.index, tc.limit)
			if len(store.calls) != 1 {
				t.Fatalf("storage calls = %d", len(store.calls))
			}
			if strings.Contains(w.Body.String(), "private storage detail") {
				t.Fatal("storage detail leaked")
			}
		})
	}
}

func checkExternalVideoTagsError(t *testing.T, w *httptest.ResponseRecorder, status int, code extgen.ErrorCode, reason extgen.ErrorReason, index *int, limit int) {
	t.Helper()
	var got extgen.Error
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var wantReason *extgen.ErrorReason
	var wantLimit *int
	if reason != "" {
		wantReason = &reason
	}
	if limit != 0 {
		wantLimit = &limit
	}
	if w.Code != status || got.Code != code || !reflect.DeepEqual(got.Reason, wantReason) || !reflect.DeepEqual(got.Index, index) || !reflect.DeepEqual(got.Limit, wantLimit) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}

func TestExternalVideoTagsBodyLimit(t *testing.T) {
	if got := (&externalServer{}).bodyLimit(); got != 32<<20 {
		t.Fatalf("default body limit = %d", got)
	}
	const limit = 128
	body := `{"videos":[{"id":1}],"action":"add","tags":["cat"]}`
	for _, size := range []int{limit - 1, limit, limit + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			store := &externalVideoTagsStore{}
			e := externalVideoTagsHandler(store)
			e.maxBody = limit
			w := postExternalVideoTags(t, e, body+strings.Repeat(" ", size-len(body)))
			if size <= limit {
				if w.Code != http.StatusOK || len(store.calls) != 1 {
					t.Fatalf("status=%d calls=%d: %s", w.Code, len(store.calls), w.Body)
				}
			} else {
				checkExternalVideoTagsError(t, w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", nil, 0)
				if len(store.calls) != 0 || !strings.Contains(w.Body.String(), "at most 128 bytes") {
					t.Fatalf("calls=%d: %s", len(store.calls), w.Body)
				}
			}
		})
	}
	store := &externalVideoTagsStore{}
	e := externalVideoTagsHandler(store)
	e.maxBody = limit
	w := postExternalVideoTags(t, e, `{"videos":[{"path":"`+strings.Repeat("x", limit)+`"}],"action":"add","tags":["cat"]}`)
	checkExternalVideoTagsError(t, w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", nil, 0)
	if len(store.calls) != 0 || !strings.Contains(w.Body.String(), "at most 128 bytes") {
		t.Fatalf("calls=%d: %s", len(store.calls), w.Body)
	}
}

// Only the route's authentication boundary needs the real authentication fixture.
func TestExternalVideoTagsBoundary(t *testing.T) {
	store := &externalVideoTagsStore{}
	env, owner := newExternalEnv(t, Options{Tags: store})
	secret := env.createAPIToken(owner, "scraper").Secret
	body := `{"videos":[{"id":1}],"action":"add","tags":["cat"]}`
	assertBearerUnauthenticated(t, "no token", env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body}))
	assertBearerUnauthenticated(t, "cookie only", env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body, cookies: []*http.Cookie{owner}}))
	header := bearer(secret)
	header["Content-Type"] = "text/plain"
	w := env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body, header: header})
	if w.Code != http.StatusBadRequest || len(store.calls) != 0 {
		t.Fatalf("status=%d calls=%d: %s", w.Code, len(store.calls), w.Body)
	}
	header = bearer(secret)
	header["Origin"] = "https://scraper.example"
	w = env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body, header: header})
	if w.Code != http.StatusOK || len(store.calls) != 1 {
		t.Fatalf("status=%d calls=%d: %s", w.Code, len(store.calls), w.Body)
	}
}
