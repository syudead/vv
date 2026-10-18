package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestValidateNesting(t *testing.T) {
	// 1 has children 2 and 4; 2 has child 3; 5 is independent.
	edges := []GroupEdge{{Parent: 1, Child: 2}, {Parent: 2, Child: 3}, {Parent: 1, Child: 4}}
	for name, c := range map[string]struct {
		parent, child int64
		cycle         bool
	}{
		"自分":           {parent: 2, child: 2, cycle: true},
		"子を親に入れる":      {parent: 2, child: 1, cycle: true},
		"孫を祖父に入れる":     {parent: 3, child: 1, cycle: true},
		"子の子に親を入れる":    {parent: 3, child: 2, cycle: true},
		"兄弟":           {parent: 4, child: 2},
		"兄弟の逆":         {parent: 2, child: 4},
		"既にある子をもう一度":   {parent: 1, child: 3},
		"ほかの枝のグループ":    {parent: 5, child: 1},
		"独立したグループを入れる": {parent: 3, child: 5},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateNesting(c.parent, c.child, edges)
			if c.cycle != errors.Is(err, ErrGroupCycle) || (!c.cycle && err != nil) {
				t.Fatalf("ValidateNesting(%d, %d) = %v; cycle %v", c.parent, c.child, err, c.cycle)
			}
		})
	}
}

func TestGroupPlayOrder(t *testing.T) {
	children := map[int64][]GroupPlayItem{
		10: {{VideoID: 1}, {VideoID: 2}},
		20: {{VideoID: 2}, {VideoID: 0}, {ChildID: 30}, {VideoID: 3}},
		30: {{VideoID: 4}, {VideoID: 1}},
	}
	for name, c := range map[string]struct {
		items []GroupPlayItem
		want  []int64
	}{
		"動画だけ": {items: []GroupPlayItem{{VideoID: 5}, {VideoID: 6}}, want: []int64{5, 6}},
		"子グループを深さ優先で置き換え、重複は最初だけ": {
			items: []GroupPlayItem{{VideoID: 9}, {ChildID: 10}, {ChildID: 20}, {VideoID: 1}},
			want:  []int64{9, 1, 2, 4, 3},
		},
		"見せない項目と無いグループは飛ばす": {
			items: []GroupPlayItem{{VideoID: 0}, {ChildID: 99}, {VideoID: 7}},
			want:  []int64{7},
		},
		"同じ子グループに 2 度着いても 1 度": {
			items: []GroupPlayItem{{ChildID: 30}, {ChildID: 30}},
			want:  []int64{4, 1},
		},
		"空": {items: nil, want: []int64{}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := GroupPlayOrder(c.items, children); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("GroupPlayOrder = %v; want %v", got, c.want)
			}
		})
	}
}

// Malformed cycles must terminate even though writes normally reject them.
func TestGroupPlayOrderStopsOnCycle(t *testing.T) {
	children := map[int64][]GroupPlayItem{
		1: {{VideoID: 1}, {ChildID: 2}},
		2: {{VideoID: 2}, {ChildID: 1}},
	}
	if got := GroupPlayOrder([]GroupPlayItem{{ChildID: 1}}, children); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("GroupPlayOrder = %v", got)
	}
}
