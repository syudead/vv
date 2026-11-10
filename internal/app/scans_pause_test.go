package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A pause stops the running force rescan and closes it as paused without finishing it; the resume reopens the same id
// with the same scope, and the resumed pass counts on top of the files completed before the pause.
func TestPauseClosesTheRunningScanAndResumeReopensIt(t *testing.T) {
	scanner := &fakeScanner{
		result:  domain.ScanResult{Total: 3, Processed: 3},
		release: make(chan struct{}), started: make(chan struct{}),
	}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	scope := domain.RescanScope{Kind: domain.RescanFolder, FolderID: 2, Path: fixturePath("/media/b")}
	scan, err := scans.StartRescan(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	<-scanner.started

	if err := scans.PauseBackground(context.Background()); err != nil {
		t.Fatal(err)
	}
	paused := store.waitPaused(t)
	if paused.ID != scan.ID || paused.Completed != 1 {
		t.Fatalf("paused scan = #%d with %d completed, want #%d with the 1 file read", paused.ID, paused.Completed, scan.ID)
	}
	select {
	case finished := <-store.finished:
		t.Fatalf("the paused scan was finished as %q", finished.State)
	default:
	}

	scanner.release, scanner.started = nil, nil
	if err := scans.ResumeBackground(context.Background()); err != nil {
		t.Fatal(err)
	}
	closed := store.waitFinished(t)
	scans.Wait()
	if closed.ID != scan.ID || closed.State != domain.ScanDone {
		t.Fatalf("closed = #%d %q, want #%d done", closed.ID, closed.State, scan.ID)
	}
	if !slices.Equal(store.reopened, []int64{scan.ID}) {
		t.Fatalf("reopened = %v, want [%d]", store.reopened, scan.ID)
	}
	want := []fakeRescan{{scanID: scan.ID, scope: scope}, {scanID: scan.ID, scope: scope}}
	if !slices.Equal(scanner.rescans, want) {
		t.Fatalf("Rescan = %+v, want %+v", scanner.rescans, want)
	}
	progress := store.progress[scan.ID]
	if got, want := progress[len(progress)-1], (domain.ScanProgress{Total: 4, Completed: 4}); got != want {
		t.Fatalf("final progress = %+v, want %+v", got, want)
	}
}

// During a pause Scan library creates a waiting scan and runs nothing, a second request answers the same scan, and a
// force rescan is refused while it waits.
func TestStartDuringPauseWaitsForTheResume(t *testing.T) {
	scanner := &fakeScanner{}
	store := newFakeScanStore()
	store.backgroundPaused = true
	scans := NewScans(ScansOptions{
		Store: store, Jobs: store, FolderIndex: store,
		NewScanner: func(reporter ScanReporter) Scanner {
			scanner.reporter = reporter
			return scanner
		},
		Logger: discardLogger(), BackgroundPaused: true,
	})

	first, started, err := scans.StartScan(context.Background())
	if err != nil || !started || first.State != domain.ScanPaused {
		t.Fatalf("StartScan = %+v, started %v, err %v; want a new paused scan", first, started, err)
	}
	second, started, err := scans.StartScan(context.Background())
	if err != nil || started || second.ID != first.ID {
		t.Fatalf("second StartScan = #%d, started %v, err %v; want #%d again", second.ID, started, err, first.ID)
	}
	if _, err := scans.StartRescan(context.Background(), domain.RescanScope{Kind: domain.RescanLibrary}); !errors.Is(
		err, domain.ErrScanRunning) {
		t.Fatalf("StartRescan over the paused scan = %v, want ErrScanRunning", err)
	}
	scans.Wait()
	if scanner.runs != 0 {
		t.Fatalf("scanner runs = %d, want none while paused", scanner.runs)
	}
}

// A start whose store read the pause as off before it was saved closes as paused at once instead of running.
func TestStartRacingAPauseClosesAsPaused(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	if err := scans.PauseBackground(context.Background()); err != nil {
		t.Fatal(err)
	}

	scan, _, err := scans.StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if paused := store.waitPaused(t); paused.ID != scan.ID {
		t.Fatalf("paused = #%d, want #%d", paused.ID, scan.ID)
	}
	scans.Wait()
}

// Startup while paused resumes no interrupted scan; the later resume does.
func TestStartupWhilePausedResumesTheInterruptedScanOnlyOnResume(t *testing.T) {
	scanner := &fakeScanner{}
	store := newFakeScanStore()
	store.current = domain.Scan{ID: 4, State: domain.ScanFailed, ErrorCode: domain.ScanErrorInterrupted}
	store.hasScan, store.nextID = true, 4
	scans := NewScans(ScansOptions{
		Store: store, Jobs: store, FolderIndex: store,
		NewScanner: func(reporter ScanReporter) Scanner {
			scanner.reporter = reporter
			return scanner
		},
		Logger: discardLogger(), BackgroundPaused: true,
	})

	if started, err := scans.ResumeInterrupted(context.Background()); err != nil || started {
		t.Fatalf("ResumeInterrupted while paused = %v, %v; want nothing started", started, err)
	}
	if err := scans.ResumeBackground(context.Background()); err != nil {
		t.Fatal(err)
	}
	closed := store.waitFinished(t)
	scans.Wait()
	if closed.ID != 5 || !slices.Equal(store.resumedFrom, []int64{4}) || scanner.runs != 1 {
		t.Fatalf("closed #%d, resumed from %v, runs %d; want #5 resumed from [4] once",
			closed.ID, store.resumedFrom, scanner.runs)
	}
}
