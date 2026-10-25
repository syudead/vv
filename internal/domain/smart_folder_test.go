package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseSmartRuleAcceptsEveryMember(t *testing.T) {
	raw := `{"q":"cat","favorite":true,"tags":[3,1],"tagKind":4,"person":5,"organization":6}`
	rule, err := ParseSmartRule([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSmartRule() error = %v", err)
	}
	want := SmartRule{
		Q: "cat", Favorite: true, Tags: []int64{3, 1}, TagKind: 4, Person: 5, Organization: 6,
	}
	if !reflect.DeepEqual(rule, want) {
		t.Fatalf("rule = %+v, want %+v", rule, want)
	}
	encoded, err := rule.Encode()
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseSmartRule([]byte(encoded))
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("round trip = %+v, %v", again, err)
	}
	query := rule.Query()
	if query.Query != "cat" || !query.FavoriteOnly || !reflect.DeepEqual(query.TagIDs, []int64{3, 1}) ||
		query.TagKindID != 4 || query.Person != 5 || query.Organization != 6 {
		t.Fatalf("Query() = %+v", query)
	}
	wantRefs := []SmartRuleRef{
		{SmartRefTag, 3}, {SmartRefTag, 1}, {SmartRefTagKind, 4}, {SmartRefPerson, 5}, {SmartRefOrganization, 6},
	}
	if refs := rule.Refs(); !reflect.DeepEqual(refs, wantRefs) {
		t.Fatalf("Refs() = %+v, want %+v", refs, wantRefs)
	}
}

func TestEmptySmartRuleMatchesEverything(t *testing.T) {
	rule, err := ParseSmartRule([]byte(` {} `))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rule.Query(), VideoQuery{}) || len(rule.Refs()) != 0 {
		t.Fatalf("Query() = %+v, Refs() = %+v", rule.Query(), rule.Refs())
	}
	if encoded, err := rule.Encode(); err != nil || encoded != "{}" {
		t.Fatalf("Encode() = %q, %v", encoded, err)
	}
}

