package media

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// applyProcessAttributes keeps a console child from opening its own console
// window when the parent is a GUI-subsystem process (research R-11).
func applyProcessAttributes(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
