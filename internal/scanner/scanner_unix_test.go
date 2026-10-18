//go:build unix

package scanner

import (
	"context"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A named pipe discovered during enumeration is unreadable, not changed during import.
func TestScanReportsNonRegularFileAsUnreadable(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a"})
	pipe := filepath.Join(root, "pipe.mp4")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Skipf("named pipes are unavailable: %v", err)
	}
	index := newFakeIndex()
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	result, err := scanner.Scan(context.Background(), domain.ActivityByOwner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Failed != 1 {
		t.Fatalf("result = %+v, want 2 targets with one failure", result)
	}
	want := []domain.ScanFileIssue{{Path: pipe, Kind: domain.IssueUnreadable}}
	if !slices.Equal(index.issues, want) {
		t.Fatalf("報告 = %+v, want %+v", index.issues, want)
	}
}
