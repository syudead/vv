package media

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A killed child can leave a grandchild holding its output pipes, such as the program a launcher
// started; without a bound, Wait would block until that grandchild exits.
const commandStopDelay = 5 * time.Second

// Priority is the CPU and I/O priority a child process runs at.
type Priority int

const (
	// PriorityNormal is for a process the owner waits on, such as live transcoding.
	PriorityNormal Priority = iota
	// PriorityLow is for background work, so playback keeps the CPU and the disk; see
	// docs/design-docs/job-scheduling.md.
	PriorityLow
)

// Cmd is an exec.Cmd that applies its priority when it starts. Start, Run, Output and CombinedOutput take the
// priority; use them, not the embedded exec.Cmd's, which would start the process without it.
type Cmd struct {
	*exec.Cmd
	priority Priority
}

// command builds every ffmpeg/ffprobe child process this package starts, so OS-specific process settings are
// applied in one place: on Windows the child gets CREATE_NO_WINDOW and, when low, BELOW_NORMAL_PRIORITY_CLASS; on
// Linux a low child is lowered right after it starts, because Go runs nothing between fork and exec.
func command(ctx context.Context, priority Priority, name string, args ...string) *Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = commandStopDelay
	applyProcessAttributes(cmd, priority)
	return &Cmd{Cmd: cmd, priority: priority}
}

// normalCommand builds a normal-priority child, whose plain exec.Cmd needs nothing after it starts.
func normalCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return command(ctx, PriorityNormal, name, args...).Cmd
}

// Command builds a child process with this package's OS-specific settings and priority, for other adapters that
// start helpers, such as the face runtime's.
func Command(ctx context.Context, priority Priority, name string, args ...string) *Cmd {
	return command(ctx, priority, name, args...)
}

func (c *Cmd) Start() error {
	if err := c.Cmd.Start(); err != nil {
		return err
	}
	if c.priority == PriorityLow {
		lowerStarted(c.Process.Pid)
	}
	return nil
}

func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// commandStderrLimit bounds the standard error Output keeps for an ExitError.
const commandStderrLimit = 64 << 10

// Output runs the process and returns its standard output; like exec.Cmd.Output, an ExitError carries the start of
// standard error when the caller set no Stderr.
func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	c.Stdout = &stdout
	var stderr *headWriter
	if c.Stderr == nil {
		stderr = &headWriter{limit: commandStderrLimit}
		c.Stderr = stderr
	}
	err := c.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && stderr != nil {
		exitErr.Stderr = stderr.bytes()
	}
	return stdout.Bytes(), err
}

// CombinedOutput runs the process and returns its standard output and standard error together.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil || c.Stderr != nil {
		return nil, errors.New("exec: Stdout or Stderr already set")
	}
	// exec.Cmd gives the same writer one pipe, so one goroutine writes to the buffer.
	var output bytes.Buffer
	c.Stdout = &output
	c.Stderr = &output
	err := c.Run()
	return output.Bytes(), err
}

// headWriter keeps the first limit bytes written to it, where FFmpeg reports why it stopped.
type headWriter struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (w *headWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if room := w.limit - len(w.data); room > 0 {
		w.data = append(w.data, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

func (w *headWriter) bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.data
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
