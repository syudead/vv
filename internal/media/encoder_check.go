package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	// encoderCheckTimeout は -encoders の読み取りと、エンコーダーごとの確認用のエンコードの
	// 上限時間である（specs/025-hardware-encoding/research.md R-2）。
	encoderCheckTimeout = 10 * time.Second
	// encoderCheckStderrLimit は確認が失敗したときに結果へ残す標準エラーの末尾の長さである。
	encoderCheckStderrLimit = 2 * 1024
)

// encoderCheckSource は確認用のエンコードの入力で、どのビルドにもある lavfi の合成映像である。
var encoderCheckSource = domain.TranscodeVideo{
	Width: 256, Height: 144, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1,
}

// EncoderCheck はハードウェアの方式が使えるかを、FFmpeg の短い実エンコードで確かめる
// （research.md R-2）。app.EncoderChecker を満たす。-encoders は最初の確認で 1 回だけ
// 読み、並行する確認で共有する。
type EncoderCheck struct {
	commandContext func(context.Context, string, ...string) *exec.Cmd
	timeout        time.Duration

	listOnce sync.Once
	listDone chan struct{}
	listed   map[string]bool
	listErr  error
}

// NewEncoderCheck は FFmpeg を使う EncoderCheck を返す。
func NewEncoderCheck() *EncoderCheck {
	return &EncoderCheck{
		commandContext: command,
		timeout:        encoderCheckTimeout,
		listDone:       make(chan struct{}),
	}
}

// CheckEncoder は encoder を確かめる。-encoders に名前が無ければ実行せずに encoder_missing、
// -encoders の読み取りが上限時間を超えたら timed_out を返す。確認用のエンコードが終了
// コード 0 で終われば available、失敗は check_failed（Detail に標準エラーの末尾）、
// 上限時間の超過は timed_out である。ctx の期限と取り消しでも打ち切る。
func (c *EncoderCheck) CheckEncoder(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability {
	name, ok := hardwareEncoderName(encoder)
	if !ok {
		return encoderUnavailable(encoder, domain.EncoderReasonCheckFailed, "not a hardware video encoder")
	}
	listed, err := c.encoders(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return encoderUnavailable(encoder, domain.EncoderReasonTimedOut, err.Error())
		}
		return encoderUnavailable(encoder, domain.EncoderReasonCheckFailed, err.Error())
	}
	if !listed[name] {
		return encoderUnavailable(encoder, domain.EncoderReasonEncoderMissing, name+" is not in ffmpeg -encoders")
	}
	return c.encode(ctx, encoder)
}

// encode は確認用の短いエンコードを 1 本実行する。
func (c *EncoderCheck) encode(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability {
	runCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	args = append(args, hardwareDeviceArgs(encoder)...)
	args = append(args,
		"-f", "lavfi", "-i", "testsrc2=size=256x144:rate=30",
		"-frames:v", "8",
	)
	args = append(args, videoEncodeArgs(encoderCheckSource, encoder, "")...)
	args = append(args, "-f", "null", "-")

	cmd := c.commandContext(runCtx, transcodeCommand, args...)
	cmd.WaitDelay = transcodeStopDelay
	stderr := &tailWriter{limit: encoderCheckStderrLimit}
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderAvailable}
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return encoderUnavailable(encoder, domain.EncoderReasonTimedOut,
			fmt.Sprintf("the check encode did not finish in time; stderr: %s", stderr.String()))
	}
	return encoderUnavailable(encoder, domain.EncoderReasonCheckFailed,
		fmt.Sprintf("the check encode failed: %v; stderr: %s", err, stderr.String()))
}

// encoders は ffmpeg -encoders の名前の集合を返す。読み取りは 1 回だけで、最初の呼び出しが
// 始め、上限時間で打ち切る。待つ側は ctx が先に終われば ctx の誤りを返す。
func (c *EncoderCheck) encoders(ctx context.Context) (map[string]bool, error) {
	c.listOnce.Do(func() {
		// 読み取りは並行する確認で共有するので、最初の呼び出しの取り消しに巻き込まない。
		listCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
		go func() {
			defer cancel()
			defer close(c.listDone)
			c.listed, c.listErr = c.listEncoders(listCtx)
		}()
	})
	select {
	case <-c.listDone:
		return c.listed, c.listErr
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for ffmpeg -encoders: %w", ctx.Err())
	}
}

func (c *EncoderCheck) listEncoders(ctx context.Context) (map[string]bool, error) {
	cmd := c.commandContext(ctx, transcodeCommand, "-hide_banner", "-nostdin", "-encoders")
	cmd.WaitDelay = transcodeStopDelay
	stderr := &tailWriter{limit: encoderCheckStderrLimit}
	cmd.Stderr = stderr
	output, err := cmd.Output()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, fmt.Errorf("ffmpeg -encoders did not finish in time: %w", contextErr)
		}
		return nil, fmt.Errorf("ffmpeg -encoders failed: %w; stderr: %s", err, stderr.String())
	}
	return parseEncoderNames(output), nil
}

// parseEncoderNames は ffmpeg -encoders の各行（" V....D h264_nvenc  説明"）から名前を取る。
func parseEncoderNames(output []byte) map[string]bool {
	names := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && len(fields[0]) == 6 && fields[0] != "------" {
			names[fields[1]] = true
		}
	}
	return names
}

// hardwareEncoderName は方式の FFmpeg のエンコーダー名を返す。
func hardwareEncoderName(encoder domain.VideoEncoder) (string, bool) {
	switch encoder {
	case domain.VideoEncoderNVENC:
		return "h264_nvenc", true
	case domain.VideoEncoderQSV:
		return "h264_qsv", true
	case domain.VideoEncoderVAAPI:
		return "h264_vaapi", true
	case domain.VideoEncoderVideoToolbox:
		return "h264_videotoolbox", true
	default:
		return "", false
	}
}

func encoderUnavailable(
	encoder domain.VideoEncoder, reason domain.EncoderUnavailableReason, detail string,
) domain.EncoderAvailability {
	return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderUnavailable, Reason: reason, Detail: detail}
}
