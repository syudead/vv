package domain

import (
	"errors"
	"reflect"
	"testing"
)

// 入れ子（research.md R-5）: 自分、子、孫を親に入れるのは循環で、兄弟やほかの枝は入れてよい。
func TestValidateNesting(t *testing.T) {
	// 1 ─┬─ 2 ── 3
	//    └─ 4
	// 5 は独立。
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

// 再生順（research.md R-7）: 深さ優先で、2 つの子グループにある動画は最初の 1 回だけ、見せない
// 項目は除く。
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

// 循環した入力でも止まる（書き込みが循環を拒むので通常は起きない）。
func TestGroupPlayOrderStopsOnCycle(t *testing.T) {
	children := map[int64][]GroupPlayItem{
		1: {{VideoID: 1}, {ChildID: 2}},
		2: {{VideoID: 2}, {ChildID: 1}},
	}
	if got := GroupPlayOrder([]GroupPlayItem{{ChildID: 1}}, children); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("GroupPlayOrder = %v", got)
	}
}
