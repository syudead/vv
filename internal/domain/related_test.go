package domain

import "testing"

func TestVideoGroupPositionAndNeighbors(t *testing.T) {
	group := VideoGroup{MemberIDs: []int64{5, 3, 9}}
	for _, tc := range []struct {
		id, position int64
		prev, next   int64
	}{
		{id: 5, position: 1, prev: 0, next: 3},
		{id: 3, position: 2, prev: 5, next: 9},
		{id: 9, position: 3, prev: 3, next: 0},
		{id: 7, position: 0, prev: 0, next: 0},
	} {
		if got := group.Position(tc.id); int64(got) != tc.position {
			t.Errorf("Position(%d) = %d, want %d", tc.id, got, tc.position)
		}
		if prev, next := group.Neighbors(tc.id); prev != tc.prev || next != tc.next {
			t.Errorf("Neighbors(%d) = %d・%d, want %d・%d", tc.id, prev, next, tc.prev, tc.next)
		}
	}
}

func TestGroupWindowStart(t *testing.T) {
	for _, tc := range []struct {
		window          GroupWindow
		total, position int
		want            int
	}{
		{window: GroupWindow{Limit: 100, Around: true}, total: 6000, position: 3000, want: 2949},
		{window: GroupWindow{Limit: 100, Around: true}, total: 6000, position: 10, want: 0},
		{window: GroupWindow{Limit: 100, Around: true}, total: 6000, position: 5990, want: 5900},
		{window: GroupWindow{Limit: 100, Around: true}, total: 30, position: 30, want: 0},
		{window: GroupWindow{Offset: 200, Limit: 50}, total: 6000, position: 1, want: 200},
		{window: GroupWindow{Offset: 7000, Limit: 50}, total: 6000, position: 1, want: 6000},
		{window: GroupWindow{}, total: 6000, position: 3000, want: 0},
	} {
		if got := tc.window.Start(tc.total, tc.position); got != tc.want {
			t.Errorf("%+v.Start(%d, %d) = %d, want %d", tc.window, tc.total, tc.position, got, tc.want)
		}
	}
}
