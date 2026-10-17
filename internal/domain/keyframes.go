package domain

// KeyframeSpacing is a bounded sample, not a guarantee about the whole video.
type KeyframeSpacing string

const (
	KeyframesUnknown     KeyframeSpacing = "unknown"
	KeyframesShort       KeyframeSpacing = "short"
	KeyframesSparse      KeyframeSpacing = "sparse"
	KeyframeProbeVersion                 = 1
)

// KeyframeProbe binds a sample to the file that was actually read.
type KeyframeProbe struct {
	Version int
	Source  FileStamp
	Spacing KeyframeSpacing
}

func (p KeyframeProbe) SparseFor(source FileStamp) bool {
	return p.Version == KeyframeProbeVersion && p.Source == source && p.Spacing == KeyframesSparse
}
