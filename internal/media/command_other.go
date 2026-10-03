//go:build !windows

package media

import "os/exec"

// applyProcessAttributes has nothing to set outside Windows.
func applyProcessAttributes(*exec.Cmd) {}
