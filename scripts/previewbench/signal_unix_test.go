//go:build unix

package main

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func TestSignalContextCancelsOnSIGTERM(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	defer stop()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM を送れない: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM でコンテキストがキャンセルされない")
	}
}
