package media

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCommandHidesConsoleWindowOnWindows(t *testing.T) {
	cmd := command(context.Background(), probeCommand, "-version")
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil; want CREATE_NO_WINDOW in CreationFlags")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x; want CREATE_NO_WINDOW (%#x) set", cmd.SysProcAttr.CreationFlags, windows.CREATE_NO_WINDOW)
	}
}

// Cancelling a launcher such as a Scoop shim must also stop the program it started.
func TestCommandKillsProcessTreeOnCancelOnWindows(t *testing.T) {
	if cmd := command(context.Background(), probeCommand, "-version"); cmd.Cancel == nil {
		t.Fatal("Cancel is nil; want the process-tree kill")
	}
}
