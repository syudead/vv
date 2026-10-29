//go:build !windows

package media

import "os/exec"

// applyProcessAttributes has nothing to set outside Windows; Linux lowers a started child instead.
func applyProcessAttributes(*exec.Cmd, Priority) {}
