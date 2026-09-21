//go:build !unix && !windows

package main

import "os/exec"

type processGroup struct{}

func newProcessGroup(*exec.Cmd) (*processGroup, error) { return &processGroup{}, nil }
func (*processGroup) attach(*exec.Cmd) error           { return nil }

func (*processGroup) terminate(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
