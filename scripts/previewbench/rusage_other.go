//go:build !linux && !darwin

package main

// Peak RSS is unavailable outside Linux and macOS.
func childrenPeakRSS() (int64, bool) { return 0, false }
