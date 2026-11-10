package domain

import (
	"errors"
	"testing"
)

// A resumed pass skips the files completed before the pause, so the row's completed count is its base; a failed file
// is read again by the pass and counts once.
func TestResumedScanProgress(t *testing.T) {
	for _, tc := range []struct {
		name string
		base int
		pass ScanResult
		want ScanProgress
	}{
		{"4 before and 6 after", 4, ScanResult{Total: 6, Processed: 6}, ScanProgress{Total: 10, Completed: 10}},
		{"a file added during the pause", 4, ScanResult{Total: 7, Processed: 7}, ScanProgress{Total: 11, Completed: 11}},
		{"a failed file read again and failing", 4, ScanResult{Total: 6, Processed: 5, Failed: 1},
			ScanProgress{Total: 10, Completed: 9, Failed: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResumedScanProgress(tc.base, tc.pass); got != tc.want {
				t.Fatalf("ResumedScanProgress(%d, %+v) = %+v, want %+v", tc.base, tc.pass, got, tc.want)
			}
		})
	}
}

func TestInitialScanState(t *testing.T) {
	for _, tc := range []struct {
		origin ScanOrigin
		paused bool
		want   ScanState
	}{
		{ScanOriginManual, false, ScanRunning},
		{ScanOriginManual, true, ScanPaused},
		{ScanOriginWatch, true, ScanRunning},
	} {
		if got := InitialScanState(tc.origin, tc.paused); got != tc.want {
			t.Errorf("InitialScanState(%s, %v) = %s, want %s", tc.origin, tc.paused, got, tc.want)
		}
	}
}

func TestScanPauseAndReopenRules(t *testing.T) {
	if err := CheckScanPause(ScanRunning); err != nil {
		t.Errorf("CheckScanPause(running) = %v, want nil", err)
	}
	if err := CheckScanPause(ScanDone); !errors.Is(err, ErrScanNotPaused) {
		t.Errorf("CheckScanPause(done) = %v, want ErrScanNotPaused", err)
	}
	if err := CheckScanReopen(ScanPaused, true, false); err != nil {
		t.Errorf("CheckScanReopen(the latest paused) = %v, want nil", err)
	}
	if err := CheckScanReopen(ScanPaused, true, true); !errors.Is(err, ErrScanRunning) {
		t.Errorf("CheckScanReopen with a running scan = %v, want ErrScanRunning", err)
	}
	if err := CheckScanReopen(ScanPaused, false, false); !errors.Is(err, ErrScanNotPaused) {
		t.Errorf("CheckScanReopen(an older paused) = %v, want ErrScanNotPaused", err)
	}
}

// A paused scan with files left keeps them pending in the import tally, as a running one does.
func TestPausedScanTallyKeepsPendingFiles(t *testing.T) {
	tally := Scan{State: ScanPaused, Total: 10, Completed: 4}.Tally(0, 0)
	if got := tally.Status(); got != ImportRunning {
		t.Fatalf("Status = %s, want running", got)
	}
	if got := tally.VideosTotal(); got != 6 {
		t.Fatalf("VideosTotal = %d, want the 6 files left", got)
	}
}
