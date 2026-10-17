package domain

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func shelfNames(proposal []ProposedShelf) []string {
	out := make([]string, len(proposal))
	for i, shelf := range proposal {
		out[i] = shelf.Name
	}
	return out
}

// 5 本以上の候補だけを、種類付きのタグ（種類の id 順）、種類なしのタグ、人物、団体の順に、それぞれ本数の
// 多い順で棚にする（docs/design-docs/home.md）。
func TestOrganizeByRulesOrdersAndFilters(t *testing.T) {
	candidates := []ShelfCandidate{
		{Kind: ShelfSourceOrganization, ID: 1, Name: "Maker", Videos: 9},
		{Kind: ShelfSourcePerson, ID: 2, Name: "Hanako", Videos: 5},
		{Kind: ShelfSourcePerson, ID: 3, Name: "Taro", Videos: 8},
		{Kind: ShelfSourceTag, ID: 4, Name: "Plain", Videos: 30},
		{Kind: ShelfSourceTag, ID: 5, Name: "Few", Videos: 4},
		{Kind: ShelfSourceTag, ID: 6, Name: "Genre B", TagKindID: 2, Videos: 7},
		{Kind: ShelfSourceTag, ID: 7, Name: "Genre A", TagKindID: 1, Videos: 5},
		{Kind: ShelfSourceTag, ID: 8, Name: "Genre C", TagKindID: 2, Videos: 12},
	}
	proposal := OrganizeByRules(candidates)
	want := []string{"Genre A", "Genre C", "Genre B", "Plain", "Taro", "Hanako", "Maker"}
	if got := shelfNames(proposal); !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	rules := []SmartRule{proposal[0].Rule, proposal[4].Rule, proposal[6].Rule}
	wantRules := []SmartRule{{Tags: []int64{7}}, {Person: 3}, {Organization: 1}}
	for i := range rules {
		if !SameSmartRule(rules[i], wantRules[i]) {
			t.Errorf("rule %d = %+v, want %+v", i, rules[i], wantRules[i])
		}
	}
	for _, shelf := range proposal {
		if _, err := shelf.Validate(); err != nil {
			t.Errorf("%s: %v", shelf.Name, err)
		}
	}
}

// 案は 24 までで、名前が規則に合わない候補は数えずに飛ばす。
func TestOrganizeByRulesCapsAtTwentyFour(t *testing.T) {
	candidates := []ShelfCandidate{{Kind: ShelfSourceTag, ID: 1, Name: strings.Repeat("x", 101), Videos: 100}}
	for i := 2; i <= 30; i++ {
		candidates = append(candidates, ShelfCandidate{Kind: ShelfSourceTag, ID: int64(i), Name: fmt.Sprintf("t%d", i), Videos: 50 - i})
	}
	proposal := OrganizeByRules(candidates)
	if len(proposal) != RuleShelfMaxShelves {
		t.Fatalf("len = %d, want %d", len(proposal), RuleShelfMaxShelves)
	}
	if proposal[0].Name != "t2" || proposal[23].Name != "t25" {
		t.Errorf("names = %v", shelfNames(proposal))
	}
}

// rebuild は extend を吸収する（R-9）。
func TestMergeHomeOrganizeRun(t *testing.T) {
	cases := []struct{ pending, run, want HomeOrganizeRun }{
		{"", HomeExtend, HomeExtend},
		{"", HomeRebuild, HomeRebuild},
		{HomeExtend, HomeRebuild, HomeRebuild},
		{HomeRebuild, HomeExtend, HomeRebuild},
		{HomeExtend, HomeExtend, HomeExtend},
		{HomeExtend, "", HomeExtend},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := MergeHomeOrganizeRun(c.pending, c.run); got != c.want {
			t.Errorf("Merge(%q, %q) = %q, want %q", c.pending, c.run, got, c.want)
		}
	}
}
