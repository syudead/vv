package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Bound probing so one corrupt file cannot stall ingestion of the remaining library.
const probeTimeout = 30 * time.Second

// Preflight already verified this command exists.
const probeCommand = "ffprobe"

// Probe reads format and streams in one process to avoid repeated startup costs, then separately
// probes keyframes for copyable video. Source stamps guard cached results against replacement, and
// optional TS broadcast metadata has its own deadline.
func Probe(ctx context.Context, path string) (domain.Probe, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	info, err := os.Stat(path)
	if err != nil {
		return domain.Probe{}, domain.NewProbeFailure(domain.ProbeErrorFileUnavailable,
			fmt.Errorf("could not check the file to probe (%s): %w", path, err))
	}
	probe, err := readImportProbe(ctx, probeCtx, path)
	if err != nil {
		return domain.Probe{}, err
	}
	if probe.Transcode != nil && videoCanCopy(probe.Transcode.Video) {
		keyframes := ProbeKeyframes(ctx, path)
		probe.Keyframes = &keyframes
	}
	after, err := os.Stat(path)
	if err != nil {
		return domain.Probe{}, domain.NewProbeFailure(domain.ProbeErrorFileUnavailable,
			fmt.Errorf("could not check the probed file (%s): %w", path, err))
	}
	return stampProbe(probe, domain.FileStampOf(info), domain.FileStampOf(after)), nil
}

// ProbeMediaInfo re-reads only the media information of an imported file; an unreadable or changing file gives
// a zero Source.
func ProbeMediaInfo(ctx context.Context, path string) domain.MediaInfoProbe {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	before, err := os.Stat(path)
	if err != nil {
		return domain.MediaInfoProbe{}
	}
	probe, err := readImportProbe(ctx, probeCtx, path)
	after, statErr := os.Stat(path)
	if statErr != nil || domain.FileStampOf(before) != domain.FileStampOf(after) {
		return domain.MediaInfoProbe{}
	}
	if err != nil {
		return domain.MediaInfoProbe{Source: domain.FileStampOf(before)}
	}
	return domain.MediaInfoProbe{Source: domain.FileStampOf(before), Info: probe.Media}
}

// readImportProbe runs the import's ffprobe and the optional reads that complete its media information; ctx bounds
// the TS read, which has its own deadline, and probeCtx the ffprobe processes.
func readImportProbe(ctx, probeCtx context.Context, path string) (domain.Probe, error) {
	output, err := command(probeCtx, PriorityLow, probeCommand, importProbeArgs(path)...).Output()
	if err != nil {
		return domain.Probe{}, probeRunFailure(probeCtx, path, err)
	}
	probe, err := parseImportProbe(path, output)
	if err != nil {
		return domain.Probe{}, err
	}
	if probe.Media != nil && isTransportStreamPath(path) {
		probe.Media.Broadcast = readBroadcast(ctx, path, probe.DurationMs)
	}
	if probe.Media != nil && probe.Media.Video != nil && strings.EqualFold(probe.VideoCodec, "av1") {
		probe.Media.Video.FilmGrain = probeFilmGrain(probeCtx, path)
	}
	return probe, nil
}

// probeRunFailure classifies an ffprobe run that did not finish; only FFmpeg's invalid-data error repeats.
func probeRunFailure(probeCtx context.Context, path string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return domain.NewProbeFailure(domain.ProbeErrorProbeFailed, exitFailure(fmt.Errorf(
			"%s failed (%s): %s", probeCommand, path, firstLine(exitErr.Stderr)), exitErr.Stderr))
	}
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
		// Treat per-file timeout as a corrupt input and continue ingestion; a slow disk may pass, so it retries.
		return domain.NewProbeFailure(domain.ProbeErrorProbeFailed,
			fmt.Errorf("%s timed out (%s): %w", probeCommand, path, err))
	}
	return domain.NewProbeFailure(domain.ProbeErrorProbeUnavailable,
		fmt.Errorf("could not run %s (%s): %w", probeCommand, path, err))
}

