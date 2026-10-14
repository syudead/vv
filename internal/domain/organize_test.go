package domain

import (
	"reflect"
	"testing"
)

// マージの計画（research.md R-5）: X を持つ親とプレイリストは X の位置で Y に置き換える。すでに Y を
// 持つ親とプレイリスト、Y 自身と Y の子孫の親では、X の項目を消す。
func TestPlanGroupMerge(t *testing.T) {
	// Y(1) ── 2 ── 3、10 は独立。X(9) は 1・2・3・10・11 の子で、11 は Y も持つ。
	const target = 1
	edges := []GroupEdge{
		{Parent: 1, Child: 2}, {Parent: 2, Child: 3},
		{Parent: 1, Child: 9}, {Parent: 2, Child: 9}, {Parent: 3, Child: 9}, {Parent: 10, Child: 9},
		{Parent: 11, Child: 1}, {Parent: 11, Child: 9},
	}
	for name, c := range map[string]struct {
		playlists, parents []GroupMergeHolder
		want               GroupMergePlan
	}{
		"ほかの枝の親は置き換える": {
			parents: []GroupMergeHolder{{ID: 10}},
			want:    GroupMergePlan{ReplaceParents: []int64{10}},
		},
		"すでに Y を持つ親は消す": {
			parents: []GroupMergeHolder{{ID: 11, HoldsTarget: true}},
			want:    GroupMergePlan{RemoveParents: []int64{11}},
		},
		"Y 自身と Y の子孫の親は消す": {
			parents: []GroupMergeHolder{{ID: 1}, {ID: 2}, {ID: 3}},
			want:    GroupMergePlan{RemoveParents: []int64{1, 2, 3}},
		},
		"両方を持つプレイリストは消し、X だけなら置き換える": {
			playlists: []GroupMergeHolder{{ID: 7, HoldsTarget: true}, {ID: 8}},
			want:      GroupMergePlan{RemovePlaylists: []int64{7}, ReplacePlaylists: []int64{8}},
		},
		"何も持たれていない": {},
	} {
		t.Run(name, func(t *testing.T) {
			if got := PlanGroupMerge(target, c.playlists, c.parents, edges); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("PlanGroupMerge = %+v, want %+v", got, c.want)
			}
		})
	}
}
