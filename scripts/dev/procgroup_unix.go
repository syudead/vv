//go:build unix

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// Keep app shutdownGrace < killGrace < drainTimeout so requests can finish before SIGKILL and output can drain afterward.
const killGrace = 15 * time.Second

type processGroup struct{}

// Use a separate process group to terminate grandchildren too, including go run binaries and Vite holding its strict port.
func newProcessGroup(cmd *exec.Cmd) (*processGroup, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &processGroup{}, nil
}

func (*processGroup) attach(*exec.Cmd) error { return nil }

// terminate sends SIGTERM to the group, then SIGKILL after the grace period.
func (*processGroup) terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	group := -cmd.Process.Pid
	if err := syscall.Kill(group, syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		return
	}
	time.AfterFunc(killGrace, func() { _ = syscall.Kill(group, syscall.SIGKILL) })
}
