package opener

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

var ErrUnavailable = errors.New("this environment cannot launch the default app")

type Opener struct {
	// Resolved at startup; empty means unavailable.
	command string
	start   func(name string, args ...string) error
}

func New() *Opener {
	return newOpener(runtime.GOOS, os.Getenv, exec.LookPath, startDetached)
}

// Require the OS opener executable and, on Linux-like hosts, DISPLAY or WAYLAND_DISPLAY so headless
// hosts are unavailable automatically.
func newOpener(
	goos string,
	getenv func(string) string,
	lookPath func(string) (string, error),
	start func(name string, args ...string) error,
) *Opener {
	opener := &Opener{start: start}
	name := commandFor(goos)
	if name == "xdg-open" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return opener
	}
	resolved, err := lookPath(name)
	if err != nil {
		return opener
	}
	opener.command = resolved
	return opener
}

func commandFor(goos string) string {
	switch goos {
	case "windows":
		return "explorer.exe"
	case "darwin":
		return "open"
	default:
		return "xdg-open"
	}
}

func (o *Opener) Available() bool {
	return o != nil && o.command != ""
}

// Empty when unavailable.
func (o *Opener) Command() string {
	if o == nil {
		return ""
	}
	return o.command
}

// Open passes path directly as an argument without a shell and returns after successful process
// launch, without checking whether the application opened.
func (o *Opener) Open(path string) error {
	if !o.Available() {
		return ErrUnavailable
	}
	if err := o.start(o.command, path); err != nil {
		return fmt.Errorf("cannot launch the default app: %w", err)
	}
	return nil
}

// Reap the child asynchronously; callers receive only launch errors.
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