// parseImportProbe reads ffprobe's output; output it cannot interpret is the same on every read, so it repeats.
func parseImportProbe(path string, output []byte) (domain.Probe, error) {
	probe, err := parseProbeOutput(output)
	if err != nil {
		return domain.Probe{}, domain.NewProbeFailure(domain.ProbeErrorInvalidMetadata, domain.FailureRepeats(
			fmt.Errorf("could not interpret the %s output (%s): %w", probeCommand, path, err)))
	}
	return probe, nil
}

// Discard transcode and media metadata when size/mtime changes during probing to avoid caching
// mixed content under a valid stamp; index values still apply and missing metadata remains eligible
// for reprobe.
func stampProbe(probe domain.Probe, before, after domain.FileStamp) domain.Probe {
	probe.Source = before
	if before != after {
		probe.Keyframes = nil
		probe.Transcode = nil
		probe.Media = nil
	}
	return probe
}

// Place paths after -- so names beginning with a dash cannot become options.
func probeArgs(path string) []string {
	return []string{
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"--", path,
	}
}

// AV1 declares film grain only in its sequence header, which ffprobe reports after decoding a frame, so only
// AV1 files pay for a second process that decodes one; a failure leaves the item unknown.
func probeFilmGrain(ctx context.Context, path string) *bool {
	output, err := command(ctx, PriorityLow, probeCommand, filmGrainProbeArgs(path)...).Output()
	if err != nil {
		return nil
	}
	return parseFilmGrain(output)
}

func filmGrainProbeArgs(path string) []string {
	return []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-analyze_frames", "-export_side_data", "film_grain",
		"-read_intervals", "%+#1",
		"-show_entries", "stream=film_grain",
		"-print_format", "json",
		"--", path,
	}
}

// Import probing adds chapters and longer TS reads; request-time probing stays short to avoid
// delaying playback.
func importProbeArgs(path string) []string {
	args := []string{"-show_chapters"}
	if isTransportStreamPath(path) {
		args = append(args, transportStreamProbeArgs...)
	}
	return append(args, probeArgs(path)...)
}

// Read up to thirty seconds/100 MiB to discover late-starting TS audio or subtitles missed by
// ffprobe defaults.
var transportStreamProbeArgs = []string{"-analyzeduration", "30M", "-probesize", "100M"}

func isTransportStreamPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".m2ts", ".mts":
		return true
	}
	return false
}

type probeSideData struct {
	SideDataType string    `json:"side_data_type"`
	Rotation     float64   `json:"rotation"`
	Projection   probeText `json:"projection"`
	Type         probeText `json:"type"`
	probeSideDataDetails
}

type probeOutput struct {
	Streams  []probeStream  `json:"streams"`
	Chapters []probeChapter `json:"chapters"`
	Format   struct {
		Duration   string    `json:"duration"`
		FormatName string    `json:"format_name"`
		Tags       probeTags `json:"tags"`
		StartTime  probeText `json:"start_time"`
		Size       probeText `json:"size"`
		ProbeScore probeText `json:"probe_score"`
	} `json:"format"`
}

