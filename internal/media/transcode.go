package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	transcodeCommand   = "ffmpeg"
	transcodeStopDelay = 5 * time.Second
	stderrTailLimit    = 32 * 1024
	liveX264Preset     = "superfast"
	maxVideoLongSide   = 3840
	maxVideoShortSide  = 2160
	maxVideoFPS        = 60
	maxMacroblocksSec  = 983040
	// frag_keyframe emits the first fragment only after this many seconds have been encoded.
	liveKeyframeInterval = 2
	vaapiDevice          = "/dev/dri/renderD128"
)

// LiveTranscoder starts one FFmpeg process for each HTTP request. Its output is
// never written to a persistent file.
type LiveTranscoder struct {
	serverDone     <-chan struct{}
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

func NewLiveTranscoder(serverDone <-chan struct{}) *LiveTranscoder {
	if serverDone == nil {
		serverDone = make(chan struct{})
	}
	return &LiveTranscoder{
		serverDone:     serverDone,
		commandContext: normalCommand,
	}
}

// Start shares one deadline across probing and copy/encode fallbacks; stale saved probes may be
// refreshed once after empty output, and hardware failures fall back to software. Copy seeks beyond
// CopySeekAllowance re-encode at the requested position; successful calls require exactly one Wait.
func (t *LiveTranscoder) Start(
	requestContext context.Context,
	request domain.LiveTranscodeRequest,
) (domain.LiveTranscode, error) {
	ctx, cancel := context.WithCancel(requestContext)
	watchDone := make(chan struct{})
	go func() {
		select {
		case <-t.serverDone:
			cancel()
		case <-watchDone:
		}
	}()
	cleanup := sync.OnceFunc(func() {
		close(watchDone)
		cancel()
	})

	metadata := request.Probe
	var probed *domain.TranscodeProbe
	if metadata == nil {
		fresh, err := t.probeSource(ctx, request)
		if err != nil {
			cleanup()
			return domain.LiveTranscode{}, err
		}
		metadata, probed = &fresh.probe, fresh.saved
	}

	for {
		started, err := t.startWithProbe(ctx, request, *metadata)
		if err == nil {
			var once sync.Once
			var waitErr error
			wait := func() error {
				once.Do(func() {
					waitErr = started.process.wait()
					cleanup()
				})
				return waitErr
			}
			return domain.LiveTranscode{
				Stream: &prefixedReadCloser{
					Reader: io.MultiReader(bytes.NewReader(started.first), started.process.stdout),
					closer: started.process.stdout,
				},
				Wait:            wait,
				Stop:            sync.OnceFunc(cancel),
				StartMs:         started.startMs,
				Probed:          probed,
				VideoEncoder:    started.encoder,
				HardwareFailure: started.hardwareFailure,
			}, nil
		}

		// Refresh saved probes only on empty output; cancellation and deadline errors must propagate
		// immediately.
		if ctx.Err() != nil || !errors.Is(err, errNoInitialData) || metadata != request.Probe {
			cleanup()
			return domain.LiveTranscode{}, fmt.Errorf("live transcode could not produce initial data: %w", err)
		}
		fresh, probeErr := t.probeSource(ctx, request)
		if probeErr != nil {
			cleanup()
			return domain.LiveTranscode{}, fmt.Errorf("transcoding with the stored probe failed and re-probing failed: %w; first transcode: %w", probeErr, err)
		}
		metadata, probed = &fresh.probe, fresh.saved
	}
}

var (
	errNoInitialData  = errors.New("FFmpeg exited before producing any data")
	errCopySeekTooFar = errors.New("the previous keyframe is too far from the requested position")
)

type startedTranscode struct {
	process transcodeProcess
	first   []byte
	startMs int64
	// Empty for stream copy.
	encoder         domain.VideoEncoder
	hardwareFailure error
}

// Try copy unless known keyframe spacing is too long; empty output or excessive seek error falls
// back to encoding.
func (t *LiveTranscoder) startWithProbe(
	ctx context.Context, request domain.LiveTranscodeRequest, metadata domain.TranscodeProbe,
) (startedTranscode, error) {
	if !request.Normalize && request.Quality == "" && videoCanCopy(metadata.Video) &&
		(request.StartMs <= 0 || !request.SparseKeyframes) {
		started, err := t.startAttempt(ctx, request, metadata, "")
		if err == nil {
			return started, nil
		}
		switchable := errors.Is(err, errNoInitialData) || errors.Is(err, errCopySeekTooFar)
		if ctx.Err() != nil || !switchable {
			return startedTranscode{}, err
		}
		copyErr := err
		started, err = t.startEncode(ctx, request, metadata)
		if err != nil {
			// Keep copy failures textual so only the encode failure determines whether to refresh the probe.
			return startedTranscode{}, fmt.Errorf("%w; video copy: %s", err, copyErr.Error())
		}
		return started, nil
	}
	return t.startEncode(ctx, request, metadata)
}

// Hardware empty-output failures retry with software using the same probe and startup deadline;
// cancellation and timeout never trigger fallback.
func (t *LiveTranscoder) startEncode(
	ctx context.Context, request domain.LiveTranscodeRequest, metadata domain.TranscodeProbe,
) (startedTranscode, error) {
	encoder := request.VideoEncoder
	if !slices.Contains(domain.HardwareVideoEncoders, encoder) {
		return t.startAttempt(ctx, request, metadata, domain.VideoEncoderSoftware)
	}
	started, err := t.startAttempt(ctx, request, metadata, encoder)
	if err == nil {
		return started, nil
	}
	if ctx.Err() != nil || !errors.Is(err, errNoInitialData) {
		return startedTranscode{}, err
	}
	hardwareErr := fmt.Errorf("%s video encoder failed before producing data: %w", encoder, err)
	started, err = t.startAttempt(ctx, request, metadata, domain.VideoEncoderSoftware)
	if err != nil {
		// Keep hardware failures textual so only software failure controls probe refresh.
		return startedTranscode{}, fmt.Errorf("%w; %s", err, hardwareErr.Error())
	}
	started.hardwareFailure = hardwareErr
	return started, nil
}

// An empty encoder means stream copy; midstream copies read and rebase moov edit lists before
// returning initial data.
func (t *LiveTranscoder) startAttempt(
	ctx context.Context, request domain.LiveTranscodeRequest, metadata domain.TranscodeProbe, encoder domain.VideoEncoder,
) (startedTranscode, error) {
	copyVideo := encoder == ""
	args := buildTranscodeArgs(request.Path, request.StartMs, metadata, request.Normalize, copyVideo, encoder, request.Quality)
	process, err := t.startProcess(ctx, args)
	if err != nil {
		return startedTranscode{}, err
	}
	seekCopy := copyVideo && request.StartMs > 0
	read := readAtLeastOneByte
	if seekCopy {
		read = readInitSegment
	}
	first, readErr := readFirstOutput(ctx, process.stdout, time.Until(request.StartupDeadline), read)
	startMs := request.StartMs
	if copyVideo && !seekCopy {
		startMs = 0
	}
	if readErr == nil && seekCopy {
		first, startMs, readErr = rebaseEditLists(first)
		if readErr == nil && !domain.CopySeekWithinAllowance(request.StartMs, startMs) {
			readErr = fmt.Errorf("%w: requested %d ms, actual start %d ms", errCopySeekTooFar, request.StartMs, startMs)
		}
	}
	if readErr == nil && len(first) > 0 {
		return startedTranscode{process: process, first: first, startMs: startMs, encoder: encoder}, nil
	}

	process.stop()
	closeErr := process.stdout.Close()
	waitErr := process.wait()
	if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) || errors.Is(readErr, errInvalidInitSegment) {
		readErr = fmt.Errorf("%w: %w", errNoInitialData, readErr)
	}
	return startedTranscode{}, errors.Join(readErr, closeErr, waitErr)
}

