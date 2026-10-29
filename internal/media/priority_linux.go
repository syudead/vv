package media

import "golang.org/x/sys/unix"

const (
	lowNice = 10
	// The ioprio_set(2) arguments: IOPRIO_WHO_PROCESS, and the best-effort class (2) at its lowest level, 7.
	ioprioWhoProcess    = 1
	ioprioClassShift    = 13
	ioprioBestEffortLow = 2<<ioprioClassShift | 7
)

// lowerStarted sets a started child's nice value and I/O priority. A failure leaves it at normal priority: it may
// already have exited, or the server may run above nice 10, which an unprivileged process cannot raise a child back to.
func lowerStarted(pid int) {
	_ = unix.Setpriority(unix.PRIO_PROCESS, pid, lowNice)
	_, _, _ = unix.Syscall(unix.SYS_IOPRIO_SET, ioprioWhoProcess, uintptr(pid), ioprioBestEffortLow)
}
