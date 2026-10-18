package domain

// Empty TranscodeQuality preserves original resolution without a bitrate cap.
type TranscodeQuality string

const (
	TranscodeQuality1080p TranscodeQuality = "1080p"
	TranscodeQuality720p  TranscodeQuality = "720p"
	TranscodeQuality480p  TranscodeQuality = "480p"
	TranscodeQuality360p  TranscodeQuality = "360p"
)

// TranscodeQualities is ordered largest first.
var TranscodeQualities = []TranscodeQuality{
	TranscodeQuality1080p,
	TranscodeQuality720p,
	TranscodeQuality480p,
	TranscodeQuality360p,
}

type TranscodeQualityLimits struct {
	// ShortSide is the displayed short edge in pixels.
	ShortSide int
	// VideoKbps is the -maxrate limit.
	VideoKbps int
	// BufferKbps is -bufsize, twice the bitrate limit.
	BufferKbps int
	AudioKbps  int
}

var transcodeQualityLimits = map[TranscodeQuality]TranscodeQualityLimits{
	TranscodeQuality1080p: {ShortSide: 1080, VideoKbps: 5000, BufferKbps: 10000, AudioKbps: 128},
	TranscodeQuality720p:  {ShortSide: 720, VideoKbps: 2500, BufferKbps: 5000, AudioKbps: 128},
	TranscodeQuality480p:  {ShortSide: 480, VideoKbps: 1200, BufferKbps: 2400, AudioKbps: 96},
	TranscodeQuality360p:  {ShortSide: 360, VideoKbps: 700, BufferKbps: 1400, AudioKbps: 64},
}

// ParseTranscodeQuality rejects unknown values, including empty.
func ParseTranscodeQuality(value string) (TranscodeQuality, bool) {
	quality := TranscodeQuality(value)
	if _, ok := transcodeQualityLimits[quality]; !ok {
		return "", false
	}
	return quality, true
}

func (q TranscodeQuality) Limits() (TranscodeQualityLimits, bool) {
	limits, ok := transcodeQualityLimits[q]
	return limits, ok
}

// Available requires known display dimensions and a strictly smaller target short edge.
func (q TranscodeQuality) Available(width, height int) bool {
	limits, ok := q.Limits()
	if !ok || width <= 0 || height <= 0 {
		return false
	}
	return limits.ShortSide < min(width, height)
}