// One running FFmpeg process.
type transcodeProcess struct {
	stdout io.ReadCloser
	wait   func() error
	stop   func()
}

// Each process has its own context so stop affects only that attempt.
func (t *LiveTranscoder) startProcess(ctx context.Context, args []string) (transcodeProcess, error) {
	processCtx, cancel := context.WithCancel(ctx)
	cmd := t.commandContext(processCtx, transcodeCommand, args...)
	cmd.WaitDelay = transcodeStopDelay
	stderr := &tailWriter{limit: stderrTailLimit}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return transcodeProcess{}, fmt.Errorf("cannot open the FFmpeg output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdout.Close()
		return transcodeProcess{}, fmt.Errorf("cannot start FFmpeg: %w", err)
	}
	wait := func() error {
		err := cmd.Wait()
		cancel()
		if err != nil {
			return fmt.Errorf("FFmpeg exited: %w; stderr: %s", err, stderr.String())
		}
		return nil
	}
	return transcodeProcess{stdout: stdout, wait: wait, stop: cancel}, nil
}

type firstOutput struct {
	data []byte
	err  error
}

// Stopping the process and closing output releases an in-progress reader after cancellation or
// timeout.
func readFirstOutput(
	ctx context.Context, stdout io.Reader, timeout time.Duration, read func(io.Reader) ([]byte, error),
) ([]byte, error) {
	result := make(chan firstOutput, 1)
	go func() {
		data, err := read(stdout)
		result <- firstOutput{data: data, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case read := <-result:
		return read.data, read.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("timed out after %s waiting for initial data", timeout)
	}
}

func readAtLeastOneByte(stdout io.Reader) ([]byte, error) {
	buffer := make([]byte, 32*1024)
	length, err := io.ReadAtLeast(stdout, buffer, 1)
	return buffer[:length], err
}

type prefixedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *prefixedReadCloser) Close() error { return r.closer.Close() }

