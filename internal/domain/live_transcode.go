package domain

import (
	"io"
	"time"
)

type LiveTranscodeRequest struct {
	Path string
	// On-demand probe results are usable only if file size and modification time still match Source afterward.
	Source FileStamp
	// A nil Probe triggers ffprobe on demand.
	Probe   *TranscodeProbe
	StartMs int64
	// Normalize forces audio/video re-encoding only when switching from direct playback, not for seek or resume.
	Normalize bool
	// SparseKeyframes skips an unhelpful copy attempt on mid-file starts.
	SparseKeyframes bool
	// StartupDeadline covers both on-demand probing and the first output bytes.
	StartupDeadline time.Time
	// VideoEncoder defaults to software and is ignored for video-copy requests.
	VideoEncoder VideoEncoder
	// A nonempty Quality forces re-encoding, short-edge scaling, and bitrate limits; the caller must check TranscodeQuality.Available.
	Quality TranscodeQuality
}

type LiveTranscode struct {
	Stream io.ReadCloser
	// Call Wait exactly once.
	Wait func() error
	// Stop is safe to call repeatedly.
	Stop func()
	// StartMs maps output time zero to the original video: the requested position for encoding, zero for full copies, or the preceding keyframe for partial copies.
	StartMs int64
	// Probed is nil when using cached metadata or when the source changed after probing; persist only non-nil results.
	Probed *TranscodeProbe
	// VideoEncoder is empty when video was copied.
	VideoEncoder VideoEncoder
	// HardwareFailure captures the failed hardware attempt when this request falls back to software, including the stderr tail; callers log it.
	HardwareFailure error
}
