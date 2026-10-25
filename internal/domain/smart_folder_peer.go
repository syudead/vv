package domain

import (
	"slices"
	"strings"
)

// SmartPathQuery joins a smart folder's path rules into one query of this node's ids, which the paired node
// matches by name. The tags of every rule add up and the search inputs are joined by JoinSearches; favorite is
// set when any rule has it. ok is false when no one query holds the rules: more than SmartRuleMaxTags tags, two
// rules naming different kinds, people or organizations, or searches whose join changes what they match.
func SmartPathQuery(rules []SmartRule) (q VideoQuery, ok bool) {
	var queries []string
	for _, rule := range rules {
		for _, id := range rule.Tags {
			if !slices.Contains(q.TagIDs, id) {
				q.TagIDs = append(q.TagIDs, id)
			}
		}
		for _, single := range []struct {
			value int64
			out   *int64
		}{{rule.TagKind, &q.TagKindID}, {rule.Person, &q.Person}, {rule.Organization, &q.Organization}} {
			if single.value == 0 {
				continue
			}
			if *single.out != 0 && *single.out != single.value {
				return VideoQuery{}, false
			}
			*single.out = single.value
		}
		if text := strings.TrimSpace(rule.Q); text != "" && !slices.Contains(queries, text) {
			queries = append(queries, text)
		}
		q.FavoriteOnly = q.FavoriteOnly || rule.Favorite
	}
	joined, joins := JoinSearches(queries...)
	if !joins || len(q.TagIDs) > SmartRuleMaxTags {
		return VideoQuery{}, false
	}
	q.Query = joined
	return q, true
}

// JoinSearches joins search inputs by spaces into one that must match every input. ok is false when the joined
// input does not parse as the inputs' clauses together: more than MaxSearchTerms terms, or an OR or a quote of
// one input reaching into the next.
func JoinSearches(inputs ...string) (joined string, ok bool) {
	var parts []string
	var want []SearchClause
	for _, input := range inputs {
		if text := strings.TrimSpace(input); text != "" {
			parts = append(parts, text)
			want = append(want, ParseSearchQuery(text).Clauses...)
		}
	}
	joined = strings.Join(parts, " ")
	got := ParseSearchQuery(joined).Clauses
	return joined, slices.EqualFunc(got, want, func(a, b SearchClause) bool { return slices.Equal(a.Terms, b.Terms) })
}

// SmartRuleCount is what a smart folder card adds from the paired node's videos matching its rule.
type SmartRuleCount struct {
	VideoCount int
	// DurationMs is nil when no duration is known.
	DurationMs *int64
	SizeBytes  int64
}

// Add sums c and other, keeping DurationMs nil only when neither knows a duration.
func (c SmartRuleCount) Add(other SmartRuleCount) SmartRuleCount {
	out := SmartRuleCount{VideoCount: c.VideoCount + other.VideoCount, SizeBytes: c.SizeBytes + other.SizeBytes}
	if c.DurationMs != nil || other.DurationMs != nil {
		total := int64(0)
		for _, duration := range []*int64{c.DurationMs, other.DurationMs} {
			if duration != nil {
				total += *duration
			}
		}
		out.DurationMs = &total
	}
	return out
}
