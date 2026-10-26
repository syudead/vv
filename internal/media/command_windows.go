package media

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// applyProcessAttributes keeps a console child from opening its own console
// window when the parent is a GUI-subsystem process (research R-11), and has
// cancellation kill the child's process tree.
func applyProcessAttributes(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
	cmd.Cancel = func() error { return killProcessTree(cmd.Process) }
}

// killProcessTree also stops the real program behind a launcher such as a Scoop shim, which
// TerminateProcess on the launcher alone would leave running. The tree must be killed before the
// launcher, whose exit would orphan its children.
func killProcessTree(process *os.Process) error {
	taskkill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(process.Pid))
	taskkill.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	treeErr := taskkill.Run()
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && treeErr != nil {
		return err
	}
	return nil
}