type sourceProbe struct {
	probe domain.TranscodeProbe
	// Nil when the source changed after probing.
	saved *domain.TranscodeProbe
}

// Use fresh probe results for this conversion but do not persist them if size/mtime differs from
// the opened source.
func (t *LiveTranscoder) probeSource(ctx context.Context, request domain.LiveTranscodeRequest) (sourceProbe, error) {
	metadata, err := t.probe(ctx, request.Path, request.StartupDeadline)
	if err != nil {
		return sourceProbe{}, err
	}
	result := sourceProbe{probe: metadata}
	if info, err := os.Stat(request.Path); err == nil && domain.FileStampOf(info) == request.Source {
		saved := metadata
		result.saved = &saved
	}
	return result, nil
}

// Share the startup deadline; cancellation and timeout are not media-specific failures.
func (t *LiveTranscoder) probe(ctx context.Context, path string, startupDeadline time.Time) (domain.TranscodeProbe, error) {
	probeCtx, cancel := context.WithDeadline(ctx, startupDeadline)
	defer cancel()

	output, err := t.commandContext(probeCtx, probeCommand, probeArgs(path)...).Output()
	if err != nil {
		if contextErr := probeCtx.Err(); contextErr != nil {
			return domain.TranscodeProbe{}, fmt.Errorf("on-demand ffprobe did not finish in time: %w", contextErr)
		}
		return domain.TranscodeProbe{}, fmt.Errorf("%w: on-demand ffprobe failed: %w", domain.ErrUnprocessableMedia, err)
	}
	metadata, err := parseTranscodeProbe(output)
	if err != nil {
		return domain.TranscodeProbe{}, fmt.Errorf("%w: %w", domain.ErrUnprocessableMedia, err)
	}
	return metadata, nil
}

// Reuse ingestion's parser; nonpositive duration or absent usable video dimensions prevents
// conversion.
func parseTranscodeProbe(output []byte) (domain.TranscodeProbe, error) {
	probe, err := parseProbeOutput(output)
	if err != nil {
		return domain.TranscodeProbe{}, err
	}
	if probe.DurationMs <= 0 {
		return domain.TranscodeProbe{}, fmt.Errorf("the video has no duration")
	}
	if probe.Transcode == nil {
		return domain.TranscodeProbe{}, fmt.Errorf("no non-attached video stream with dimensions")
	}
	return *probe.Transcode, nil
}

func transcodeArgs(path string, startMs int64, metadata domain.TranscodeProbe, normalize bool) []string {
	return buildTranscodeArgs(path, startMs, metadata, normalize, !normalize && videoCanCopy(metadata.Video), domain.VideoEncoderSoftware, "")
}

