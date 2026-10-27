package media

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A killed child can leave a grandchild holding its output pipes, such as the program a launcher
// started; without a bound, Wait would block until that grandchild exits.
const commandStopDelay = 5 * time.Second

// command builds every ffmpeg/ffprobe child process this package starts, so
// OS-specific process settings are applied in one place. On Windows the child
// gets CREATE_NO_WINDOW; elsewhere nothing is added.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = commandStopDelay
	applyProcessAttributes(cmd)
	return cmd
}

// Command builds a child process with this package's OS-specific settings, for other adapters that start
// helpers, such as the face runtime's.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return command(ctx, name, args...)
}

// ffmpegInvalidData is FFmpeg's message for AVERROR_INVALIDDATA, which reading the same file reproduces every time.
const ffmpegInvalidData = "Invalid data found when processing input"

// exitFailure marks err as repeating when the process's output reports FFmpeg's invalid-data error. An I/O error, a
// permission error or a timeout may pass, so it stays a plain failure.
func exitFailure(err error, output []byte) error {
	if bytes.Contains(output, []byte(ffmpegInvalidData)) {
		return domain.FailureRepeats(err)
	}
	return err
}
