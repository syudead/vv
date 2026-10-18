package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func TestBusyIgnoresRunningWatchScan(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	if _, _, err := scans.StartWatchScan(context.Background(), []domain.DirtyDirectory{{Path: fixturePath("/media/a")}}); err != nil {
		t.Fatal(err)
	}
	<-scanner.started

	busy, err := scans.Busy(context.Background())
	if err != nil || busy {
		t.Fatalf("Busy = %v, %v; 走っている監視の走査で true になってはいけない", busy, err)
	}
	close(scanner.release)
	store.waitFinished(t)
	scans.Wait()
}

func TestStartScanSupersedesRunningWatchScan(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	if _, _, err := scans.StartWatchScan(context.Background(), []domain.DirtyDirectory{{Path: fixturePath("/media/a")}}); err != nil {
		t.Fatal(err)
	}
	<-scanner.started

	// Clear the one-shot started channel before beginning the manual scan.
	scanner.mu.Lock()
	scanner.started = nil
	scanner.mu.Unlock()
	scan, started, err := scans.StartScan(context.Background())
	if err != nil || !started || scan.Origin != domain.ScanOriginManual {
		t.Fatalf("StartScan = %+v, started %v, err %v; 手動の走査が始まるはず", scan, started, err)
	}
	if closed := store.waitFinished(t); closed.State != domain.ScanDone || closed.Origin != domain.ScanOriginWatch {
		t.Fatalf("先に閉じた走査 = %+v, want done の watch", closed)
	}
	close(scanner.release)
	if closed := store.waitFinished(t); closed.Origin != domain.ScanOriginManual || closed.State != domain.ScanDone {
		t.Fatalf("あとに閉じた走査 = %+v, want done の manual", closed)
	}
	scans.Wait()
}

func TestOnFinishReportsOriginAndState(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{Total: 1, Processed: 1}}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	type finish struct {
		origin domain.ScanOrigin
		state  domain.ScanState
	}
	var mu sync.Mutex
	var got []finish
	scans.OnFinish(func(origin domain.ScanOrigin, state domain.ScanState) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, finish{origin, state})
	})

	if _, _, err := scans.StartWatchScan(context.Background(), []domain.DirtyDirectory{{Path: fixturePath("/media/a")}}); err != nil {
		t.Fatal(err)
	}
	store.waitFinished(t)
	scans.Wait()
	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.waitFinished(t)
	scans.Wait()

	mu.Lock()
	defer mu.Unlock()
	want := []finish{{domain.ScanOriginWatch, domain.ScanDone}, {domain.ScanOriginManual, domain.ScanDone}}
	if !slices.Equal(got, want) {
		t.Fatalf("知らせ = %v, want %v", got, want)
	}
}

func TestOnStartReportsOrigin(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{Total: 1, Processed: 1}}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	var mu sync.Mutex
	var got []domain.ScanOrigin
	scans.OnStart(func(origin domain.ScanOrigin) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, origin)
	})

	if _, _, err := scans.StartWatchScan(context.Background(), []domain.DirtyDirectory{{Path: fixturePath("/media/a")}}); err != nil {
		t.Fatal(err)
	}
	store.waitFinished(t)
	scans.Wait()
	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.waitFinished(t)
	scans.Wait()

	mu.Lock()
	defer mu.Unlock()
	want := []domain.ScanOrigin{domain.ScanOriginWatch, domain.ScanOriginManual}
	if !slices.Equal(got, want) {
		t.Fatalf("知らせ = %v, want %v", got, want)
	}
}

// Completion callbacks must precede the next start callback under the same lock.
func TestOnFinishRunsBeforeNextScanStarts(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{Total: 1, Processed: 1}}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	var mu sync.Mutex
	var events []string
	scans.OnStart(func(origin domain.ScanOrigin) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, "start")
	})
	nextStarted := make(chan struct{})
	var once sync.Once
	scans.OnFinish(func(origin domain.ScanOrigin, state domain.ScanState) {
		once.Do(func() {
			go func() {
				defer close(nextStarted)
				if _, _, err := scans.StartScan(context.Background()); err != nil {
					t.Error(err)
				}
			}()
			// An unsynchronized next scan would publish its start here.
			select {
			case <-nextStarted:
			case <-time.After(200 * time.Millisecond):
			}
		})
		mu.Lock()
		defer mu.Unlock()
		events = append(events, "finish")
	})

	if _, _, err := scans.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.waitFinished(t)
	<-nextStarted
	store.waitFinished(t)
	scans.Wait()

	mu.Lock()
	defer mu.Unlock()
	want := []string{"start", "finish", "start", "finish"}
	if !slices.Equal(events, want) {
		t.Fatalf("知らせの順 = %v, want %v", events, want)
	}
}
