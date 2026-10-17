package domain

import (
	"errors"
	"reflect"
	"testing"
)

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
