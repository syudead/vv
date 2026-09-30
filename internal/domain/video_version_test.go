package domain

import "testing"

// 尺がほぼ同じかは、差が 1 秒と長い方の 0.5% の大きい方以下で決まる（data-model.md §2）。
func TestDurationsMatch(t *testing.T) {
	for _, c := range []struct {
		name string
		a, b int64
		want bool
	}{
		{"同じ", 60_000, 60_000, true},
		{"1 秒の差", 60_000, 61_000, true},
		{"1 秒を超える差", 60_000, 61_001, false},
		{"順序に依らない", 61_001, 60_000, false},
		{"長い動画は 0.5%", 3_600_000, 3_618_000, true},
		{"0.5% を超える", 3_600_000, 3_618_091, false},
		{"0 の尺", 0, 0, false},
		{"負の尺", -1, 500, false},
	} {
		if got := DurationsMatch(c.a, c.b); got != c.want {
			t.Errorf("%s: DurationsMatch(%d, %d) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
	}
}
