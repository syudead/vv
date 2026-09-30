package domain

// TranscodeQuality はライブ変換で縮める画質である。空は「元の画質」で、今までどおり
// 縮めず、ビットレートの上限も付けない
// （specs/027-playback-quality/contracts/transcode-quality-api.md）。
type TranscodeQuality string

const (
	TranscodeQuality1080p TranscodeQuality = "1080p"
	TranscodeQuality720p  TranscodeQuality = "720p"
	TranscodeQuality480p  TranscodeQuality = "480p"
	TranscodeQuality360p  TranscodeQuality = "360p"
)

// TranscodeQualities は知っている画質を大きい順に並べる。
var TranscodeQualities = []TranscodeQuality{
	TranscodeQuality1080p,
	TranscodeQuality720p,
	TranscodeQuality480p,
	TranscodeQuality360p,
}

// TranscodeQualityLimits は 1 つの画質の変換の約束である（契約 §2）。
type TranscodeQualityLimits struct {
	// ShortSide は表示の短辺（画素）である。
	ShortSide int
	// VideoKbps は映像のビットレートの上限（-maxrate）である。
	VideoKbps int
	// BufferKbps は VBV の大きさ（-bufsize）で、上限の 2 倍である。
	BufferKbps int
	// AudioKbps は AAC の音声のビットレートである。
	AudioKbps int
}

var transcodeQualityLimits = map[TranscodeQuality]TranscodeQualityLimits{
	TranscodeQuality1080p: {ShortSide: 1080, VideoKbps: 5000, BufferKbps: 10000, AudioKbps: 128},
	TranscodeQuality720p:  {ShortSide: 720, VideoKbps: 2500, BufferKbps: 5000, AudioKbps: 128},
	TranscodeQuality480p:  {ShortSide: 480, VideoKbps: 1200, BufferKbps: 2400, AudioKbps: 96},
	TranscodeQuality360p:  {ShortSide: 360, VideoKbps: 700, BufferKbps: 1400, AudioKbps: 64},
}

// ParseTranscodeQuality は文字列を画質として解釈する。知らない文字列（空を含む）は
// 解釈できない。
func ParseTranscodeQuality(value string) (TranscodeQuality, bool) {
	quality := TranscodeQuality(value)
	if _, ok := transcodeQualityLimits[quality]; !ok {
		return "", false
	}
	return quality, true
}

// Limits は画質の変換の約束を返す。知らない画質（空を含む）は ok が偽である。
func (q TranscodeQuality) Limits() (TranscodeQualityLimits, bool) {
	limits, ok := transcodeQualityLimits[q]
	return limits, ok
}

// Available は、表示の寸法（回転を反映済み）が width×height の動画にこの画質が使えるかを
// 返す。画質の短辺が動画の短辺より小さいときだけ使え、寸法が無ければ使えない
// （specs/027-playback-quality/research.md R-3）。
func (q TranscodeQuality) Available(width, height int) bool {
	limits, ok := q.Limits()
	if !ok || width <= 0 || height <= 0 {
		return false
	}
	return limits.ShortSide < min(width, height)
}
