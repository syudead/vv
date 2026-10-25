package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// The route table is hand-written, so it must name each catalog operation's method, path and body as openapi.yaml does.
func TestAssistantRoutesMatchOpenAPI(t *testing.T) {
	declared := openAPIOperationIDs(t)
	withBody, _ := openAPIOperations(t)
	actions := map[string]bool{}
	current := map[string]bool{}
	for _, row := range domain.AssistantCatalog() {
		actions[row.Operation] = actions[row.Operation] || row.Action != ""
		current[row.Operation] = row.ReadsCurrent
	}

	for operation, route := range assistantRoutes {
		op, ok := declared[operation]
		if !ok {
			t.Errorf("%s は openapi.yaml に無い", operation)
			continue
		}
		if _, ok := actions[operation]; !ok {
			t.Errorf("%s の経路があるがカタログに行が無い", operation)
		}
		if route.method != op.method || route.path != op.path || route.body != slices.Contains(withBody, op) {
			t.Errorf("%s = %s %s (本文 %v), openapi.yaml は %s %s (本文 %v)", operation,
				route.method, route.path, route.body, op.method, op.path, slices.Contains(withBody, op))
		}
		if (route.actionField != "" || len(route.actionKeys) > 0) != actions[operation] {
			t.Errorf("%s: actionField %q とカタログの変種の有無 %v が合わない", operation, route.actionField, actions[operation])
		}
		if (len(route.current) > 0) != current[operation] {
			t.Errorf("%s: 現在値のフィールド %v とカタログの ReadsCurrent %v が合わない", operation, route.current, current[operation])
		}
	}
	for operation := range actions {
		if _, ok := assistantRoutes[operation]; !ok {
			t.Errorf("カタログの %s に経路が無い", operation)
		}
	}
}

var openAPIOperationID = regexp.MustCompile(`^      operationId: (\S+)$`)

func openAPIOperationIDs(t *testing.T) map[string]operation {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("api/openapi.yaml を読めない: %v", err)
	}
	ids := map[string]operation{}
	var current operation
	for _, line := range strings.Split(string(raw), "\n") {
		if m := openAPIPath.FindStringSubmatch(line); m != nil {
			current.path = m[1]
		} else if m := openAPIMethod.FindStringSubmatch(line); m != nil {
			current.method = openAPIMethodSet[m[1]]
		} else if m := openAPIOperationID.FindStringSubmatch(line); m != nil {
			ids[m[1]] = current
		}
	}
	return ids
}

type recordedCall struct {
	method, target, contentType string
	body                        map[string]any
	audience                    domain.Audience
}

// stubAPI records each request and answers from respond, standing in for the generated mux.
type stubAPI struct {
	calls   []recordedCall
	respond func(r *http.Request) (int, string)
}

