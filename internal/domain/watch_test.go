package domain

import "testing"

// 視聴状態の定義は data-model.md §6 と一覧画面の watchState に揃える。
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

func TestWatchFilter(t *testing.T) {
	for _, f := range []WatchFilter{WatchAll, WatchUnwatched, WatchInProgress, WatchWatched} {
		if !f.Valid() {
			t.Errorf("%q は既知の値のはず", f)
		}
	}
	for _, f := range []WatchFilter{"", "Watched", "done"} {
		if f.Valid() {
			t.Errorf("%q は未知の値のはず", f)
		}
	}
	states := []WatchState{WatchStateUnwatched, WatchStateInProgress, WatchStateWatched}
	for _, state := range states {
		if !WatchAll.Matches(state) || !WatchFilter("").Matches(state) {
			t.Errorf("all は %q に合うはず", state)
		}
	}
	if !WatchInProgress.Matches(WatchStateInProgress) || WatchInProgress.Matches(WatchStateWatched) {
		t.Error("inProgress は視聴途中だけに合うはず")
	}
}
