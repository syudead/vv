//go:build !linux && !darwin

package main

// childrenPeakRSS は Linux と macOS 以外（Windows など）ではピークメモリを測らない。
func childrenPeakRSS() (int64, bool) { return 0, false }