func (s *stubAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	call := recordedCall{
		method: r.Method, target: r.URL.RequestURI(), contentType: r.Header.Get("Content-Type"),
		audience: audienceFrom(r.Context()),
	}
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &call.body)
	}
	s.calls = append(s.calls, call)
	status, body := http.StatusOK, `{}`
	if s.respond != nil {
		status, body = s.respond(r)
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestActionRunnerSendsTheRowsRequestAsOwner(t *testing.T) {
	cases := []struct {
		name string
		call domain.AssistantCall
		want recordedCall
	}{
		{
			name: "updateVideoTags.remove は remove を送る",
			call: domain.AssistantCall{ActionID: "updateVideoTags.remove", Body: map[string]any{
				"videoIds": []any{1.0, 2.0}, "tag": map[string]any{"id": 7.0},
			}},
			want: recordedCall{method: http.MethodPost, target: "/api/video-tags", body: map[string]any{
				"videoIds": []any{1.0, 2.0}, "tag": map[string]any{"id": 7.0}, "action": "remove",
			}},
		},
		{
			name: "batchTags.reject は呼び出し側の action より行の reject を優先する",
			call: domain.AssistantCall{ActionID: "batchTags.reject", Body: map[string]any{
				"ids": []any{3.0}, "action": "confirm",
			}},
			want: recordedCall{method: http.MethodPost, target: "/api/tags/batch", body: map[string]any{
				"ids": []any{3.0}, "action": "reject",
			}},
		},
		{
			name: "パスとクエリを埋め、本文の無い操作には本文を送らない",
			call: domain.AssistantCall{
				ActionID: "removeTagSynonym", Path: map[string]string{"id": "5"}, Query: map[string]string{"name": "a b"},
			},
			want: recordedCall{method: http.MethodDelete, target: "/api/tags/5/synonyms?name=a+b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &stubAPI{}
			runner := &ActionRunner{api: api}
			if _, err := runner.Run(context.Background(), tc.call); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(api.calls) != 1 {
				t.Fatalf("ハンドラーの呼び出し %d 回, want 1", len(api.calls))
			}
			got := api.calls[0]
			tc.want.audience = domain.AudienceOwner
			if tc.want.body != nil {
				tc.want.contentType = "application/json"
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("呼び出し = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestActionRunnerSendsSettingsBackWithOneFieldChanged(t *testing.T) {
	api := &stubAPI{respond: func(r *http.Request) (int, string) {
		if r.Method == http.MethodGet {
			return http.StatusOK, `{"enabled":true,"afterImport":false,"threshold":0.5,"tentativeThreshold":0.3,"queue":{"pending":2}}`
		}
		return http.StatusOK, `{}`
	}}
	runner := &ActionRunner{api: api}
	call := domain.AssistantCall{ActionID: "updateAutoTaggingSettings", Body: map[string]any{"threshold": 0.8}}
	if _, err := runner.Run(context.Background(), call); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(api.calls) != 2 || api.calls[0].method != http.MethodGet || api.calls[1].method != http.MethodPut {
		t.Fatalf("呼び出し = %+v, want GET のあと PUT", api.calls)
	}
	want := map[string]any{"enabled": true, "afterImport": false, "threshold": 0.8, "tentativeThreshold": 0.3}
	if !reflect.DeepEqual(api.calls[1].body, want) {
		t.Errorf("PUT の本文 = %v, want %v", api.calls[1].body, want)
	}
}

func TestActionRunnerMapsErrorResponses(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		want       domain.AssistantCallError
	}{
		{"404", `{"code":"tag_not_found","message":"Tag not found."}`, http.StatusNotFound,
			domain.AssistantCallError{Status: http.StatusNotFound, Code: "tag_not_found", Message: "Tag not found."}},
		{"400", `{"code":"invalid_request","reason":"merge_same_tag","message":"same"}`, http.StatusBadRequest,
			domain.AssistantCallError{Status: http.StatusBadRequest, Code: "invalid_request", Reason: "merge_same_tag", Message: "same"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &ActionRunner{api: &stubAPI{respond: func(*http.Request) (int, string) { return tc.status, tc.body }}}
			_, err := runner.Run(context.Background(), domain.AssistantCall{ActionID: "deleteTag", Path: map[string]string{"id": "1"}})
			var failure *domain.AssistantCallError
			if !errors.As(err, &failure) {
				t.Fatalf("err = %v, want *domain.AssistantCallError", err)
			}
			if *failure != tc.want {
				t.Errorf("失敗 = %+v, want %+v", *failure, tc.want)
			}
		})
	}
}

func TestActionRunnerRefusesActionsOutsideTheCatalog(t *testing.T) {
	for _, id := range []string{"deleteVideo", "batchTags", ""} {
		api := &stubAPI{}
		runner := &ActionRunner{api: api}
		_, err := runner.Run(context.Background(), domain.AssistantCall{ActionID: id, Path: map[string]string{"id": "1"}})
		if !errors.Is(err, domain.ErrAssistantActionUnknown) {
			t.Errorf("%q: err = %v, want ErrAssistantActionUnknown", id, err)
		}
		if len(api.calls) != 0 {
			t.Errorf("%q: 拒否する前にハンドラーを %d 回呼んだ", id, len(api.calls))
		}
	}
}

func TestActionRunnerRefusesAnEditOfTheOtherAction(t *testing.T) {
	api := &stubAPI{}
	runner := &ActionRunner{api: api}
	_, err := runner.Run(context.Background(), domain.AssistantCall{ActionID: "applyVideoEdits.add", Body: map[string]any{
		"videoIds": []any{1.0}, "credits": []any{map[string]any{"kind": "person", "role": "cast", "remove": []any{2.0}}},
	}})
	if err == nil || len(api.calls) != 0 {
		t.Errorf("err = %v, 呼び出し %d 回, want 呼ぶ前の拒否", err, len(api.calls))
	}
}

// NewServer's runner reaches the real handlers without a session cookie or Origin.
func TestNewServerRunnerCallsTheScreenHandlers(t *testing.T) {
	tags := &fakeTags{batchOutcome: domain.TagBatchOutcome{AppliedIDs: []int64{3}}}
	_, runner := NewServer(Options{Tags: tags, Auth: ownerAuth{}, Assets: emptyAssets{}})

	result, err := runner.Run(context.Background(), domain.AssistantCall{
		ActionID: "batchTags.reject", Body: map[string]any{"ids": []int64{3}},
	})
	if err != nil {
		t.Fatalf("batchTags.reject: %v", err)
	}
	if result.Status != http.StatusOK || tags.lastBatchAction != domain.TagBatchReject || !slices.Equal(tags.lastIDs, []int64{3}) {
		t.Errorf("状態 %d, 操作 %s %v, want 200 と reject [3]", result.Status, tags.lastBatchAction, tags.lastIDs)
	}

	_, err = runner.Run(context.Background(), domain.AssistantCall{
		ActionID: "mergeTag", Path: map[string]string{"id": "4"}, Body: map[string]any{"sourceIds": []int64{4}},
	})
	var failure *domain.AssistantCallError
	if !errors.As(err, &failure) || failure.Status != http.StatusBadRequest || failure.Reason != "merge_same_tag" {
		t.Errorf("mergeTag の自己統合: err = %v, want 400 merge_same_tag", err)
	}
}
