package main

import "syscall"

// childrenPeakRSS は macOS の maxrss（バイト）をそのまま返す。
func childrenPeakRSS() (int64, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage); err != nil {
		return 0, false
	}
	return usage.Maxrss, true
}
