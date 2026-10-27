package media

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestCommandKeepsNameAndArguments(t *testing.T) {
	cmd := command(context.Background(), PriorityNormal, transcodeCommand, "-hide_banner", "-version")
	if got := cmd.Args; !slices.Equal(got, []string{transcodeCommand, "-hide_banner", "-version"}) {
		t.Fatalf("Args = %q", got)
	}
}

// FFmpeg prints why it stopped after its diagnostics, so a long standard error must keep its end as well as its first
// line.
func TestOutputStderrKeepsTheEndOfALongLog(t *testing.T) {
	stderr := &headTailWriter{limit: commandStderrLimit}
	_, _ = stderr.Write([]byte("[mov,mp4] first diagnostic\n"))
	line := []byte(strings.Repeat("x", 1023) + "\n")
	for range 70 {
		_, _ = stderr.Write(line)
	}
	_, _ = stderr.Write([]byte(ffmpegInvalidData + "\n"))

	got := stderr.bytes()
	if first := firstLine(got); first != "[mov,mp4] first diagnostic" {
		t.Errorf("first line = %q", first)
	}
	if len(got) > commandStderrLimit+64 {
		t.Errorf("kept %d bytes, want about %d", len(got), commandStderrLimit)
	}
	if err := exitFailure(errors.New("ffmpeg failed"), got); !errors.Is(err, domain.ErrFailureRepeats) {
		t.Errorf("exitFailure = %v, want a repeating failure", err)
	}
}

// A grandchild that keeps the output pipes open must not block Wait after cancellation.
func TestCommandBoundsWaitForOutputPipes(t *testing.T) {
	cmd := command(context.Background(), PriorityNormal, transcodeCommand, "-version")
	if cmd.WaitDelay != commandStopDelay {
		t.Fatalf("WaitDelay = %v, want %v", cmd.WaitDelay, commandStopDelay)
	}
}
