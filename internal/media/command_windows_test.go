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
