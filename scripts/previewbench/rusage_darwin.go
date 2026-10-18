package main

import "syscall"

// macOS reports maxrss in bytes.
func childrenPeakRSS() (int64, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage); err != nil {
		return 0, false
	}
	return usage.Maxrss, true
}
