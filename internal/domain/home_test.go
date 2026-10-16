package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProposedShelfValidate(t *testing.T) {
	valid := []SmartRule{{Tags: []int64{1}}, {Person: 2}, {Organization: 3}}
	for _, rule := range valid {
		shelf, err := ProposedShelf{Name: "  Cats  ", Rule: rule}.Validate()
		if err != nil || shelf.Name != "Cats" {
			t.Errorf("Validate(%+v) = %+v, %v", rule, shelf, err)
		}
	}
	invalid := []SmartRule{
		{},
		{Tags: []int64{1, 2}},
		{Tags: []int64{1}, Person: 2},
		{Person: 2, Watch: WatchUnwatched},
		{Q: "cats"},
		{TagKind: 1},
		{Tags: []int64{-1}},
	}
	for _, rule := range invalid {
		if _, err := (ProposedShelf{Name: "Cats", Rule: rule}).Validate(); !errors.Is(err, ErrInvalidSmartRule) {
			t.Errorf("Validate(%+v) error = %v, want ErrInvalidSmartRule", rule, err)
		}
	}
	var nameErr *InvalidGroupNameError
	for _, name := range []string{" ", strings.Repeat("a", GroupNameMaxLength+1)} {
		if _, err := (ProposedShelf{Name: name, Rule: SmartRule{Person: 1}}).Validate(); !errors.As(err, &nameErr) {
			t.Errorf("Validate(name %d runes) error = %v", len(name), err)
		}
	}
}

func TestProposedShelfFolderIsARootAddedDescFolder(t *testing.T) {
	got := ProposedShelf{Name: "Cats", Rule: SmartRule{Tags: []int64{4}}}.Folder()
	want := SmartFolderCreate{Name: "Cats", Rule: SmartRule{Tags: []int64{4}}, Sort: SortAddedDesc}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Folder() = %+v, want %+v", got, want)
	}
}

func TestSameSmartRule(t *testing.T) {
	if !SameSmartRule(SmartRule{Tags: []int64{1, 2}}, SmartRule{Tags: []int64{2, 1}}) {
		t.Error("tag order changed the rule")
	}
	if SameSmartRule(SmartRule{Tags: []int64{1}}, SmartRule{Tags: []int64{1}, Favorite: true}) {
		t.Error("an extra filter was ignored")
	}
	if SameSmartRule(SmartRule{Person: 1}, SmartRule{Organization: 1}) {
		t.Error("a person and an organization were the same")
	}
}

func TestValidateHomeShelfOrder(t *testing.T) {
	if err := ValidateHomeShelfOrder([]int64{3, 1, 2}); err != nil {
		t.Fatal(err)
	}
	tooMany := make([]int64, MaxHomeShelves+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	for _, ids := range [][]int64{tooMany, {1, 1}, {0}} {
		if err := ValidateHomeShelfOrder(ids); !errors.Is(err, ErrInvalidHomeShelves) {
			t.Errorf("ValidateHomeShelfOrder(%d ids) error = %v", len(ids), err)
		}
	}
}

func TestMovedShelves(t *testing.T) {
	cases := []struct {
		before, after, want []int64
	}{
		{[]int64{1, 2, 3}, []int64{1, 2, 3}, nil},
		{[]int64{1, 2, 3, 4, 5}, []int64{5, 1, 2, 3, 4}, []int64{5}},
		{[]int64{1, 2, 3}, []int64{1, 3}, nil},
		{[]int64{1, 2}, []int64{1, 2, 9}, []int64{9}},
		{[]int64{1, 2, 3, 4}, []int64{1, 3, 2, 4}, []int64{2}},
	}
	for _, c := range cases {
		if got := MovedShelves(c.before, c.after); !reflect.DeepEqual(got, c.want) {
			t.Errorf("MovedShelves(%v, %v) = %v, want %v", c.before, c.after, got, c.want)
		}
	}
}

func TestHomeOrganizeLastValidate(t *testing.T) {
	if err := (HomeOrganizeLast{Run: HomeExtend, Outcome: HomeOrganizeUnchanged}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, last := range []HomeOrganizeLast{{Run: "later", Outcome: HomeOrganizeDone}, {Run: HomeRebuild, Outcome: "ok"}, {Run: HomeRebuild, Outcome: HomeOrganizeDone, Added: -1}} {
		if err := last.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil", last)
		}
	}
}
