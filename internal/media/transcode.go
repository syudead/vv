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
	// liveKeyframeInterval は映像をエンコードするときのキーフレームの間隔（秒）。
	// 出力は frag_keyframe でキーフレームごとに fragment を区切るので、最初の
	// データはこの間隔分をエンコードし終えるまで出ない。
	liveKeyframeInterval = 2
	// vaapiDevice は VAAPI で使う DRM の render node である（research.md R-7）。
	vaapiDevice = "/dev/dri/renderD128"
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
		commandContext: command,
	}
}

// Start は変換を始め、最初のデータが出たところで返す。request.Probe があれば ffprobe を起動せず、
// 無ければその場で ffprobe を実行して結果を Probed に載せる。
//
// 映像がコピーできる動画（videoCanCopy が真で Normalize でない）は、まず映像をコピーして
// 始める。途中からのコピーは直前のキーフレームから始まるので、出力の moov から実際の
// 開始位置を読み、指定位置との差が domain.CopySeekAllowance を超えたら映像をエンコード
// し直して指定位置から始める。コピーが最初のデータを出さずに終わったときも同じ要求の中で
// エンコードに切り替える。保存済みの解析情報で始めた変換がそれでも最初のデータを出さずに
// 終わったときは、その場の ffprobe を実行し、コピーから 1 回だけやり直す。期限
// （StartupDeadline）は解析と切り替えを含めて 1 つで、期限切れは切り替えずに失敗にする。
//
// 映像をエンコードする段は request.VideoEncoder がハードウェアの方式ならまずそれで始め、
// 最初のデータを出さずに終わったら同じ解析情報で libx264 で始め直す（startEncode）。
// 切り替えたことは戻り値の HardwareFailure に載せる。
// 成功したら Wait をちょうど 1 回呼ぶこと。
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

		// その場の解析に切り替えるのは、保存済みの解析情報で始めたプロセスがデータを
		// 出さずに終わったときだけである。取り消しと期限切れはそのまま失敗にする。
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
	// errNoInitialData はプロセスが最初のデータ（途中からのコピーでは moov まで）を
	// 出さずに終わったことを表す。
	errNoInitialData = errors.New("FFmpeg exited before producing any data")
	// errCopySeekTooFar はコピーの実際の開始位置が指定位置から離れすぎていることを表す。
	errCopySeekTooFar = errors.New("the previous keyframe is too far from the requested position")
)

// startedTranscode は最初のデータを出した FFmpeg 1 本である。
type startedTranscode struct {
	process transcodeProcess
	first   []byte
	startMs int64
	// encoder は映像をエンコードした方式で、コピーでは空である。
	encoder domain.VideoEncoder
	// hardwareFailure はハードウェアから software に切り替えたときの誤りである。
	hardwareFailure error
}

// startWithProbe は 1 つの解析情報で変換を始める。映像をコピーできればコピーを試し、
// 最初のデータを出さずに終わるか差の上限を超えたら、映像をエンコードして始め直す。
func (t *LiveTranscoder) startWithProbe(
	ctx context.Context, request domain.LiveTranscodeRequest, metadata domain.TranscodeProbe,
) (startedTranscode, error) {
	if !request.Normalize && request.Quality == "" && videoCanCopy(metadata.Video) {
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
			// その場の解析へ切り替えるかはエンコードの失敗だけで決めるので、コピーの
			// 誤りは文字列として添える。
			return startedTranscode{}, fmt.Errorf("%w; video copy: %s", err, copyErr.Error())
		}
		return started, nil
	}
	return t.startEncode(ctx, request, metadata)
}

// startEncode は映像をエンコードして始める。要求の方式がハードウェアなら、まずそれで始め、
// 最初のデータを出さずに終わったら同じ解析情報で software で始め直す（research.md R-6）。
// 期限切れと取り消しは切り替えない。期限は Start と共通の StartupDeadline 1 つである。
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
		// その場の解析へ切り替えるかは software の失敗だけで決めるので、ハードウェアの
		// 誤りは文字列として添える。
		return startedTranscode{}, fmt.Errorf("%w; %s", err, hardwareErr.Error())
	}
	started.hardwareFailure = hardwareErr
	return started, nil
}

// startAttempt は FFmpeg を 1 本起動し、最初のデータを待つ。encoder が空なら映像をコピーし、
// そうでなければその方式でエンコードする。途中からのコピーでは moov までを読み、edit list
// から実際の開始位置を得て書き換える（fmp4.go）。
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

// transcodeProcess は起動した FFmpeg 1 本である。
type transcodeProcess struct {
	stdout io.ReadCloser
	wait   func() error
	stop   func()
}

// startProcess は FFmpeg を 1 本起動する。プロセスは自分の context を持ち、stop で
// そのプロセスだけを止める。
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

// readFirstOutput は read で最初のデータを待つ。期限切れと取り消しでは、読みかけの
// goroutine はプロセスを止めて出力を閉じたところで終わる。
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

// prefixedReadCloser は先に読んだ最初のデータに続けて FFmpeg の出力を読ませる。
type prefixedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *prefixedReadCloser) Close() error { return r.closer.Close() }

type sourceProbe struct {
	probe domain.TranscodeProbe
	// saved は保存してよい結果で、解析のあとにファイルが変わっていたら nil。
	saved *domain.TranscodeProbe
}

// probeSource はその場で ffprobe を実行する。解析のあとにファイルの大きさか
// 更新時刻が開いたときと違っていたら、変換には使うが保存用には返さない
// （data-model.md §4）。
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

