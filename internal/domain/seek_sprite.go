package domain

// シーク用サムネイルのスプライトの配置（specs/021-seek-thumbnail-sprite/research.md
// R-1）。1 シートは SeekSpriteColumns × SeekSpriteRows のコマの格子で、1 本の動画は
// 最大 SeekSpriteMaxSheets 枚・SeekSpriteMaxFrames コマになる。間隔は
// SeekThumbnailInterval を最小とし、それで覆えない長さの動画だけ広げる。
const (
	SeekSpriteColumns   = 10
	SeekSpriteRows      = 10
	SeekSpriteMaxSheets = 6
	// SeekSpriteFramesPerSheet は 1 シートに並ぶコマの数である。
	SeekSpriteFramesPerSheet = SeekSpriteColumns * SeekSpriteRows
	// SeekSpriteMaxFrames は 1 本の動画のコマの上限である。
	SeekSpriteMaxFrames = SeekSpriteFramesPerSheet * SeekSpriteMaxSheets
)

// SeekSpriteLayout は 1 本の動画のスプライトの配置である。コマ k は
// [k*IntervalMs, (k+1)*IntervalMs) の位置を受け持ち、シート k/SeekSpriteFramesPerSheet の
// 左上から行優先で並ぶ。生成（internal/media）と読み出しが同じ値を使う。
type SeekSpriteLayout struct {
	IntervalMs int64
	FrameCount int
	Columns    int
	Rows       int
	SheetCount int
}

// NewSeekSpriteLayout は動画の長さ durationMs からスプライトの配置を決める。
// 長さが分からない（0 以下の）動画は、最小の間隔の 1 コマ・1 シートにする。
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

// FrameAt は位置 positionMs を受け持つコマの番号を返す。範囲の外の位置は
// 先頭または末尾のコマに寄せる。
func (l SeekSpriteLayout) FrameAt(positionMs int64) int {
	if positionMs <= 0 || l.IntervalMs <= 0 {
		return 0
	}
	return int(min(positionMs/l.IntervalMs, int64(l.FrameCount-1)))
}

// ceilDiv は正の a, b について a/b の切り上げを返す。a+b-1 は a が int64 の上限に
// 近いとあふれるため、商と余りから求める。
func ceilDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 {
		q++
	}
	return q
}
