//go:build !linux

package media

// lowerStarted has nothing to do: Windows sets the priority class at creation, and other systems keep the default.
func lowerStarted(int) {}
