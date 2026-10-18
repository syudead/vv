package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestScansPassTheDeciderOfTheirOrigin(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		start func(*Scans, *fakeScanStore) error
		want  domain.ActivityDecider
	}{
		{"manual", func(scans *Scans, _ *fakeScanStore) error {
			_, _, err := scans.StartScan(ctx)
			return err
		}, domain.ActivityByOwner},
		{"watch", func(scans *Scans, _ *fakeScanStore) error {
			_, _, err := scans.StartWatchScan(ctx, []domain.DirtyDirectory{{Path: fixturePath("/media/a"), Recursive: true}})
			return err
		}, domain.ActivityByRule},
		{"resume", func(scans *Scans, store *fakeScanStore) error {
			store.current = domain.Scan{ID: 4, State: domain.ScanFailed, ErrorCode: domain.ScanErrorInterrupted}
			store.hasScan, store.nextID = true, 4
			_, err := scans.ResumeInterrupted(ctx)
			return err
		}, domain.ActivityByRule},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanner := &fakeScanner{}
			scans, store, _ := newTestScans(t, ctx, scanner)
			if err := tc.start(scans, store); err != nil {
				t.Fatal(err)
			}
			store.waitFinished(t)
			scans.Wait()
			store.mu.Lock()
			finished := slices.Clone(store.finishDeciders)
			store.mu.Unlock()
			if !slices.Equal(finished, []domain.ActivityDecider{tc.want}) {
				t.Errorf("FinishScan deciders = %v, want [%s]", finished, tc.want)
			}
			scanner.mu.Lock()
			scanned := slices.Clone(scanner.deciders)
			scanner.mu.Unlock()
			if !slices.Equal(scanned, []domain.ActivityDecider{tc.want}) {
				t.Errorf("Scan deciders = %v, want [%s]", scanned, tc.want)
			}
		})
	}
}

func TestScansRetryActivityStartsAManualScan(t *testing.T) {
	ctx := context.Background()
	scanner := &fakeScanner{started: make(chan struct{}), release: make(chan struct{})}
	scans, store, _ := newTestScans(t, ctx, scanner)
	entry := domain.ActivityEntry{Kind: domain.ActivityScan, Status: domain.ActivityAttention, Reason: domain.ActivityFailed}

	if err := scans.RetryActivity(ctx, entry); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	if err := scans.RetryActivity(ctx, entry); !errors.Is(err, domain.ErrScanRunning) {
		t.Errorf("retry while scanning = %v, want ErrScanRunning", err)
	}
	close(scanner.release)
	store.waitFinished(t)
	scans.Wait()
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.current.Origin != domain.ScanOriginManual ||
		!slices.Equal(store.finishDeciders, []domain.ActivityDecider{domain.ActivityByOwner}) {
		t.Errorf("scan = %+v, deciders = %v, want one manual scan by owner", store.current, store.finishDeciders)
	}
}

type fakeImportRetryStore struct {
	ids  []int64
	keys []string
}

func (f *fakeImportRetryStore) RetryImport(_ context.Context, entryID int64, contentKey string) error {
	f.ids = append(f.ids, entryID)
	f.keys = append(f.keys, contentKey)
	return nil
}

func TestImportRetrierRetriesTheSubjectVideo(t *testing.T) {
	store := &fakeImportRetryStore{}
	entry := domain.ActivityEntry{ID: 7, Kind: domain.ActivityImport, Subject: domain.ActivitySubject{Key: "key-a"}}
	if err := NewImportRetrier(store).RetryActivity(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.keys, []string{"key-a"}) || !slices.Equal(store.ids, []int64{7}) {
		t.Errorf("retried ids = %v, keys = %v, want [7] and [key-a]", store.ids, store.keys)
	}
}
