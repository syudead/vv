package domain

import (
	"math"
	"testing"
)

func TestNewSeekSpriteLayout(t *testing.T) {
	tests := []struct {
		name       string
		durationMs int64
		frames     int
		intervalMs int64
	}{
		{"unknown duration", 0, 1, 1000},
		{"half a second", 500, 1, 1000},
		{"ten seconds", 10_000, 10, 1000},
		{"thirty seconds", 30_000, 30, 1000},
		{"81 frames at one second", 81_000, 81, 1000},
		{"above 81 frames widens interval", 81_001, 81, 1001},
		{"three minutes", 180_000, 81, 2223},
		{"45 minutes", 2_700_000, 81, 33_334},
		{"two hours", 7_200_000, 81, 88_889},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewSeekSpriteLayout(tc.durationMs)
			if got.FrameCount != tc.frames || got.IntervalMs != tc.intervalMs ||
				got.Columns != 9 || got.Rows != 9 || got.SheetCount != 1 {
				t.Errorf("NewSeekSpriteLayout(%d) = %+v", tc.durationMs, got)
			}
		})
	}
}

func TestSeekSpriteLayoutNeverExceedsLimits(t *testing.T) {
	durations := []int64{1, 999, 1001, 80_999, 81_000, 81_001, 405_001, 2_700_000, math.MaxInt64 - 1, math.MaxInt64}
	for d := int64(1); d < 20_000_000; d = d*3 + 7 {
		durations = append(durations, d)
	}
	for _, durationMs := range durations {
		l := NewSeekSpriteLayout(durationMs)
		if l.FrameCount < 1 || l.FrameCount > SeekSpriteMaxFrames || l.SheetCount != 1 {
			t.Errorf("duration %d: layout = %+v", durationMs, l)
		}
		lastStart := int64(l.FrameCount-1) * l.IntervalMs
		if lastStart >= durationMs || durationMs-lastStart > l.IntervalMs {
			t.Errorf("duration %d: last frame starts at %d with interval %d", durationMs, lastStart, l.IntervalMs)
		}
	}
}

func TestSeekSpriteLayoutFrameAt(t *testing.T) {
	l := NewSeekSpriteLayout(2_700_000)
	for _, tc := range []struct {
		positionMs int64
		want       int
	}{
		{-1, 0}, {0, 0}, {33_333, 0}, {33_334, 1}, {2_700_000, 80}, {99_999_999, 80},
	} {
		if got := l.FrameAt(tc.positionMs); got != tc.want {
			t.Errorf("FrameAt(%d) = %d, want %d", tc.positionMs, got, tc.want)
		}
	}
}
