package app

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 走っている監視の走査は、取り込みの途中（Busy）に数えない。
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

// 手動の走査の開始は、走っている監視の走査を止めてから始まる。
func TestStartScanSupersedesRunningWatchScan(t *testing.T) {
	scanner := &fakeScanner{release: make(chan struct{}), started: make(chan struct{})}
	scans, store, _ := newTestScans(t, context.Background(), scanner)
	if _, _, err := scans.StartWatchScan(context.Background(), []domain.DirtyDirectory{{Path: fixturePath("/media/a")}}); err != nil {
		t.Fatal(err)
	}
	<-scanner.started

	// fakeScanner は started を 1 度しか閉じられないので、手動の走査の前に外す。
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

// 走査を閉じるたびに、始めた主体と閉じた状態を知らせる。
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

// 走査を始めるたびに、始めた主体を知らせる。
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

// 閉じる知らせの最中に次の手動の走査は始められず、開始の知らせは閉じる知らせのあとに届く。
// 錠を放してから閉じる知らせを呼ぶと、その隙に次の走査の開始の知らせが先に届く。
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
			// 次の走査が割り込めるなら、ここで開始の知らせが届く。
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
