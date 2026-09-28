package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// wantError は、エラーの応答に期待する HTTP 状態・code・reason・limit・tagName
// である（specs/023-english-i18n/contracts/error-api.md §1）。reason・tagName が
// 空、limit が 0 のときは、その項目が応答に無いことを期待する。
type wantError struct {
	status  int
	code    gen.ErrorCode
	reason  gen.ErrorReason
	limit   int
	tagName string
}

// assertErrorBody は、エラーの応答が want のとおりで、message が空でないことを
// 確かめる。
func assertErrorBody(t *testing.T, label string, status int, body []byte, want wantError) {
	t.Helper()
	if status != want.status {
		t.Errorf("%s: status = %d, want %d: %s", label, status, want.status, body)
		return
	}
	var got gen.Error
	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("%s: 本文を読めません: %v: %s", label, err, body)
		return
	}
	if got.Code != want.code {
		t.Errorf("%s: code = %q, want %q", label, got.Code, want.code)
	}
	if got.Message == "" {
		t.Errorf("%s: message が空", label)
	}
	switch {
	case want.reason == "" && got.Reason != nil:
		t.Errorf("%s: reason = %q, want なし", label, *got.Reason)
	case want.reason != "" && (got.Reason == nil || *got.Reason != want.reason):
		t.Errorf("%s: reason = %v, want %q", label, got.Reason, want.reason)
	}
	switch {
	case want.limit == 0 && got.Limit != nil:
		t.Errorf("%s: limit = %d, want なし", label, *got.Limit)
	case want.limit != 0 && (got.Limit == nil || *got.Limit != want.limit):
		t.Errorf("%s: limit = %v, want %d", label, got.Limit, want.limit)
	}
	switch {
	case want.tagName == "" && got.TagName != nil:
		t.Errorf("%s: tagName = %q, want なし", label, *got.TagName)
	case want.tagName != "" && (got.TagName == nil || *got.TagName != want.tagName):
		t.Errorf("%s: tagName = %v, want %q", label, got.TagName, want.tagName)
	}
}
