package domain

import "time"

const (
	// CompletionTailMs allows skipping ending credits.
	CompletionTailMs = 15_000
	// CompletionRatio avoids an overly strict fixed tail for long videos.
	CompletionRatio = 0.95
	// MinResumeMs avoids resuming barely started videos.
	MinResumeMs = 5_000
)

// Progress follows content_key across moves, renames, and replacements.
type Progress struct {
	PositionMs int64
	DurationMs int64
	Completed  bool
	UpdatedAt  time.Time
}

// EvaluateProgress determines completion server-side so clients and library views agree.
func EvaluateProgress(positionMs, durationMs int64) Progress {
	position := max(positionMs, 0)

	if durationMs > 0 {
		position = min(position, durationMs)
	}

	return Progress{
		PositionMs: position,
		DurationMs: durationMs,
		Completed:  isCompleted(position, durationMs),
	}
}

// Use either a fixed tail or ratio because a fixed tail dominates short videos while a percentage can span minutes on long ones.
func isCompleted(positionMs, durationMs int64) bool {
	if durationMs <= 0 {
		return false
	}
	if positionMs >= durationMs-CompletionTailMs {
		return true
	}
	return float64(positionMs)/float64(durationMs) >= CompletionRatio
}

// ResumePosition restarts completed or barely started videos from zero.
func (p Progress) ResumePosition() int64 {
	if p.Completed || p.PositionMs < MinResumeMs {
		return 0
	}
	return p.PositionMs
}