func TestParseSmartRuleAcceptsVideoConditions(t *testing.T) {
	raw := `{"minDurationMs":600000,"maxDurationMs":1800000,"addedFrom":"2025-01-01","addedTo":"2025-12-31","watchState":"inProgress"}`
	rule, err := ParseSmartRule([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSmartRule() error = %v", err)
	}
	want := VideoConditions{
		MinDurationMs: 600000, MaxDurationMs: 1800000, AddedFrom: "2025-01-01", AddedTo: "2025-12-31",
		WatchState: WatchStateInProgress,
	}
	if rule.VideoConditions != want || rule.Query().VideoConditions != want {
		t.Fatalf("conditions = %+v, query = %+v, want %+v", rule.VideoConditions, rule.Query().VideoConditions, want)
	}
	if encoded, err := rule.Encode(); err != nil || encoded != raw {
		t.Fatalf("Encode() = %s, %v", encoded, err)
	}
	// One day is a valid range, as is a lone bound.
	for _, raw := range []string{`{"addedFrom":"2025-03-01","addedTo":"2025-03-01"}`, `{"maxDurationMs":60000}`} {
		if _, err := ParseSmartRule([]byte(raw)); err != nil {
			t.Errorf("ParseSmartRule(%s) error = %v", raw, err)
		}
	}
}

func TestParseSmartRuleRejectsOutOfRangeMembers(t *testing.T) {
	sixteen := "[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16"
	cases := map[string]string{
		"not an object":     `[]`,
		"null":              `null`,
		"empty":             ``,
		"unknown member":    `{"rating":3}`,
		"trailing text":     `{} {}`,
		"trailing brace":    `{}}`,
		"trailing bracket":  `{}]`,
		"q null":            `{"q":null}`,
		"favorite null":     `{"favorite":null}`,
		"tags null":         `{"tags":null}`,
		"tagKind null":      `{"tagKind":null}`,
		"person null":       `{"person":null}`,
		"organization null": `{"organization":null}`,
		"q too long":        `{"q":"` + strings.Repeat("あ", SmartRuleMaxQueryLength+1) + `"}`,
		"q not a string":    `{"q":3}`,
		"favorite not bool": `{"favorite":1}`,
		"tags empty":        `{"tags":[]}`,
		"tags too many":     `{"tags":` + sixteen + `,17]}`,
		"tags zero id":      `{"tags":[0]}`,
		"tags negative id":  `{"tags":[-1]}`,
		"tags duplicate":    `{"tags":[2,2]}`,
		"tags not array":    `{"tags":2}`,
		"tagKind zero":      `{"tagKind":0}`,
		"person negative":   `{"person":-3}`,
		"organization zero": `{"organization":0}`,
		"negative duration": `{"minDurationMs":-1}`,
		"reversed duration": `{"minDurationMs":2000,"maxDurationMs":1000}`,
		"day not a day":     `{"addedFrom":"2025-02-30"}`,
		"day not iso":       `{"addedTo":"2025/01/01"}`,
		"reversed days":     `{"addedFrom":"2025-03-02","addedTo":"2025-03-01"}`,
		"unknown watch":     `{"watchState":"seen"}`,
		"addedFrom null":    `{"addedFrom":null}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSmartRule([]byte(raw)); !errors.Is(err, ErrInvalidSmartRule) {
				t.Fatalf("ParseSmartRule(%s) error = %v, want ErrInvalidSmartRule", raw, err)
			}
		})
	}
	if _, err := ParseSmartRule([]byte(`{"tags":` + sixteen + `]}`)); err != nil {
		t.Fatalf("16 tags error = %v", err)
	}
	if _, err := ParseSmartRule([]byte(`{"q":"` + strings.Repeat("あ", SmartRuleMaxQueryLength) + `"}`)); err != nil {
		t.Fatalf("500 characters error = %v", err)
	}
}

func TestSmartRuleEncodeValidates(t *testing.T) {
	if _, err := (SmartRule{Person: -1}).Encode(); !errors.Is(err, ErrInvalidSmartRule) {
		t.Fatalf("Encode() error = %v", err)
	}
}

func TestValidateSmartSortRejectsRandom(t *testing.T) {
	for _, sort := range []VideoSort{SortRandom, "", "nameAsc"} {
		if err := ValidateSmartSort(sort); !errors.Is(err, ErrInvalidSmartRule) {
			t.Errorf("ValidateSmartSort(%q) error = %v", sort, err)
		}
	}
	for _, sort := range []VideoSort{SortAddedDesc, SortTitleAsc, SortFavoritedDesc} {
		if err := ValidateSmartSort(sort); err != nil {
			t.Errorf("ValidateSmartSort(%q) error = %v", sort, err)
		}
	}
}

func TestValidateSmartFolderParent(t *testing.T) {
	// 1 → 2 → 3; 4 is a separate root.
	parents := map[int64]int64{2: 1, 3: 2}
	cases := []struct {
		id, parent int64
		want       error
	}{
		{1, 1, ErrSmartFolderCycle},
		{1, 2, ErrSmartFolderCycle},
		{1, 3, ErrSmartFolderCycle},
		{2, 3, ErrSmartFolderCycle},
		{3, 1, nil},
		{2, 4, nil},
		{1, 0, nil},
		{0, 3, nil},
	}
	for _, c := range cases {
		if err := ValidateSmartFolderParent(c.id, c.parent, parents); !errors.Is(err, c.want) || (c.want == nil && err != nil) {
			t.Errorf("ValidateSmartFolderParent(%d, %d) = %v, want %v", c.id, c.parent, err, c.want)
		}
	}
	// Malformed parent cycles must terminate.
	if err := ValidateSmartFolderParent(9, 5, map[int64]int64{5: 6, 6: 5}); err != nil {
		t.Errorf("loop error = %v", err)
	}
}

func TestSmartRuleReplaceRefs(t *testing.T) {
	rule := SmartRule{Q: "x", Tags: []int64{3, 1, 2}, TagKind: 1, Person: 1, Organization: 2}
	cases := []struct {
		name    string
		kind    SmartRuleRefKind
		sources []int64
		target  int64
		want    SmartRule
		changed bool
	}{
		{"tags keep order and drop duplicates", SmartRefTag, []int64{1, 3}, 2,
			SmartRule{Q: "x", Tags: []int64{2}, TagKind: 1, Person: 1, Organization: 2}, true},
		{"a tag source becomes the target in place", SmartRefTag, []int64{1}, 9,
			SmartRule{Q: "x", Tags: []int64{3, 9, 2}, TagKind: 1, Person: 1, Organization: 2}, true},
		{"tag kind", SmartRefTagKind, []int64{1}, 5,
			SmartRule{Q: "x", Tags: []int64{3, 1, 2}, TagKind: 5, Person: 1, Organization: 2}, true},
		{"person only", SmartRefPerson, []int64{1}, 7,
			SmartRule{Q: "x", Tags: []int64{3, 1, 2}, TagKind: 1, Person: 7, Organization: 2}, true},
		{"organization only", SmartRefOrganization, []int64{2}, 8,
			SmartRule{Q: "x", Tags: []int64{3, 1, 2}, TagKind: 1, Person: 1, Organization: 8}, true},
		{"no source named", SmartRefPerson, []int64{4}, 7, rule, false},
		{"no tag source named", SmartRefTag, []int64{4, 5}, 1, rule, false},
	}
	for _, c := range cases {
		got, changed := rule.ReplaceRefs(c.kind, c.sources, c.target)
		if !reflect.DeepEqual(got, c.want) || changed != c.changed {
			t.Errorf("%s: ReplaceRefs = %+v, %v; want %+v, %v", c.name, got, changed, c.want, c.changed)
		}
	}
	if !reflect.DeepEqual(rule.Tags, []int64{3, 1, 2}) {
		t.Errorf("ReplaceRefs changed the receiver's tags: %v", rule.Tags)
	}
	if got, changed := (SmartRule{}).ReplaceRefs(SmartRefPerson, []int64{0}, 7); changed || got.Person != 0 {
		t.Errorf("empty rule = %+v, %v", got, changed)
	}
}
