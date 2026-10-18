package domain

import "testing"

func TestEvaluateProgressCompletion(t *testing.T) {
	tests := []struct {
		name       string
		positionMs int64
		durationMs int64
		want       bool
	}{
		{"先頭は未完了", 0, 600_000, false},
		{"途中は未完了", 300_000, 600_000, false},
		// For 100 seconds, only the 15-second tail applies at this position.
		{"残り 15 秒ちょうどで完了", 85_000, 100_000, true},
		{"残り 16 秒は未完了", 84_000, 100_000, false},
		// For ten minutes, the ratio permits completion before the fixed tail.
		{"95% ちょうどで完了", 570_000, 600_000, true},
		{"94% は未完了", 564_000, 600_000, false},
		{"長い動画は 95% で完了（残りは 6 分）", 6_840_000, 7_200_000, true},
		{"短い動画は残り 15 秒で完了", 5_000, 20_000, true},
		{"尺と同じなら完了", 600_000, 600_000, true},
		{"尺が不明なら完了にしない", 300_000, 0, false},
		{"尺が負なら完了にしない", 300_000, -1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateProgress(tc.positionMs, tc.durationMs)
			if got.Completed != tc.want {
				t.Errorf("Completed = %v, want %v", got.Completed, tc.want)
			}
		})
	}
}

func TestEvaluateProgressClampsPosition(t *testing.T) {
	tests := []struct {
		name       string
		positionMs int64
		durationMs int64
		want       int64
	}{
		{"負は 0 に丸める", -5000, 600_000, 0},
		{"尺を越えたら尺に丸める", 900_000, 600_000, 600_000},
		{"尺が不明なら丸めない", 900_000, 0, 900_000},
		{"範囲内はそのまま", 300_000, 600_000, 300_000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateProgress(tc.positionMs, tc.durationMs); got.PositionMs != tc.want {
				t.Errorf("PositionMs = %d, want %d", got.PositionMs, tc.want)
			}
		})
	}
}

func TestResumePosition(t *testing.T) {
	tests := []struct {
		name     string
		progress Progress
		want     int64
	}{
		{"途中まで見た動画は続きから", Progress{PositionMs: 300_000}, 300_000},
		{"見終わった動画は先頭から", Progress{PositionMs: 590_000, Completed: true}, 0},
		{"5 秒未満は先頭から", Progress{PositionMs: 4_999}, 0},
		{"5 秒ちょうどは続きから", Progress{PositionMs: 5_000}, 5_000},
		{"先頭は先頭のまま", Progress{PositionMs: 0}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.progress.ResumePosition(); got != tc.want {
				t.Errorf("ResumePosition() = %d, want %d", got, tc.want)
			}
		})
	}
}

// Changing these constants must be an intentional change to the completion contract.
func TestProgressThresholds(t *testing.T) {
	if CompletionTailMs != 15_000 {
		t.Errorf("CompletionTailMs = %d, want 15000", CompletionTailMs)
	}
	if CompletionRatio != 0.95 {
		t.Errorf("CompletionRatio = %v, want 0.95", CompletionRatio)
	}
	if MinResumeMs != 5_000 {
		t.Errorf("MinResumeMs = %d, want 5000", MinResumeMs)
	}
}
