package media

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCommandHidesConsoleWindowOnWindows(t *testing.T) {
	cmd := command(context.Background(), PriorityNormal, probeCommand, "-version")
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil; want CREATE_NO_WINDOW in CreationFlags")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x; want CREATE_NO_WINDOW (%#x) set", cmd.SysProcAttr.CreationFlags, windows.CREATE_NO_WINDOW)
	}
}

func TestCommandCreatesALowChildBelowNormalPriorityOnWindows(t *testing.T) {
	for _, tc := range []struct {
		priority Priority
		below    bool
	}{{PriorityNormal, false}, {PriorityLow, true}} {
		cmd := command(context.Background(), tc.priority, probeCommand, "-version")
		if got := cmd.SysProcAttr.CreationFlags&windows.BELOW_NORMAL_PRIORITY_CLASS != 0; got != tc.below {
			t.Errorf("priority %d: BELOW_NORMAL_PRIORITY_CLASS set = %v, want %v", tc.priority, got, tc.below)
		}
	}
}
