package app

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestTurnFilterDropsAFieldWordTheModelPutAsATag(t *testing.T) {
	f := newTurnFixture(`{"kind":"filter","filter":{"q":"カラオケ","tags":["tags"]}}`, nil)
	result, err := f.a.Turn(context.Background(), domain.AssistantTurnInput{Text: "カラオケ関連の動画"})
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnFilter {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	if rule := result.Criteria.Rule; rule.Q != "カラオケ" || len(rule.Tags) != 0 || len(result.Criteria.Labels) != 0 {
		t.Fatalf("criteria = %+v", result.Criteria)
	}
}

func TestTurnFilterSearchesUnresolvedNamesAsText(t *testing.T) {
	f := newTurnFixture(`{"kind":"filter","filter":{"q":"live","tags":["cat","カラオケ"],"person":"Jane Doe"}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.a.Turn(context.Background(), domain.AssistantTurnInput{Text: "request"})
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnFilter {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	rule := result.Criteria.Rule
	if rule.Q != `live カラオケ "Jane Doe"` || !slices.Equal(rule.Tags, []int64{3}) || rule.Person != 0 {
		t.Fatalf("rule = %+v", rule)
	}
}

func TestSearchUnresolvedAppendsTheNameAsALiteralRequiredTerm(t *testing.T) {
	for _, tc := range []struct{ q, name, want string }{
		{"cat", "-live", `cat "-live"`},
		{"cat", "OR", `cat "OR"`},
		{"cat OR dog", "cat", "cat OR dog cat"},
		{"-cat", "cat", "-cat cat"},
		{"live cat", "Cat", "live cat"},
	} {
		got, ok := searchUnresolved(tc.q, domain.AssistantLabel{Name: tc.name})
		if got != tc.want || ok {
			t.Errorf("searchUnresolved(%q, %q) = %q, %v; want %q, false", tc.q, tc.name, got, ok, tc.want)
		}
	}
}

func TestTurnAsksWhenAWriteTargetFilterNamesAMissingTag(t *testing.T) {
	f := newTurnFixture(`{"kind":"action","action":{"operation":"updateVideoTags.add",`+
		`"target":{"kind":"filter","filter":{"tags":["カラオケ"]}},"arguments":{"tagNames":["cat"]}}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.a.Turn(context.Background(), selection())
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnClarify || !slices.Equal(result.Clarify.Names, []string{"カラオケ"}) {
		t.Fatalf("result = %+v", result)
	}
	if len(f.videos.queries) != 0 || f.store.created.ActionID != "" {
		t.Fatalf("queries = %+v, created = %+v", f.videos.queries, f.store.created)
	}
}
