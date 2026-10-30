package app

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestResolveFilterDropsAFieldWordTheModelPutAsATag(t *testing.T) {
	f := newTurnFixture(`{"kind":"filter","filter":{"q":"カラオケ","tags":["tags"]}}`, nil)
	result, err := f.resolve(context.Background(), domain.AssistantTurnInput{Text: "カラオケ関連の動画"})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedFilter {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	if rule := result.Criteria.Rule; rule.Q != "カラオケ" || len(rule.Tags) != 0 || len(result.Criteria.Labels) != 0 {
		t.Fatalf("criteria = %+v", result.Criteria)
	}
}

func TestResolveFilterSearchesUnresolvedNamesAsText(t *testing.T) {
	f := newTurnFixture(`{"kind":"filter","filter":{"q":"live","tags":["cat","カラオケ"],"person":"Jane Doe"}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.resolve(context.Background(), domain.AssistantTurnInput{Text: "request"})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedFilter {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	rule := result.Criteria.Rule
	if rule.Q != `live カラオケ "Jane Doe"` || !slices.Equal(rule.Tags, []int64{3}) || rule.Person != 0 {
		t.Fatalf("rule = %+v", rule)
	}
}

// A model that fills every member writes 0 for the bound the request does not set, which must not fail the
// request as a bound out of order.
func TestResolveFilterDropsAZeroDurationBound(t *testing.T) {
	f := newTurnFixture(`{"kind":"filter","filter":{"q":"","tags":[],"minDurationMs":1800000,"maxDurationMs":0,`+
		`"addedFrom":"","addedTo":""}}`, nil)
	result, err := f.resolve(context.Background(), domain.AssistantTurnInput{Text: "30分以上のライブ"})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedFilter {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	rule := result.Criteria.Rule
	if rule.MinDurationMs == nil || *rule.MinDurationMs != 1800000 || rule.MaxDurationMs != nil {
		t.Fatalf("rule = %+v", rule)
	}
}

// A model that fills every member writes "0000-00-00" for a day the request does not set; a read drops it, a
// change's target keeps it for the validation to ask.
func TestResolveFilterDropsADayThatIsNotADateOnlyWhenReading(t *testing.T) {
	filter := `{"tags":["cat"],"addedFrom":"0000-00-00","addedTo":"0000-00-00","watchState":"unwatched"}`
	tags := map[string][]domain.AssistantCandidate{"cat": {catTag}}
	f := newTurnFixture(`{"kind":"filter","filter":`+filter+`}`, tags)
	result, err := f.resolve(context.Background(), domain.AssistantTurnInput{Text: "request"})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedFilter || result.Criteria.Rule.AddedFrom != "" || result.Criteria.Rule.AddedTo != "" {
		t.Fatalf("result = %+v, clarify = %+v", result, result.Clarify)
	}
	f = newTurnFixture(`{"kind":"action","action":{"operation":"updateVideoTags.add","target":{"kind":"filter","filter":`+
		filter+`},"arguments":{"tagNames":["cat"]}}}`, tags)
	result, err = f.resolve(context.Background(), selection())
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedClarify || len(f.videos.queries) != 0 {
		t.Fatalf("result = %+v, queries = %+v", result, f.videos.queries)
	}
}

// A change's target keeps an explicit 0 bound, so a reversed range asks instead of widening the target.
func TestResolveWriteTargetKeepsAZeroDurationBound(t *testing.T) {
	f := newTurnFixture(`{"kind":"action","action":{"operation":"updateVideoTags.add","target":{"kind":"filter",`+
		`"filter":{"minDurationMs":1800000,"maxDurationMs":0}},"arguments":{"tagNames":["cat"]}}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.resolve(context.Background(), selection())
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedClarify || len(f.videos.queries) != 0 {
		t.Fatalf("result = %+v, queries = %+v", result, f.videos.queries)
	}
}

// The selection limit applies to the narrowed selection, not to the library-wide match of the narrowing filter.
func TestResolveFilterOnTheSelectionIgnoresTheLibraryWideTotal(t *testing.T) {
	f := newTurnFixture(`{"kind":"action","action":{"operation":"updateVideoTags.add",`+
		`"target":{"kind":"selection","filter":{"maxDurationMs":600000}},"arguments":{"tagNames":["cat"]}}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	f.videos.items, f.videos.total = []domain.Video{f.videos.videos[8]}, domain.AssistantSelectionMax+1
	result, err := f.resolve(context.Background(), selection(7, 8))
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	if keys := f.store.created.Targets.VideoKeys; !slices.Equal(keys, []string{"key-8"}) {
		t.Fatalf("targets = %v", keys)
	}
}

// A follow-up such as "leave out the long ones" keeps the selection and narrows it by a filter.
func TestResolveFilterOnTheSelectionNarrowsIt(t *testing.T) {
	f := newTurnFixture(`{"kind":"action","action":{"operation":"updateVideoTags.add",`+
		`"target":{"kind":"selection","filter":{"maxDurationMs":600000}},"arguments":{"tagNames":["cat"]}}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	f.videos.items = []domain.Video{f.videos.videos[8], {ID: 9, UserKey: "key-9"}}
	result, err := f.resolve(context.Background(), selection(7, 8))
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	if keys := f.store.created.Targets.VideoKeys; !slices.Equal(keys, []string{"key-8"}) {
		t.Fatalf("targets = %v, want the selected video the filter matches", keys)
	}
	if q := f.videos.queries; len(q) != 1 || q[0].MaxDurationMs == nil || *q[0].MaxDurationMs != 600000 {
		t.Fatalf("queries = %+v", q)
	}
}

// Llama 3.3 70B answers "長いのは除いて" after a proposal on the selection with a filter target alone; the follow-up
// keeps the selection instead of widening the change to the whole library.
func TestResolveFollowUpFilterKeepsTheSelection(t *testing.T) {
	f := newTurnFixture(`{"kind":"action","action":{"operation":"updateVideoTags.add",`+
		`"target":{"kind":"filter","filter":{"maxDurationMs":600000}},"arguments":{"tagNames":["cat"]}}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	f.videos.items = []domain.Video{f.videos.videos[8], {ID: 9, UserKey: "key-9"}}
	in := selection(7, 8)
	in.PreviousTurnID = 5
	result, err := f.resolve(context.Background(), in)
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	if keys := f.store.created.Targets.VideoKeys; !slices.Equal(keys, []string{"key-8"}) {
		t.Fatalf("targets = %v, want the selected video the filter matches", keys)
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

func TestResolveAsksWhenAWriteTargetFilterNamesAMissingTag(t *testing.T) {
	f := newTurnFixture(`{"kind":"action","action":{"operation":"updateVideoTags.add",`+
		`"target":{"kind":"filter","filter":{"tags":["カラオケ"]}},"arguments":{"tagNames":["cat"]}}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.resolve(context.Background(), selection())
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedClarify || !slices.Equal(result.Clarify.Names, []string{"カラオケ"}) {
		t.Fatalf("result = %+v", result)
	}
	if len(f.videos.queries) != 0 || f.store.created.ActionID != "" {
		t.Fatalf("queries = %+v, created = %+v", f.videos.queries, f.store.created)
	}
}
