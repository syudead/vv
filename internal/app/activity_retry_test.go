package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 走査を始めたものは、手動の走査なら所有者、監視の走査と起動時の再開なら規則で、走査と閉じる記録に渡る
// （specs/055-activity/research.md R-5）。
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

// 失敗した走査の記録のやり直しは手動の走査を始め、走査中なら domain.ErrScanRunning を返す。
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

// fakeImportRetryStore は受け取った内容の鍵を残す。
type fakeImportRetryStore struct{ keys []string }

func (f *fakeImportRetryStore) RetryImport(_ context.Context, contentKey string) error {
	f.keys = append(f.keys, contentKey)
	return nil
}

// import の記録のやり直しは、対象の内容の鍵の動画の失敗した段階を積み直す。
func TestImportRetrierRetriesTheSubjectVideo(t *testing.T) {
	store := &fakeImportRetryStore{}
	entry := domain.ActivityEntry{Kind: domain.ActivityImport, Subject: domain.ActivitySubject{Key: "key-a"}}
	if err := NewImportRetrier(store).RetryActivity(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.keys, []string{"key-a"}) {
		t.Errorf("retried keys = %v, want [key-a]", store.keys)
	}
}
