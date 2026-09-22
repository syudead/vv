package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os/exec"
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
	maxVideoWidth      = 3840
	maxVideoHeight     = 2160
	maxVideoFPS        = 60
	maxMacroblocksSec  = 983040
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
	return &LiveTranscoder{serverDone: serverDone, commandContext: exec.CommandContext}
}

// Start returns FFmpeg stdout, a wait function, and an idempotent stop function.
// startupDeadline bounds the request-time probe; the caller uses the same
// deadline while waiting for the first output. The caller must call wait
// exactly once after a successful start.
func (t *LiveTranscoder) Start(
	requestContext context.Context,
	path string,
	startMs int64,
	normalize bool,
	startupDeadline time.Time,
) (io.ReadCloser, func() error, func(), error) {
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

	metadata, err := t.probe(ctx, path, startupDeadline)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}

	cmd := t.commandContext(ctx, transcodeCommand, transcodeArgs(path, startMs, metadata, normalize)...)
	cmd.WaitDelay = transcodeStopDelay
	stderr := &tailWriter{limit: stderrTailLimit}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("FFmpeg の出力を開けません: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cleanup()
		_ = stdout.Close()
		return nil, nil, nil, fmt.Errorf("FFmpeg を開始できません: %w", err)
	}

	var once sync.Once
	var waitErr error
	wait := func() error {
		once.Do(func() {
			waitErr = cmd.Wait()
			cleanup()
			if waitErr != nil {
				waitErr = fmt.Errorf("FFmpeg が終了しました: %w; stderr: %s", waitErr, stderr.String())
			}
		})
		return waitErr
	}
	stop := sync.OnceFunc(cancel)
	return stdout, wait, stop, nil
}

type transcodeStream struct {
	Index            int
	CodecType        string
	CodecName        string
	Profile          string
	PixelFormat      string
	BitsPerRawSample int
	Width            int
	Height           int
	Level            int
	FPS              float64
	RealFPS          float64
	SampleAspectNum  int64
	SampleAspectDen  int64
	Rotation         int
	SampleRate       int
	Channels         int
	AttachedPicture  bool
}

type transcodeMetadata struct {
	FormatName string
	Video      transcodeStream
	Audio      *transcodeStream
}

func (t *LiveTranscoder) probe(ctx context.Context, path string, startupDeadline time.Time) (transcodeMetadata, error) {
	probeCtx, cancel := context.WithDeadline(ctx, startupDeadline)
	defer cancel()

	output, err := t.commandContext(probeCtx, probeCommand, probeArgs(path)...).Output()
	if err != nil {
		if contextErr := probeCtx.Err(); contextErr != nil {
			return transcodeMetadata{}, fmt.Errorf("request時probeが期限内に完了しませんでした: %w", contextErr)
		}
		return transcodeMetadata{}, fmt.Errorf("%w: request時probeに失敗しました: %w", domain.ErrUnprocessableMedia, err)
	}
	metadata, err := parseTranscodeProbe(output)
	if err != nil {
		return transcodeMetadata{}, fmt.Errorf("%w: %w", domain.ErrUnprocessableMedia, err)
	}
	return metadata, nil
}

func parseTranscodeProbe(output []byte) (transcodeMetadata, error) {
	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return transcodeMetadata{}, fmt.Errorf("probe出力をJSONとして読めません: %w", err)
	}
	duration, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil || duration <= 0 || math.IsInf(duration, 0) || math.IsNaN(duration) {
		return transcodeMetadata{}, fmt.Errorf("動画の尺がありません")
	}

	result := transcodeMetadata{FormatName: strings.ToLower(parsed.Format.FormatName)}
	for _, stream := range parsed.Streams {
		sampleAspectNum, sampleAspectDen := parseAspectRatio(stream.SampleAspectRatio)
		rotation := parseRotation(stream.Tags.Rotate)
		for _, sideData := range stream.SideDataList {
			if strings.EqualFold(sideData.SideDataType, "Display Matrix") {
				rotation = normalizeRotation(sideData.Rotation)
				break
			}
		}
		converted := transcodeStream{
			Index:            stream.Index,
			CodecType:        stream.CodecType,
			CodecName:        strings.ToLower(stream.CodecName),
			Profile:          stream.Profile,
			PixelFormat:      strings.ToLower(stream.PixelFormat),
			BitsPerRawSample: parsePositiveInt(stream.BitsPerRawSample),
			Width:            stream.Width,
			Height:           stream.Height,
			Level:            stream.Level,
			FPS:              parseFrameRate(stream.AverageFrameRate),
			RealFPS:          parseFrameRate(stream.RealFrameRate),
			SampleAspectNum:  sampleAspectNum,
			SampleAspectDen:  sampleAspectDen,
			Rotation:         rotation,
			SampleRate:       parsePositiveInt(stream.SampleRate),
			Channels:         stream.Channels,
			AttachedPicture:  stream.Disposition.AttachedPicture != 0,
		}
		switch stream.CodecType {
		case "video":
			if result.Video.CodecType == "" && !converted.AttachedPicture {
				result.Video = converted
			}
		case "audio":
			if result.Audio == nil {
				result.Audio = &converted
			}
		}
	}
	if result.Video.CodecType == "" {
		return transcodeMetadata{}, fmt.Errorf("非添付の映像streamがありません")
	}
	if result.Video.Width <= 0 || result.Video.Height <= 0 {
		return transcodeMetadata{}, fmt.Errorf("映像の寸法がありません")
	}
	return result, nil
}

