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
	// Deadline for encoder discovery and each trial encode.
	encoderCheckTimeout = 10 * time.Second
	// Keep only the stderr tail on failure.
	encoderCheckStderrLimit = 2 * 1024
)

// Use a synthetic lavfi source available in every build.
var encoderCheckSource = domain.TranscodeVideo{
	Width: 256, Height: 144, FPS: 30, RealFPS: 30, SampleAspectNum: 1, SampleAspectDen: 1,
}

// EncoderCheck verifies hardware support with short real encodes and shares one cached -encoders
// result across concurrent checks.
type EncoderCheck struct {
	commandContext func(context.Context, string, ...string) *exec.Cmd
	timeout        time.Duration

	listOnce sync.Once
	listDone chan struct{}
	listed   map[string]bool
	listErr  error
}

func NewEncoderCheck() *EncoderCheck {
	return &EncoderCheck{
		commandContext: normalCommand,
		timeout:        encoderCheckTimeout,
		listDone:       make(chan struct{}),
	}
}

// CheckEncoder distinguishes missing encoders, timeout, and failed trial encodes; failures retain a
// bounded stderr tail, and ctx cancellation stops work.
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

// Runs one short trial encode.
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

// Cache discovery once with its own deadline; waiting callers may cancel independently.
func (c *EncoderCheck) encoders(ctx context.Context) (map[string]bool, error) {
	c.listOnce.Do(func() {
		// Shared discovery must outlive the first caller's cancellation.
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

// Parse names from rows shaped like " V....D h264_nvenc description".
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
