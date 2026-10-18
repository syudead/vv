package domain

import (
	"encoding/json"
	"io/fs"
)

// Bump TranscodeProbeVersion when fields or semantics change; mismatches read as absent until re-probed.
const TranscodeProbeVersion = 1

// TranscodeProbe is persisted as JSON with Go field names in video_transcode_probes.probe.
type TranscodeProbe struct {
	// FormatName is lowercase and controls the MOV two-input workaround.
	FormatName string
	Video      TranscodeVideo
	Audio      *TranscodeAudio
}

type TranscodeVideo struct {
	Index            int
	CodecName        string
	Profile          string
	Level            int
	PixelFormat      string
	BitsPerRawSample int
	// Width and Height are encoded dimensions before rotation.
	Width           int
	Height          int
	SampleAspectNum int64
	SampleAspectDen int64
	// Rotation is 0, 90, 180, or 270 from Display Matrix or rotate metadata.
	Rotation int
	// FPS is avg_frame_rate; RealFPS is r_frame_rate.
	FPS     float64
	RealFPS float64
}

type TranscodeAudio struct {
	Index      int
	CodecName  string
	Profile    string
	SampleRate int
	Channels   int
}

// FileStamp compares size and modification time in Unix nanoseconds.
type FileStamp struct {
	SizeBytes int64
	ModTimeNs int64
}

func FileStampOf(info fs.FileInfo) FileStamp {
	return FileStamp{SizeBytes: info.Size(), ModTimeNs: info.ModTime().UnixNano()}
}

type StoredTranscodeProbe struct {
	Version int
	Source  FileStamp
	Probe   string
}

// TranscodeProbeUsable requires the current version, valid JSON, and a matching opened-file stamp.
func TranscodeProbeUsable(stored *StoredTranscodeProbe, opened FileStamp) (TranscodeProbe, bool) {
	if stored == nil || stored.Version != TranscodeProbeVersion || stored.Source != opened {
		return TranscodeProbe{}, false
	}
	var probe TranscodeProbe
	if err := json.Unmarshal([]byte(stored.Probe), &probe); err != nil {
		return TranscodeProbe{}, false
	}
	// Valid JSON such as null or {} still lacks the dimensions needed for transcoding.
	if probe.Video.Width <= 0 || probe.Video.Height <= 0 {
		return TranscodeProbe{}, false
	}
	return probe, true
}