func transcodeArgs(path string, startMs int64, metadata transcodeMetadata, normalize bool) []string {
	normalize = normalize || startMs > 0
	args := []string{"-hide_banner", "-loglevel", "warning"}
	if startMs > 0 {
		args = append(args, "-ss", formatSeconds(startMs))
	}
	if usesMOVDemuxer(metadata.FormatName) {
		// MOV系をネットワークドライブから読むと、stream間のpacket並べ替えで
		// 小さなseekを繰り返すことがある。demuxerには各trackを順に読ませる。
		args = append(args, "-interleaved_read", "0")
	}
	args = append(args, "-i", path, "-map", fmt.Sprintf("0:%d", metadata.Video.Index))
	if metadata.Audio != nil {
		args = append(args, "-map", fmt.Sprintf("0:%d", metadata.Audio.Index))
	}

	encodeVideo := normalize || !videoCanCopy(metadata.Video)
	if encodeVideo {
		args = append(args, videoEncodeArgs(metadata.Video)...)
	} else {
		args = append(args, "-c:v", "copy")
	}

	if metadata.Audio != nil {
		if normalize || !audioCanCopy(*metadata.Audio) {
			args = append(args, "-c:a", "aac", "-profile:a", "aac_low", "-ac", "2", "-b:a", "192k", "-ar", "48000")
		} else {
			args = append(args, "-c:a", "copy")
		}
	}

	return append(args,
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
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

func videoCanCopy(stream transcodeStream) bool {
	profiles := map[string]bool{
		"baseline": true, "constrained baseline": true, "main": true, "high": true,
	}
	if stream.CodecName != "h264" || !profiles[strings.ToLower(stream.Profile)] ||
		stream.Level <= 0 || stream.Level > 51 || stream.PixelFormat != "yuv420p" ||
		stream.BitsPerRawSample != 8 || !constantFrameRate(stream) {
		return false
	}
	if stream.Width > maxVideoWidth || stream.Height > maxVideoHeight {
		return false
	}
	displayWidth, displayHeight, _, _ := displayGeometry(stream)
	width, height := outputDimensions(displayWidth, displayHeight)
	return width == displayWidth && height == displayHeight && displayWidth%2 == 0 && displayHeight%2 == 0 &&
		stream.FPS <= maxOutputFPS(width, height)+0.0001
}

func audioCanCopy(stream transcodeStream) bool {
	return stream.CodecName == "aac" && strings.EqualFold(stream.Profile, "LC") &&
		stream.Channels >= 1 && stream.Channels <= 2 &&
		stream.SampleRate >= 8000 && stream.SampleRate <= 48000
}

func videoEncodeArgs(stream transcodeStream) []string {
	displayWidth, displayHeight, sampleAspectNum, sampleAspectDen := displayGeometry(stream)
	width, height := outputDimensions(displayWidth, displayHeight)
	filters := make([]string, 0, 2)
	dimensionsChanged := width != displayWidth || height != displayHeight
	if dimensionsChanged {
		if displayWidth > maxVideoWidth || displayHeight > maxVideoHeight {
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

	args := []string{"-c:v", "libx264", "-profile:v", "high", "-level:v", "5.1", "-pix_fmt", "yuv420p", "-preset", liveX264Preset, "-crf", "23"}
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	return args
}

func constantFrameRate(stream transcodeStream) bool {
	return stream.FPS > 0 && stream.RealFPS > 0 && math.Abs(stream.FPS-stream.RealFPS) < 0.0001
}

func outputDimensions(width, height int) (int, int) {
	ratio := math.Min(1, math.Min(float64(maxVideoWidth)/float64(width), float64(maxVideoHeight)/float64(height)))
	if ratio < 1 {
		width = max(2, int(math.Floor(float64(width)*ratio))&^1)
		height = max(2, int(math.Floor(float64(height)*ratio))&^1)
		return width, height
	}
	return width + width%2, height + height%2
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

func displayGeometry(stream transcodeStream) (int, int, int64, int64) {
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
	// 丸めでmacroblock rate上限を越えないよう、ミリfps単位で切り捨てる。
	return strconv.FormatFloat(math.Floor(fps*1000)/1000, 'f', 3, 64)
}

func formatExactFPS(fps float64) string {
	// 低fpsのVFRを0へ丸めず、ffprobeから得た正の値を保つ。
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
