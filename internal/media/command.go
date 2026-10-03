package media

import (
	"context"
	"os/exec"
)

// command builds every ffmpeg/ffprobe child process this package starts, so
// OS-specific process settings are applied in one place. On Windows the child
// gets CREATE_NO_WINDOW; elsewhere nothing is added.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	applyProcessAttributes(cmd)
	return cmd
}
