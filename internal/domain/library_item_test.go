package domain

import (
	"testing"
	"time"
)

// 並んだメンバーの再生の記録の書き方: n は記録なし、0 は位置 0、m は途中、d は完了。
func progressOf(pattern string) []*Progress {
	out := make([]*Progress, 0, len(pattern))
	for _, c := range pattern {
		switch c {
		case 'n':
			out = append(out, nil)
		case '0':
			out = append(out, &Progress{PositionMs: 0})
		case 'm':
			out = append(out, &Progress{PositionMs: 30_000})
		case 'd':
			out = append(out, &Progress{PositionMs: 60_000, Completed: true})
		}
	}
	return out
}

// グループの視聴状態・見終えた本数・開くメンバー（specs/017-folder-groups/data-model.md §6）。
func TestGroupWatchAndOpenIndex(t *testing.T) {
	tests := []struct {
		pattern string
		state   WatchState
		watched int
		open    int
	}{
		{"nn", WatchStateUnwatched, 0, 0},
		{"n0", WatchStateUnwatched, 0, 0},
		// 途中のメンバーがあれば、それが最初の未完了より先でも後でも開く。
		{"nm", WatchStateInProgress, 0, 1},
		{"dnm", WatchStateInProgress, 1, 2},
		{"mdm", WatchStateInProgress, 1, 0},
		// 途中のものが無ければ最初の未完了。
		{"ddn", WatchStateInProgress, 2, 2},
		{"d0d", WatchStateInProgress, 2, 1},
		// 全部完了なら watched で、最初のメンバー。
		{"ddd", WatchStateWatched, 3, 0},
		{"", WatchStateUnwatched, 0, 0},
	}
	for _, tc := range tests {
		progress := progressOf(tc.pattern)
		state, watched := GroupWatch(progress)
		if state != tc.state || watched != tc.watched {
			t.Errorf("%q: GroupWatch = %s・%d, want %s・%d", tc.pattern, state, watched, tc.state, tc.watched)
		}
		if open := GroupOpenIndex(progress); open != tc.open {
			t.Errorf("%q: GroupOpenIndex = %d, want %d", tc.pattern, open, tc.open)
		}
	}
}

// 合計と最大は、分かっている値だけから作る。
func TestNewLibraryGroupAggregates(t *testing.T) {
	base := time.Unix(1_000, 0)
	duration := int64(1500)
	members := []Video{
		{ID: 1, SizeBytes: 10, AddedAt: base},
		{ID: 2, SizeBytes: 20, AddedAt: base.Add(time.Hour), DurationMs: &duration},
		{ID: 3, SizeBytes: 30, AddedAt: base.Add(time.Minute), DurationMs: &duration},
	}
	progress := []*Progress{nil, {PositionMs: 10, UpdatedAt: base.Add(2 * time.Hour)}, {Completed: true, UpdatedAt: base}}
	group := NewLibraryGroup("/m/g", "g", members, progress)
	if group.SizeBytes != 60 || group.DurationMs == nil || *group.DurationMs != 3000 {
		t.Errorf("大きさ・長さ = %d・%v", group.SizeBytes, group.DurationMs)
	}
	if !group.AddedAt.Equal(base.Add(time.Hour)) {
		t.Errorf("追加日時 = %v", group.AddedAt)
	}
	if group.LastPlayedAt == nil || !group.LastPlayedAt.Equal(base.Add(2*time.Hour)) {
		t.Errorf("最後に再生した時刻 = %v", group.LastPlayedAt)
	}
	if group.OpenVideoID != 2 || group.WatchState != WatchStateInProgress || group.WatchedCount != 1 {
		t.Errorf("開く・状態・本数 = %d・%s・%d", group.OpenVideoID, group.WatchState, group.WatchedCount)
	}

	empty := NewLibraryGroup("/m/g", "g", members[:1], []*Progress{nil})
	if empty.DurationMs != nil || empty.LastPlayedAt != nil {
		t.Errorf("長さ・再生の無いグループ = %v・%v, want どちらも無い", empty.DurationMs, empty.LastPlayedAt)
	}
}
