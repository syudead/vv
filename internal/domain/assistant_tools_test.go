package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestClassifyAssistantOperation(t *testing.T) {
	cases := []struct {
		name, operation, method, path string
		want                          AssistantToolKind
		row                           string
	}{
		{"一覧にない GET は読み取り", "listTags", "GET", "/api/tags", AssistantToolRead, ""},
		{"何も書かない POST は読み取り", "tagImpact", "POST", "/api/tags/{id}/impact", AssistantToolRead, ""},
		{"カタログの操作は書き込み", "renameTag", "PATCH", "/api/tags/{id}", AssistantToolWrite, "renameTag"},
		{"スキーマ行の操作は書き込み", "undoActivity", "POST", "/api/activity/{id}/undo", AssistantToolWrite, "undoActivity"},
		{"複合行の後始末も書き込み", "discardThumbnailEdit", "DELETE", "/api/thumbnail-edits/{editId}",
			AssistantToolWrite, "editVideoThumbnail"},
		{"除外の GET", "getPluginProxySettings", "GET", "/api/settings/plugin-proxy", AssistantToolExcluded, ""},
		{"除外の書き込み", "deleteVideo", "DELETE", "/api/videos/{id}", AssistantToolExcluded, ""},
		{"アシスタント自身", "listAssistantProposals", "GET", "/api/assistant/proposals", AssistantToolExcluded, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ClassifyAssistantOperation(tc.operation, tc.method, tc.path)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got.Kind != tc.want {
				t.Errorf("kind = %s, want %s", got.Kind, tc.want)
			}
			if (got.Row != nil) != (tc.row != "") || (got.Row != nil && got.Row.ID != tc.row) {
				t.Errorf("row = %+v, want %q", got.Row, tc.row)
			}
			if (got.Kind == AssistantToolExcluded) != (got.Exclusion != nil) {
				t.Errorf("exclusion = %+v for kind %s", got.Exclusion, got.Kind)
			}
		})
	}
}

func TestClassifyAssistantOperationRefusesUnlistedAndDoubleListedOperations(t *testing.T) {
	if _, err := ClassifyAssistantOperation("frobnicate", "POST", "/api/frobnicate"); !errors.Is(err, ErrAssistantUnclassified) {
		t.Errorf("一覧にない POST: err = %v, want ErrAssistantUnclassified", err)
	}

	original := assistantReadPosts
	t.Cleanup(func() { assistantReadPosts = original })
	assistantReadPosts = append(slices.Clone(original), "renameTag", "listTags")
	for _, op := range []struct{ id, method string }{{"renameTag", "PATCH"}, {"listTags", "GET"}} {
		if _, err := ClassifyAssistantOperation(op.id, op.method, "/api/tags"); !errors.Is(err, ErrAssistantClassifiedTwice) {
			t.Errorf("%s: err = %v, want ErrAssistantClassifiedTwice", op.id, err)
		}
	}
}

// Every exclusion answers a reason and a page the page table holds, except the assistant's own operations.
func TestAssistantExclusionsNameAReasonAndAPage(t *testing.T) {
	pages := map[string]bool{}
	for _, page := range AssistantPages() {
		pages[page.Route] = true
	}
	for _, exclusion := range AssistantExclusions() {
		if exclusion.Reason == "" || !pages[exclusion.Page] {
			t.Errorf("%s: 理由 %q, ページ %q がページ表に無い", exclusion.Operation, exclusion.Reason, exclusion.Page)
		}
	}
}
