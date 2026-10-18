package domain

// Seek sprites use up to 81 frames in a 9×9 sheet, preserving one-second spacing for short videos and spreading frames across longer videos.
const (
	SeekSpriteColumns        = 9
	SeekSpriteRows           = 9
	SeekSpriteMaxSheets      = 1
	SeekSpriteFramesPerSheet = SeekSpriteColumns * SeekSpriteRows
	SeekSpriteMaxFrames      = SeekSpriteFramesPerSheet * SeekSpriteMaxSheets
)

// Frame k covers [k*IntervalMs, (k+1)*IntervalMs), laid out row-first; generation and reads share this layout.
type SeekSpriteLayout struct {
	IntervalMs int64
	FrameCount int
	Columns    int
	Rows       int
	SheetCount int
}

// Unknown duration uses one frame and sheet at the minimum interval.
func NewSeekSpriteLayout(durationMs int64) SeekSpriteLayout {
	minInterval := SeekThumbnailInterval.Milliseconds()
	intervalMs := minInterval
	frameCount := int64(1)
	if durationMs > 0 {
		intervalMs = max(minInterval, ceilDiv(durationMs, SeekSpriteMaxFrames))
		frameCount = max(1, ceilDiv(durationMs, intervalMs))
	}
	return SeekSpriteLayout{
		IntervalMs: intervalMs,
		FrameCount: int(frameCount),
		Columns:    SeekSpriteColumns,
		Rows:       SeekSpriteRows,
		SheetCount: int(ceilDiv(frameCount, SeekSpriteFramesPerSheet)),
	}
}

func (l SeekSpriteLayout) FrameAt(positionMs int64) int {
	if positionMs <= 0 || l.IntervalMs <= 0 {
		return 0
	}
	return int(min(positionMs/l.IntervalMs, int64(l.FrameCount-1)))
}

// Compute ceiling division from quotient and remainder to avoid overflowing a+b-1.
func ceilDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 {
		q++
	}
	return q
}

// Published sprite dimensions come from generated sheets; every frame has the same pixel size.
type SeekSprite struct {
	SeekSpriteLayout
	FrameWidth  int
	FrameHeight int
}
