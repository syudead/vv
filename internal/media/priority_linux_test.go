package media

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// processPriority reads a process's nice value from /proc and its I/O priority from ioprio_get(2); pid 0 is this
// process.
func processPriority(t *testing.T, pid int) (nice, ioprio int) {
	t.Helper()
	stat := "/proc/self/stat"
	if pid != 0 {
		stat = "/proc/" + strconv.Itoa(pid) + "/stat"
	}
	data, err := os.ReadFile(stat)
	if err != nil {
		t.Fatal(err)
	}
	// The fields after the parenthesized command name start at field 3; nice is field 19.
	fields := strings.Fields(string(data[strings.LastIndexByte(string(data), ')')+1:]))
	if nice, err = strconv.Atoi(fields[19-3]); err != nil {
		t.Fatal(err)
	}
	value, _, errno := unix.Syscall(unix.SYS_IOPRIO_GET, ioprioWhoProcess, uintptr(pid), 0)
	if errno != 0 {
		t.Fatal(errno)
	}
	return nice, int(value)
}

// startSleeping starts a child that lives until the test ends, so its priority can be read.
func startSleeping(t *testing.T, start func(ctx context.Context) (*exec.Cmd, error)) int {
	t.Helper()
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep is not installed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := start(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

func TestLowCommandRunsAtLowCPUAndIOPriority(t *testing.T) {
	pid := startSleeping(t, func(ctx context.Context) (*exec.Cmd, error) {
		cmd := command(ctx, PriorityLow, "sleep", "60")
		return cmd.Cmd, cmd.Start()
	})
	ownNice, _ := processPriority(t, 0)
	nice, ioprio := processPriority(t, pid)
	// An unprivileged server already above nice 10 cannot raise a child back to 10, so the child keeps the server's nice.
	if want := max(ownNice, lowNice); nice != want {
		t.Errorf("nice = %d, want %d", nice, want)
	}
	if ioprio != ioprioBestEffortLow {
		t.Errorf("ioprio = %#x, want best-effort level 7 (%#x)", ioprio, ioprioBestEffortLow)
	}
}

func TestLiveTranscodeCommandKeepsTheServerPriority(t *testing.T) {
	pid := startSleeping(t, func(ctx context.Context) (*exec.Cmd, error) {
		cmd := NewLiveTranscoder(nil).commandContext(ctx, "sleep", "60")
		return cmd, cmd.Start()
	})
	ownNice, ownIOPrio := processPriority(t, 0)
	nice, ioprio := processPriority(t, pid)
	if nice != ownNice || ioprio != ownIOPrio {
		t.Errorf("nice = %d, ioprio = %#x; want the server's %d and %#x", nice, ioprio, ownNice, ownIOPrio)
	}
}
