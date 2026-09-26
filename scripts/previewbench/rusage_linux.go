package main

import "syscall"

// childrenPeakRSS は Linux の maxrss（KiB）をバイトに直して返す。
func childrenPeakRSS() (int64, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage); err != nil {
		return 0, false
	}
	return usage.Maxrss * 1024, true
}
