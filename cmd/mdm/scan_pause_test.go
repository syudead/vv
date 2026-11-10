package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/scanner"
)

// pausingReads grants content-key reads, and on the read numbered pauseAt starts a pause of background work and holds
// that read until the pause cancels it, so the scan stops between two files.
type pausingReads struct {
	mu      sync.Mutex
	reads   int
	pauseAt int
	pause   func()
}

func (r *pausingReads) AcquireRead(ctx context.Context, _ string) (func(), error) {
	r.mu.Lock()
	r.reads++
	pausing := r.reads == r.pauseAt
	r.mu.Unlock()
	if !pausing {
		return func() {}, nil
	}
	go r.pause()
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*pausingReads) KeepAhead(string) func() { return func() {} }

func (r *pausingReads) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

// A scan paused mid-walk through the real scanner and store closes as paused under its id, and the resume finishes
// the same scan: every file is registered once and completed ends equal to total.
func TestScanPausedMidWalkAndResumedRegistersEveryFileOnce(t *testing.T) {
	ctx, db, mediaDir := openTestDB(t)
	const files = 4
	for i := range files {
		if err := os.WriteFile(filepath.Join(mediaDir, fmt.Sprintf("movie%d.mp4", i)),
			[]byte(fmt.Sprintf("movie %d", i)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.DiscardHandler)
	var scans *app.Scans
	paused := make(chan error, 1)
	reads := &pausingReads{pauseAt: 3, pause: func() {
		if err := db.Settings().SaveBackgroundPaused(ctx, true); err != nil {
			paused <- err
			return
		}
		paused <- scans.PauseBackground(ctx)
	}}
	scans = app.NewScans(app.ScansOptions{
		Store: db.Scans(), Jobs: db.Ingest(), FolderIndex: db.ScanIndex(),
		NewScanner: func(reporter app.ScanReporter) app.Scanner {
			return scanner.New(scanner.Options{
				Index: db.ScanIndex(), Queue: db.Ingest(), Reporter: reporter, ReadSlots: reads, Logger: logger,
			})
		},
		Logger: logger,
	})

	started, _, err := scans.StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-paused; err != nil {
		t.Fatal(err)
	}
	current, err := db.Scans().CurrentScan(ctx)
	if err != nil || current.ID != started.ID || current.State != domain.ScanPaused || current.Completed != 2 {
		t.Fatalf("after the pause = %+v, err %v; want #%d paused with 2 files completed", current, err, started.ID)
	}

	if err := db.Settings().SaveBackgroundPaused(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := scans.ResumeBackground(ctx); err != nil {
		t.Fatal(err)
	}
	scans.Wait()

	current, err = db.Scans().CurrentScan(ctx)
	if err != nil || current.ID != started.ID || current.State != domain.ScanDone ||
		current.Total != files || current.Completed != files || current.Failed != 0 {
		t.Fatalf("after the resume = %+v, err %v; want #%d done with %d of %d", current, err, started.ID, files, files)
	}
	// Two reads before the pause, the held one, and only the two files left after it.
	if got := reads.count(); got != files+1 {
		t.Fatalf("content-key reads = %d, want %d", got, files+1)
	}
	indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, video := range indexed {
		ids[video.ID] = true
	}
	if len(indexed) != files || len(ids) != files {
		t.Fatalf("indexed = %d locations of %d videos, want %d of each", len(indexed), len(ids), files)
	}
}
