package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestParseSmartRuleAcceptsEveryMember(t *testing.T) {
	raw := `{"q":"cat","watch":"inProgress","playable":true,"favorite":true,"tags":[3,1],
		"tagKind":4,"person":5,"organization":6,"addedWithin":7,"minDuration":60,"maxDuration":600}`
	rule, err := ParseSmartRule([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSmartRule() error = %v", err)
	}
	want := SmartRule{
		Q: "cat", Watch: WatchInProgress, Playable: true, Favorite: true, Tags: []int64{3, 1},
		TagKind: 4, Person: 5, Organization: 6,
		AddedWithin: intPtr(7), MinDuration: intPtr(60), MaxDuration: intPtr(600),
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
	if query.Query != "cat" || query.Watch != WatchInProgress || !query.PlayableOnly || !query.FavoriteOnly ||
		!reflect.DeepEqual(query.TagIDs, []int64{3, 1}) || query.Person != 5 || query.Organization != 6 ||
		query.AddedWithinDays != 7 || *query.MinDurationSec != 60 || *query.MaxDurationSec != 600 {
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

func TestParseSmartRuleRejectsOutOfRangeMembers(t *testing.T) {
	sixteen := "[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16"
	cases := map[string]string{
		"not an object":       `[]`,
		"null":                `null`,
		"empty":               ``,
		"unknown member":      `{"rating":3}`,
		"trailing text":       `{} {}`,
		"q too long":          `{"q":"` + strings.Repeat("あ", SmartRuleMaxQueryLength+1) + `"}`,
		"q not a string":      `{"q":3}`,
		"watch all":           `{"watch":"all"}`,
		"watch unknown":       `{"watch":"seen"}`,
		"playable not bool":   `{"playable":"yes"}`,
		"favorite not bool":   `{"favorite":1}`,
		"tags empty":          `{"tags":[]}`,
		"tags too many":       `{"tags":` + sixteen + `,17]}`,
		"tags zero id":        `{"tags":[0]}`,
		"tags negative id":    `{"tags":[-1]}`,
		"tags duplicate":      `{"tags":[2,2]}`,
		"tags not array":      `{"tags":2}`,
		"tagKind zero":        `{"tagKind":0}`,
		"person negative":     `{"person":-3}`,
		"organization zero":   `{"organization":0}`,
		"addedWithin zero":    `{"addedWithin":0}`,
		"addedWithin over":    `{"addedWithin":36501}`,
		"minDuration under":   `{"minDuration":-1}`,
		"maxDuration over":    `{"maxDuration":360001}`,
		"min over max":        `{"minDuration":601,"maxDuration":600}`,
		"addedWithin a float": `{"addedWithin":1.5}`,
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
	if _, err := (SmartRule{MinDuration: intPtr(10), MaxDuration: intPtr(5)}).Encode(); !errors.Is(err, ErrInvalidSmartRule) {
		t.Fatalf("Encode() error = %v", err)
	}
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
	// 1 ─ 2 ─ 3、4 は別の根。
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
	// 輪になった親（壊れたデータ）でも止まる。
	if err := ValidateSmartFolderParent(9, 5, map[int64]int64{5: 6, 6: 5}); err != nil {
		t.Errorf("loop error = %v", err)
	}
}
