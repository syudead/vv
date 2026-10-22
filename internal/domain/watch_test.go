package domain

import "testing"

func TestClassifyWatch(t *testing.T) {
	tests := []struct {
		name     string
		progress *Progress
		want     WatchState
	}{
		{"記録なし", nil, WatchStateUnwatched},
		{"位置 0", &Progress{PositionMs: 0}, WatchStateUnwatched},
		{"途中", &Progress{PositionMs: 1}, WatchStateInProgress},
		{"完了", &Progress{PositionMs: 100, Completed: true}, WatchStateWatched},
		{"位置 0 でも完了", &Progress{PositionMs: 0, Completed: true}, WatchStateWatched},
	}
	for _, tc := range tests {
		if got := ClassifyWatch(tc.progress); got != tc.want {
			t.Errorf("%s: ClassifyWatch = %q, want %q", tc.name, got, tc.want)
		}
	}
}
