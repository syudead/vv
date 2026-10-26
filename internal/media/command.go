package media

import (
	"context"
	"os/exec"
	"time"
)

// A killed child can leave a grandchild holding its output pipes, such as the real ffmpeg behind a
// Scoop shim; without a bound, Wait would block until that grandchild exits.
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