// probe はその場で ffprobe を実行する。期限は変換の開始と共通で、期限切れと
// 取り消しは動画固有の誤りにしない。
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

// parseTranscodeProbe は要求時の ffprobe の出力を、取り込みと同じ parseProbeOutput で
// 解釈し、ライブ変換に要る値を返す。尺が正でない動画と、使える映像 stream（非添付で
// 寸法がある）の無い動画は変換できない。
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

// transcodeArgs は software の方式で最初に試す引数を返す。映像はコピーできればコピーする。
func transcodeArgs(path string, startMs int64, metadata domain.TranscodeProbe, normalize bool) []string {
	return buildTranscodeArgs(path, startMs, metadata, normalize, !normalize && videoCanCopy(metadata.Video), domain.VideoEncoderSoftware, "")
}

// buildTranscodeArgs は FFmpeg の引数を組み立てる。copyVideo は映像をコピーするかで、
// normalize でなく videoCanCopy が真のときだけ効く。encoder は映像をエンコードするときの
// 方式で、コピーするときは見ない。quality は縮める画質で、空なら元の画質である。
//
// 音声は normalize でなく audioCanCopy が真ならコピーする（親 Issue #371 要件 4）。ただし
// 映像をエンコードして途中から始めるときは、今までどおり音声もエンコードする。入力側の
// -ss は、エンコードする stream では指定位置より前を捨てるが、コピーする stream では
// demuxer が着いたキーフレームからの区間を残すので、音声だけが指定位置より前から始まる。
//
// 途中からのコピーだけ -copyts -start_at_zero と delay_moov を付け、mp4 muxer が各 track の
// 開始時刻を moov の edit list に書くようにする（research.md R-1）。-noaccurate_seek は、
// エンコードする音声もコピーする映像と同じくキーフレームの時刻から始めるためのものである。
//
// 画質があるときは映像も音声も必ずエンコードし、音声はその画質の kbps にする
// （specs/027-playback-quality/research.md R-2）。
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
		// MOVでは映像と音声を別々の入力で読む。各demuxerが一方のtrackだけを
		// 追うため、ネットワークドライブ上でtrack間を往復seekせずに済む。
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

// hardwareDeviceArgs は方式が要る入力より前の大域の指定を返す。VAAPI だけがデバイスを開く。
func hardwareDeviceArgs(encoder domain.VideoEncoder) []string {
	if encoder == domain.VideoEncoderVAAPI {
		return []string{"-vaapi_device", vaapiDevice}
	}
	return nil
}

// encoderCodecArgs は方式ごとの符号化器の指定である（research.md R-7）。どれも H.264 High・
// Level 5.1・4:2:0 8bit で、強制キーフレームを IDR にする。VAAPI の画素形式は
// フィルターの format=nv12,hwupload で与える。知らない方式は software とする。
//
// 画質が無ければ一定品質である。画質があれば、software と NVENC は一定品質に
// -maxrate／-bufsize の上限を重ね、QSV・VAAPI・VideoToolbox は一定品質をやめて上限の
// VBR にする（specs/027-playback-quality/research.md R-2）。
func encoderCodecArgs(encoder domain.VideoEncoder, quality domain.TranscodeQuality) []string {
	limits, hasQuality := quality.Limits()
	var capped []string
	if hasQuality {
		capped = []string{"-maxrate", kbps(limits.VideoKbps), "-bufsize", kbps(limits.BufferKbps)}
	}
	// variable は QSV・VAAPI・VideoToolbox の品質の指定で、画質があれば上限の VBR に替える。
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

// videoEncodeArgs は映像をエンコードする引数を返す。フィルター（縮小・pad・setsar・fps）と
// キーフレームの指定は方式に依らず共通で、出力の約束を守る。符号化器の指定だけを方式で
// 差し替える。画質があれば表示の短辺をその画質へ縮め（qualityDimensions）、符号化器に
// ビットレートの上限を付ける。
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
		// h264_vaapi は hw フレームしか受けないので、8bit 4:2:0 にしてから GPU へ上げる。
		filters = append(filters, "format=nv12", "hwupload")
	}

	args := encoderCodecArgs(encoder, quality)
	// フレーム数ではなく出力の時刻で揃えるため、fps フィルターで間引いたあとも
	// 入力のフレームレートによらず同じ間隔になる。
	args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", liveKeyframeInterval))
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	return args
}

func constantFrameRate(stream domain.TranscodeVideo) bool {
	return stream.FPS > 0 && stream.RealFPS > 0 && math.Abs(stream.FPS-stream.RealFPS) < 0.0001
}

// exceedsVideoBounds は、長辺・短辺のどちらかが上限を超えるかを返す。上限は向きを問わず
// 長辺 maxVideoLongSide・短辺 maxVideoShortSide とし、縦長の 2160x3840 も横長の 3840x2160 と
// 同じく縮めずに通す（どちらも H.264 Level 5.1 の 1 フレームの上限に収まる）。
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

// qualityDimensions は表示の寸法 width×height を、縦横比を保って短辺が shortSide になるよう
// 縮めた偶数の寸法を返す。縮める比は shortSide への比と、今の変換の枠（長辺 maxVideoLongSide・
// 短辺 maxVideoShortSide）への比の小さい方で、極端に細長い動画だけは短辺が shortSide より
// 小さくなる（specs/027-playback-quality/research.md R-2）。shortSide は動画の短辺より小さいこと。
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

// streamRotation は stream の回転（0/90/180/270）を返す。Display Matrix が
// あればそれを、無ければ旧来の rotate タグを採る。
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
