package main

import "syscall"

// Linux reports maxrss in KiB.
func childrenPeakRSS() (int64, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage); err != nil {
		return 0, false
	}
	return usage.Maxrss * 1024, true
}