// Midstream video encoding also encodes audio so input seeking cannot leave copied audio ahead of
// video; copy seeks use edit-list timestamps and noaccurate_seek to preserve A/V alignment.
// Explicit quality forces both streams to encode with its bitrate limits.
func buildTranscodeArgs(
	path string, startMs int64, metadata domain.TranscodeProbe, normalize, copyVideo bool,
	encoder domain.VideoEncoder, quality domain.TranscodeQuality,
) []string {
	limits, hasQuality := quality.Limits()
	copyVideo = copyVideo && !normalize && !hasQuality && videoCanCopy(metadata.Video)
	seekCopy := copyVideo && startMs > 0
	args := []string{"-hide_banner", "-loglevel", "warning"}
	if !copyVideo {
		args = append(args, hardwareDeviceArgs(encoder)...)
	}
	appendInput := func(disableStream string) {
		if startMs > 0 {
			if seekCopy {
				args = append(args, "-noaccurate_seek")
			}
			args = append(args, "-ss", formatSeconds(startMs))
		}
		if disableStream != "" {
			args = append(args, disableStream)
		}
		args = append(args, "-i", path)
	}

	if metadata.Audio != nil && usesMOVDemuxer(metadata.FormatName) {
		// Separate MOV audio/video demuxers avoid repeated inter-track seeks on network drives.
		appendInput("-an")
		appendInput("-vn")
		args = append(args,
			"-map", fmt.Sprintf("0:%d", metadata.Video.Index),
			"-map", fmt.Sprintf("1:%d", metadata.Audio.Index),
		)
	} else {
		appendInput("")
		args = append(args, "-map", fmt.Sprintf("0:%d", metadata.Video.Index))
		if metadata.Audio != nil {
			args = append(args, "-map", fmt.Sprintf("0:%d", metadata.Audio.Index))
		}
	}

	if copyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, videoEncodeArgs(metadata.Video, encoder, quality)...)
	}

	if metadata.Audio != nil {
		encodeAudio := normalize || !audioCanCopy(*metadata.Audio) || (startMs > 0 && !copyVideo)
		if hasQuality {
			args = append(args, "-c:a", "aac", "-profile:a", "aac_low", "-ac", "2", "-b:a", kbps(limits.AudioKbps), "-ar", "48000")
		} else if encodeAudio {
			args = append(args, "-c:a", "aac", "-profile:a", "aac_low", "-ac", "2", "-b:a", "192k", "-ar", "48000")
		} else {
			args = append(args, "-c:a", "copy")
		}
	}

	movflags := "frag_keyframe+empty_moov+default_base_moof"
	if seekCopy {
		args = append(args, "-copyts", "-start_at_zero")
		movflags += "+delay_moov"
	}
	return append(args,
		"-movflags", movflags,
		"-f", "mp4", "pipe:1",
	)
}

func usesMOVDemuxer(formatName string) bool {
	for format := range strings.SplitSeq(strings.ToLower(formatName), ",") {
		if strings.TrimSpace(format) == "mov" {
			return true
		}
	}
	return false
}

func videoCanCopy(stream domain.TranscodeVideo) bool {
	profiles := map[string]bool{
		"baseline": true, "constrained baseline": true, "main": true, "high": true,
	}
	if stream.CodecName != "h264" || !profiles[strings.ToLower(stream.Profile)] ||
		stream.Level <= 0 || stream.Level > 51 || stream.PixelFormat != "yuv420p" ||
		stream.BitsPerRawSample != 8 || !constantFrameRate(stream) {
		return false
	}
	if exceedsVideoBounds(stream.Width, stream.Height) {
		return false
	}
	displayWidth, displayHeight, _, _ := displayGeometry(stream)
	width, height := outputDimensions(displayWidth, displayHeight)
	return width == displayWidth && height == displayHeight && displayWidth%2 == 0 && displayHeight%2 == 0 &&
		stream.FPS <= maxOutputFPS(width, height)+0.0001
}

func audioCanCopy(stream domain.TranscodeAudio) bool {
	return stream.CodecName == "aac" && strings.EqualFold(stream.Profile, "LC") &&
		stream.Channels >= 1 && stream.Channels <= 2 &&
		stream.SampleRate >= 8000 && stream.SampleRate <= 48000
}

// Only VAAPI requires opening a device before input.
func hardwareDeviceArgs(encoder domain.VideoEncoder) []string {
	if encoder == domain.VideoEncoderVAAPI {
		return []string{"-vaapi_device", vaapiDevice}
	}
	return nil
}

