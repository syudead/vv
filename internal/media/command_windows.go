package media

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// applyProcessAttributes keeps a console child from opening its own console window when the parent is a
// GUI-subsystem process, and creates a low child below normal priority.
func applyProcessAttributes(cmd *exec.Cmd, priority Priority) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
	if priority == PriorityLow {
		cmd.SysProcAttr.CreationFlags |= windows.BELOW_NORMAL_PRIORITY_CLASS
	}
}
