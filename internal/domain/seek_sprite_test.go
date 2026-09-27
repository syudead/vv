package domain

import (
	"math"
	"testing"
)

func TestNewSeekSpriteLayout(t *testing.T) {
	tests := []struct {
		name       string
		durationMs int64
		want       SeekSpriteLayout
	}{
		{"unknown duration", 0, SeekSpriteLayout{IntervalMs: 5000, FrameCount: 1, Columns: 10, Rows: 10, SheetCount: 1}},
		{"one second is a single frame", 1000, SeekSpriteLayout{IntervalMs: 5000, FrameCount: 1, Columns: 10, Rows: 10, SheetCount: 1}},
		{"exactly one interval", 5000, SeekSpriteLayout{IntervalMs: 5000, FrameCount: 1, Columns: 10, Rows: 10, SheetCount: 1}},
		{"three minutes", 180_000, SeekSpriteLayout{IntervalMs: 5000, FrameCount: 36, Columns: 10, Rows: 10, SheetCount: 1}},
		{"fifty minutes keeps five seconds", 3_000_000, SeekSpriteLayout{IntervalMs: 5000, FrameCount: 600, Columns: 10, Rows: 10, SheetCount: 6}},
		{"just over fifty minutes widens", 3_000_001, SeekSpriteLayout{IntervalMs: 5001, FrameCount: 600, Columns: 10, Rows: 10, SheetCount: 6}},
		{"one hour", 3_600_000, SeekSpriteLayout{IntervalMs: 6000, FrameCount: 600, Columns: 10, Rows: 10, SheetCount: 6}},
		{"two hours", 7_200_000, SeekSpriteLayout{IntervalMs: 12000, FrameCount: 600, Columns: 10, Rows: 10, SheetCount: 6}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewSeekSpriteLayout(tc.durationMs); got != tc.want {
				t.Errorf("NewSeekSpriteLayout(%d) = %+v, want %+v", tc.durationMs, got, tc.want)
			}
		})
	}
}

func TestSeekSpriteLayoutKeepsFiveSecondsUpToFiftyMinutes(t *testing.T) {
	for durationMs := int64(1); durationMs <= 3_000_000; durationMs += 997 {
		if got := NewSeekSpriteLayout(durationMs).IntervalMs; got != 5000 {
			t.Fatalf("NewSeekSpriteLayout(%d).IntervalMs = %d, want 5000", durationMs, got)
		}
	}
}

func TestSeekSpriteLayoutNeverExceedsLimits(t *testing.T) {
	durations := []int64{1, 4999, 5001, 499_999, 2_999_999, 3_000_001, 3_000_599, 3_000_600, 3_000_601, 7_199_999, 7_200_001, 36_000_000, 1<<53 - 1, math.MaxInt64 - 1, math.MaxInt64}
	for d := int64(1); d < 20_000_000; d = d*3 + 7 {
		durations = append(durations, d)
	}
	for _, durationMs := range durations {
		l := NewSeekSpriteLayout(durationMs)
		if l.FrameCount < 1 || l.FrameCount > SeekSpriteMaxFrames {
			t.Errorf("duration %d: FrameCount = %d", durationMs, l.FrameCount)
		}
		if l.SheetCount < 1 || l.SheetCount > SeekSpriteMaxSheets {
			t.Errorf("duration %d: SheetCount = %d", durationMs, l.SheetCount)
		}
		if l.SheetCount != (l.FrameCount+SeekSpriteFramesPerSheet-1)/SeekSpriteFramesPerSheet {
			t.Errorf("duration %d: SheetCount = %d for %d frames", durationMs, l.SheetCount, l.FrameCount)
		}
		// 全フレームで動画全体を覆い、最後のコマの区間は動画の中から始まる。
		// FrameCount*IntervalMs は int64 の上限付近であふれるため、最後のコマの
		// 開始位置から比べる。
		lastStart := int64(l.FrameCount-1) * l.IntervalMs
		if lastStart >= durationMs {
			t.Errorf("duration %d: last frame starts at %d", durationMs, lastStart)
		}
		if durationMs-lastStart > l.IntervalMs {
			t.Errorf("duration %d: %d frames x %d ms do not cover it", durationMs, l.FrameCount, l.IntervalMs)
		}
	}
}

func TestSeekSpriteLayoutFrameAt(t *testing.T) {
	l := NewSeekSpriteLayout(7_200_000)
	tests := []struct {
		positionMs int64
		want       int
	}{
		{-1, 0},
		{0, 0},
		{11_999, 0},
		{12_000, 1},
		{3_600_000, 300},
		{7_199_999, 599},
		{7_200_000, 599},
		{99_999_999, 599},
	}
	for _, tc := range tests {
		if got := l.FrameAt(tc.positionMs); got != tc.want {
			t.Errorf("FrameAt(%d) = %d, want %d", tc.positionMs, got, tc.want)
		}
	}
	if got := NewSeekSpriteLayout(1000).FrameAt(900); got != 0 {
		t.Errorf("single frame FrameAt = %d, want 0", got)
	}
}