// Keep H.264 High/5.1, eight-bit 4:2:0, and forced IDR across encoders; unknown encoders use
// software. With explicit quality, software/NVENC cap constant quality while QSV/VAAPI/VideoToolbox
// use bounded VBR.
func encoderCodecArgs(encoder domain.VideoEncoder, quality domain.TranscodeQuality) []string {
	limits, hasQuality := quality.Limits()
	var capped []string
	if hasQuality {
		capped = []string{"-maxrate", kbps(limits.VideoKbps), "-bufsize", kbps(limits.BufferKbps)}
	}
	// Explicit quality switches these encoders from constant quality to bounded VBR.
	variable := func(constantQuality ...string) []string {
		if !hasQuality {
			return constantQuality
		}
		return append([]string{"-b:v", kbps(limits.VideoKbps)}, capped...)
	}
	switch encoder {
	case domain.VideoEncoderNVENC:
		args := []string{"-c:v", "h264_nvenc", "-profile:v", "high", "-level:v", "5.1", "-pix_fmt", "yuv420p",
			"-preset", "p4", "-rc", "vbr", "-cq", "23", "-b:v", "0", "-forced-idr", "1"}
		return append(args, capped...)
	case domain.VideoEncoderQSV:
		args := []string{"-c:v", "h264_qsv", "-profile:v", "high", "-level", "51", "-pix_fmt", "nv12", "-preset", "veryfast"}
		args = append(args, variable("-global_quality", "23")...)
		return append(args, "-look_ahead", "0", "-forced_idr", "1")
	case domain.VideoEncoderVAAPI:
		args := []string{"-c:v", "h264_vaapi", "-profile:v", "high", "-level", "5.1"}
		if hasQuality {
			return append(append(args, "-rc_mode", "VBR"), variable()...)
		}
		return append(args, "-rc_mode", "CQP", "-qp", "23")
	case domain.VideoEncoderVideoToolbox:
		args := []string{"-c:v", "h264_videotoolbox", "-profile:v", "high", "-level:v", "5.1", "-pix_fmt", "yuv420p"}
		args = append(args, variable("-q:v", "60")...)
		return append(args, "-realtime", "1")
	default:
		args := []string{"-c:v", "libx264", "-profile:v", "high", "-level:v", "5.1", "-pix_fmt", "yuv420p", "-preset", liveX264Preset, "-crf", "23"}
		return append(args, capped...)
	}
}

func kbps(value int) string {
	return strconv.Itoa(value) + "k"
}

// Share scaling, padding, SAR, FPS, and keyframe rules across encoders; explicit quality
// additionally bounds dimensions and bitrate.
func videoEncodeArgs(stream domain.TranscodeVideo, encoder domain.VideoEncoder, quality domain.TranscodeQuality) []string {
	displayWidth, displayHeight, sampleAspectNum, sampleAspectDen := displayGeometry(stream)
	width, height := outputDimensions(displayWidth, displayHeight)
	scaled := exceedsVideoBounds(displayWidth, displayHeight)
	if limits, ok := quality.Limits(); ok && limits.ShortSide < min(displayWidth, displayHeight) {
		width, height = qualityDimensions(displayWidth, displayHeight, limits.ShortSide)
		scaled = true
	}
	filters := make([]string, 0, 2)
	dimensionsChanged := width != displayWidth || height != displayHeight
	if dimensionsChanged {
		if scaled {
			filters = append(filters, fmt.Sprintf("scale=%d:%d", width, height))
		} else {
			filters = append(filters, fmt.Sprintf("pad=%d:%d:0:0", width, height))
		}
	}
	if dimensionsChanged || stream.Rotation != 0 {
		filters = append(filters, fmt.Sprintf(
			"setsar=%s:max=1000000",
			outputSampleAspectRatio(sampleAspectNum, sampleAspectDen, displayWidth, displayHeight, width, height),
		))
	}
	limit := maxOutputFPS(width, height)
	if stream.FPS <= 0 {
		filters = append(filters, "fps=30.000")
	} else if !constantFrameRate(stream) {
		if stream.FPS <= limit {
			filters = append(filters, "fps="+formatExactFPS(stream.FPS))
		} else {
			filters = append(filters, "fps="+formatCappedFPS(limit))
		}
	} else if stream.FPS > limit+0.0001 {
		filters = append(filters, "fps="+formatCappedFPS(limit))
	}

	if encoder == domain.VideoEncoderVAAPI {
		// VAAPI accepts hardware frames only; convert to NV12 before uploading.
		filters = append(filters, "format=nv12", "hwupload")
	}

	args := encoderCodecArgs(encoder, quality)
	// Force keyframes by output time so FPS filtering does not change their spacing.
	args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", liveKeyframeInterval))
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	return args
}

func constantFrameRate(stream domain.TranscodeVideo) bool {
	return stream.FPS > 0 && stream.RealFPS > 0 && math.Abs(stream.FPS-stream.RealFPS) < 0.0001
}

// Apply long/short-side bounds independent of orientation, allowing portrait and landscape 4K
// within Level 5.1.
func exceedsVideoBounds(width, height int) bool {
	return max(width, height) > maxVideoLongSide || min(width, height) > maxVideoShortSide
}

