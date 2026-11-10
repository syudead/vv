package domain

import (
	"errors"
	"fmt"
)

// ErrScanPaused is the cause a pause of background work cancels a running manual scan or force rescan with; the
// scan stops at the next file, removes nothing and closes as ScanPaused. See docs/design-docs/background-pause.md.
var ErrScanPaused = errors.New("background work is paused")

// ErrScanNotPaused refuses a pause or a reopen the scan's state does not allow.
var ErrScanNotPaused = errors.New("the scan cannot be paused or reopened in its state")

// InitialScanState is the state a new scan row starts in. A manual scan or force rescan requested while background
// work is paused waits as ScanPaused; a watch scan is held by automatic import instead, so it always runs.
func InitialScanState(origin ScanOrigin, backgroundPaused bool) ScanState {
	if backgroundPaused && origin == ScanOriginManual {
		return ScanPaused
	}
	return ScanRunning
}

// CheckScanPause allows closing a scan as paused only while it runs.
func CheckScanPause(state ScanState) error {
	if state != ScanRunning {
		return fmt.Errorf("%w: %s", ErrScanNotPaused, state)
	}
	return nil
}

// CheckScanReopen allows reopening only the latest scan, only while it is paused and no scan runs.
func CheckScanReopen(state ScanState, latest, running bool) error {
	if running {
		return ErrScanRunning
	}
	if !latest || state != ScanPaused {
		return fmt.Errorf("%w: %s", ErrScanNotPaused, state)
	}
	return nil
}

// ReopenDropsIssue reports whether reopening the paused scan deletes one of its issues: a scan-sourced issue its scope
// covers, which the resumed pass reports again if the file still fails. Stage issues and issues carried from outside
// a force rescan's scope stay.
func (s Scan) ReopenDropsIssue(videoID int64, path string, kind ScanIssueKind) bool {
	if !kind.FromScan() {
		return false
	}
	return s.Rescan == nil || !s.Rescan.CarriesIssue(videoID, path)
}

// ResumedScanProgress is the progress of a scan whose pass started with base files already completed: a reopened scan
// keeps the completed count from before the pause, and its pass skips those files as unchanged. Failures are the
// pass's own, since it reads a failed file again.
func ResumedScanProgress(base int, pass ScanResult) ScanProgress {
	return ScanProgress{
		Total:     base + pass.Total,
		Completed: base + pass.Completed(),
		Failed:    pass.Failed,
	}
}