type probeStream struct {
	Index             int    `json:"index"`
	CodecType         string `json:"codec_type"`
	CodecName         string `json:"codec_name"`
	Profile           string `json:"profile"`
	PixelFormat       string `json:"pix_fmt"`
	BitsPerRawSample  string `json:"bits_per_raw_sample"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	Level             int    `json:"level"`
	AverageFrameRate  string `json:"avg_frame_rate"`
	RealFrameRate     string `json:"r_frame_rate"`
	SampleAspectRatio string `json:"sample_aspect_ratio"`
	SampleRate        string `json:"sample_rate"`
	Channels          int    `json:"channels"`
	ChannelLayout     string `json:"channel_layout"`
	CodecTag          string `json:"codec_tag_string"`
	ColorTransfer     string `json:"color_transfer"`
	FieldOrder        string `json:"field_order"`
	BitRate           string `json:"bit_rate"`
	ClosedCaptions    int    `json:"closed_captions"`
	// Tag keys are normalized to lowercase.
	Tags         probeTags        `json:"tags"`
	SideDataList []probeSideData  `json:"side_data_list"`
	Disposition  probeDisposition `json:"disposition"`
	probeStreamDetails
}

// Share JSON interpretation between ingestion and live transcoding; reject unknown duration because
// listings and thumbnail positions require it.
func parseProbeOutput(output []byte) (domain.Probe, error) {
	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return domain.Probe{}, fmt.Errorf("not valid JSON: %w", err)
	}

	seconds, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		return domain.Probe{}, fmt.Errorf("could not read the duration (format.duration=%q)", parsed.Format.Duration)
	}
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return domain.Probe{}, fmt.Errorf("invalid duration (format.duration=%q)", parsed.Format.Duration)
	}

	probe := domain.Probe{
		DurationMs: int64(math.Round(seconds * 1000)),
		FormatName: parsed.Format.FormatName,
	}

	// Transcoding uses the first nonattached video and first audio stream; index codec selection skips
	// unnamed codecs so later unsupported audio is not mistaken for direct-playable content.
	transcode := domain.TranscodeProbe{FormatName: strings.ToLower(parsed.Format.FormatName)}
	videoFound := false
	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			if stream.Disposition.AttachedPicture {
				continue
			}
			if !videoFound {
				videoFound = true
				transcode.Video = transcodeVideo(stream)
			}
			if probe.VideoCodec == "" {
				probe.VideoCodec = stream.CodecName
				// Apply display rotation so portrait recordings are not indexed as landscape.
				probe.Width, probe.Height = stream.Width, stream.Height
				rotated := false
				if rotation := streamRotation(stream.Tags.get("rotate"), stream.SideDataList); rotation == 90 || rotation == 270 {
					probe.Width, probe.Height = stream.Height, stream.Width
					rotated = true
				}
				probe.DisplayAspectRatio = displayAspectRatio(stream.Width, stream.Height, stream.SampleAspectRatio, rotated)
			}
		case "audio":
			if transcode.Audio == nil {
				audio := transcodeAudio(stream)
				transcode.Audio = &audio
			}
			if probe.AudioCodec == "" {
				probe.AudioCodec = stream.CodecName
			}
		}
	}
	// Without dimensions, omit unusable transcode metadata from persistence.
	if videoFound && transcode.Video.Width > 0 && transcode.Video.Height > 0 {
		probe.Transcode = &transcode
	}
	probe.Media = parseMediaInfo(parsed, probe.DurationMs)

	return probe, nil
}

func transcodeVideo(stream probeStream) domain.TranscodeVideo {
	sampleAspectNum, sampleAspectDen := parseAspectRatio(stream.SampleAspectRatio)
	return domain.TranscodeVideo{
		Index:            stream.Index,
		CodecName:        strings.ToLower(stream.CodecName),
		Profile:          stream.Profile,
		Level:            stream.Level,
		PixelFormat:      strings.ToLower(stream.PixelFormat),
		BitsPerRawSample: parsePositiveInt(stream.BitsPerRawSample),
		Width:            stream.Width,
		Height:           stream.Height,
		SampleAspectNum:  sampleAspectNum,
		SampleAspectDen:  sampleAspectDen,
		Rotation:         streamRotation(stream.Tags.get("rotate"), stream.SideDataList),
		FPS:              parseFrameRate(stream.AverageFrameRate),
		RealFPS:          parseFrameRate(stream.RealFrameRate),
	}
}

func transcodeAudio(stream probeStream) domain.TranscodeAudio {
	return domain.TranscodeAudio{
		Index:      stream.Index,
		CodecName:  strings.ToLower(stream.CodecName),
		Profile:    stream.Profile,
		SampleRate: parsePositiveInt(stream.SampleRate),
		Channels:   stream.Channels,
	}
}

// Account for sample aspect ratio and rotation; return zero for unknown dimensions.
func displayAspectRatio(width, height int, sampleAspectRatio string, rotated bool) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	numerator, denominator := parseAspectRatio(sampleAspectRatio)
	if numerator <= 0 || denominator <= 0 {
		numerator, denominator = 1, 1
	}
	ratio := float64(width) * float64(numerator) / (float64(height) * float64(denominator))
	if rotated {
		ratio = 1 / ratio
	}
	return ratio
}

// Keep error text bounded to the first stderr line.
func firstLine(raw []byte) string {
	for i, b := range raw {
		if b == '\n' {
			return string(raw[:i])
		}
	}
	if len(raw) == 0 {
		return "(no output)"
	}
	return string(raw)
}