func outputDimensions(width, height int) (int, int) {
	longSide, shortSide := max(width, height), min(width, height)
	ratio := math.Min(1, math.Min(float64(maxVideoLongSide)/float64(longSide), float64(maxVideoShortSide)/float64(shortSide)))
	if ratio < 1 {
		width = max(2, int(math.Floor(float64(width)*ratio))&^1)
		height = max(2, int(math.Floor(float64(height)*ratio))&^1)
		return width, height
	}
	return width + width%2, height + height%2
}

// Scale to even dimensions while respecting both requested short side and global bounds; very
// narrow videos may need a smaller short side.
func qualityDimensions(width, height, shortSide int) (int, int) {
	longSide, videoShortSide := float64(max(width, height)), float64(min(width, height))
	ratio := math.Min(float64(shortSide)/videoShortSide,
		math.Min(maxVideoLongSide/longSide, maxVideoShortSide/videoShortSide))
	even := func(side int) int {
		return max(2, 2*int(math.Round(float64(side)*ratio/2)))
	}
	return even(width), even(height)
}

func maxOutputFPS(width, height int) float64 {
	macroblocks := math.Ceil(float64(width)/16) * math.Ceil(float64(height)/16)
	return math.Min(maxVideoFPS, float64(maxMacroblocksSec)/macroblocks)
}

func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func parseFrameRate(value string) float64 {
	numerator, denominator, ok := strings.Cut(value, "/")
	if !ok {
		parsed, _ := strconv.ParseFloat(value, 64)
		if parsed > 0 && !math.IsInf(parsed, 0) && !math.IsNaN(parsed) {
			return parsed
		}
		return 0
	}
	n, nErr := strconv.ParseFloat(numerator, 64)
	d, dErr := strconv.ParseFloat(denominator, 64)
	if nErr != nil || dErr != nil || n <= 0 || d <= 0 {
		return 0
	}
	return n / d
}

func parseAspectRatio(value string) (int64, int64) {
	numerator, denominator, ok := strings.Cut(value, ":")
	if !ok {
		numerator, denominator, ok = strings.Cut(value, "/")
	}
	if !ok {
		return 1, 1
	}
	n, nErr := strconv.ParseInt(numerator, 10, 64)
	d, dErr := strconv.ParseInt(denominator, 10, 64)
	if nErr != nil || dErr != nil || n <= 0 || d <= 0 {
		return 1, 1
	}
	return n, d
}

func displayGeometry(stream domain.TranscodeVideo) (int, int, int64, int64) {
	width, height := stream.Width, stream.Height
	numerator, denominator := stream.SampleAspectNum, stream.SampleAspectDen
	if numerator <= 0 || denominator <= 0 {
		numerator, denominator = 1, 1
	}
	if stream.Rotation == 90 || stream.Rotation == 270 {
		width, height = height, width
		numerator, denominator = denominator, numerator
	}
	return width, height, numerator, denominator
}

func outputSampleAspectRatio(numerator, denominator int64, sourceWidth, sourceHeight, width, height int) string {
	return fmt.Sprintf(
		"%d/%d*%d/%d*%d/%d",
		numerator,
		denominator,
		sourceWidth,
		sourceHeight,
		height,
		width,
	)
}

// Prefer Display Matrix rotation over the legacy rotate tag.
func streamRotation(tag string, sideDataList []probeSideData) int {
	for _, sideData := range sideDataList {
		if strings.EqualFold(sideData.SideDataType, "Display Matrix") {
			return normalizeRotation(sideData.Rotation)
		}
	}
	return parseRotation(tag)
}

func parseRotation(value string) int {
	rotation, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return normalizeRotation(rotation)
}

func normalizeRotation(value float64) int {
	quarterTurns := math.Round(value / 90)
	if math.Abs(value-quarterTurns*90) > 0.01 {
		return 0
	}
	rotation := int(quarterTurns*90) % 360
	if rotation < 0 {
		rotation += 360
	}
	return rotation
}

func formatSeconds(milliseconds int64) string {
	return strconv.FormatFloat(float64(milliseconds)/1000, 'f', 3, 64)
}

func formatCappedFPS(fps float64) string {
	// Floor to milli-FPS so rounding cannot exceed the macroblock-rate limit.
	return strconv.FormatFloat(math.Floor(fps*1000)/1000, 'f', 3, 64)
}

func formatExactFPS(fps float64) string {
	// Preserve positive low VFR rates instead of rounding them to zero.
	return strconv.FormatFloat(fps, 'f', -1, 64)
}

type tailWriter struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	if len(w.data) > w.limit {
		w.data = append([]byte(nil), w.data[len(w.data)-w.limit:]...)
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(string(w.data))
}
