package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestStartRescanRefusedWhileManualScanRuns(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	running, _, err := scans.StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-scanner.started

	_, err = scans.StartRescan(context.Background(), domain.RescanScope{Kind: domain.RescanLibrary})
	if !errors.Is(err, domain.ErrScanRunning) {
		t.Fatalf("StartRescan err = %v, want ErrScanRunning", err)
	}
	if current, _ := store.CurrentScan(context.Background()); current.ID != running.ID || current.State != domain.ScanRunning {
		t.Fatalf("走っている走査 = %+v, want 手動の走査 %d のまま", current, running.ID)
	}
	close(scanner.release)
	store.waitFinished(t)
	scans.Wait()
	if len(scanner.rescans) != 0 {
		t.Fatalf("Rescan = %+v, want 呼ばれない", scanner.rescans)
	}
}

func TestStartRescanSupersedesWatchScanAndRunsScope(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	var mu sync.Mutex
	var notices []ScanNotice
	record := func(notice ScanNotice) {
		mu.Lock()
		defer mu.Unlock()
		notices = append(notices, notice)
	}
	scans.OnStart(record)
	scans.OnFinish(record)
	if _, _, err := scans.StartWatchScan(context.Background(), []domain.DirtyDirectory{{Path: fixturePath("/media/a")}}); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	scanner.mu.Lock()
	scanner.started = nil
	scanner.mu.Unlock()

	scope := domain.RescanScope{Kind: domain.RescanFolder, FolderID: 2, Path: fixturePath("/media/b")}
	scan, err := scans.StartRescan(context.Background(), scope)
	if err != nil || scan.Rescan == nil || *scan.Rescan != scope {
		t.Fatalf("StartRescan = %+v, %v; want 範囲 %+v の走査", scan, err, scope)
	}
	if closed := store.waitFinished(t); closed.Origin != domain.ScanOriginWatch || closed.State != domain.ScanDone {
		t.Fatalf("先に閉じた走査 = %+v, want 置き換えられた done の watch", closed)
	}
	close(scanner.release)
	if closed := store.waitFinished(t); closed.ID != scan.ID || closed.State != domain.ScanDone {
		t.Fatalf("あとに閉じた走査 = %+v, want done の %d", closed, scan.ID)
	}
	scans.Wait()

	if want := []fakeRescan{{scanID: scan.ID, scope: scope}}; !slices.Equal(scanner.rescans, want) {
		t.Fatalf("Rescan = %+v, want %+v", scanner.rescans, want)
	}
	if got := scanner.deciders[len(scanner.deciders)-1]; got != domain.ActivityByOwner {
		t.Fatalf("強制再スキャンの決め手 = %q, want owner", got)
	}
	// The folder rescan does not read the watch batch or the whole library.
	mu.Lock()
	defer mu.Unlock()
	want := []ScanNotice{
		{Origin: domain.ScanOriginWatch},
		{Origin: domain.ScanOriginWatch, Requeue: true, State: domain.ScanDone},
		{Origin: domain.ScanOriginManual, Partial: true},
		{Origin: domain.ScanOriginManual, Partial: true, State: domain.ScanDone},
	}
	if !slices.Equal(notices, want) {
		t.Fatalf("知らせ = %+v, want %+v", notices, want)
	}
}

func TestResumeInterruptedRescanKeepsScope(t *testing.T) {
	scanner := &fakeScanner{}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	scope := domain.RescanScope{Kind: domain.RescanVideo, VideoID: 9}
	store.current = domain.Scan{ID: 4, State: domain.ScanFailed, ErrorCode: domain.ScanErrorInterrupted, Rescan: &scope}
	store.hasScan, store.nextID = true, 4

	if started, err := scans.ResumeInterrupted(context.Background()); err != nil || !started {
		t.Fatalf("ResumeInterrupted = %v, %v; want 始まる", started, err)
	}
	store.waitFinished(t)
	scans.Wait()

	if want := []fakeRescan{{scanID: 5, scope: scope}}; !slices.Equal(scanner.rescans, want) {
		t.Fatalf("Rescan = %+v, want %+v", scanner.rescans, want)
	}
}
