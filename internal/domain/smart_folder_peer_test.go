package domain

import (
	"slices"
	"testing"
)

func TestSmartPathQueryJoinsTheRulesOfAPath(t *testing.T) {
	q, ok := SmartPathQuery([]SmartRule{
		{Q: " cat ", Tags: []int64{1, 2}, Person: 5},
		{Q: "cat", Tags: []int64{2, 3}, Person: 5, Favorite: true},
		{Q: "dog", TagKind: 7, Organization: 9},
	})
	if !ok {
		t.Fatal("SmartPathQuery() refused a path one query holds")
	}
	if !slices.Equal(q.TagIDs, []int64{1, 2, 3}) || q.Query != "cat dog" || q.Person != 5 || q.TagKindID != 7 ||
		q.Organization != 9 || !q.FavoriteOnly {
		t.Errorf("SmartPathQuery() = %+v", q)
	}
}

func TestSmartPathQueryRefusesAPathNoOneQueryHolds(t *testing.T) {
	sixteen := make([]int64, SmartRuleMaxTags)
	for i := range sixteen {
		sixteen[i] = int64(i + 1)
	}
	for name, rules := range map[string][]SmartRule{
		"two people":        {{Person: 1}, {Person: 2}},
		"two kinds":         {{TagKind: 1}, {TagKind: 2}},
		"two organizations": {{Organization: 1}, {Organization: 2}},
		"seventeen tags":    {{Tags: sixteen}, {Tags: []int64{99}}},
		// Each level parses its own search, so a join that drops or merges a term matches other videos.
		"seventeen search terms": {{Q: "a b c d e f g h i j k l m n o p"}, {Q: "z"}},
		"an OR into the child":   {{Q: "a OR"}, {Q: "z"}},
		"a quote into the child": {{Q: `a "b`}, {Q: `z"`}},
	} {
		if q, ok := SmartPathQuery(rules); ok {
			t.Errorf("%s: SmartPathQuery() = %+v, want refused", name, q)
		}
	}
}

func TestSmartRuleCountAddKeepsAnUnknownDuration(t *testing.T) {
	known := int64(5)
	sum := SmartRuleCount{VideoCount: 1, SizeBytes: 10}.Add(SmartRuleCount{VideoCount: 2, SizeBytes: 20, DurationMs: &known})
	if sum.VideoCount != 3 || sum.SizeBytes != 30 || sum.DurationMs == nil || *sum.DurationMs != 5 {
		t.Errorf("Add() = %+v, want 3 videos, 30 bytes and 5 ms", sum)
	}
	if none := (SmartRuleCount{}).Add(SmartRuleCount{}); none.DurationMs != nil {
		t.Errorf("Add() of unknown durations = %d, want nil", *none.DurationMs)
	}
}
