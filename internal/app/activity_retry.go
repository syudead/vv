package app

import (
	"context"
	"errors"

	"github.com/syudead/vv/internal/domain"
)

type ImportRetryStore interface {
	// RetryImport requeues failed stages into a new owner-initiated activity; reject records no longer
	// retryable, missing videos, or empty work.
	RetryImport(ctx context.Context, entryID int64, contentKey string) error
}

// Committed JobsQueued events wake the appropriate workers after retrying failed stages.
type ImportRetrier struct {
	store ImportRetryStore
}

func NewImportRetrier(store ImportRetryStore) *ImportRetrier {
	return &ImportRetrier{store: store}
}

func (r *ImportRetrier) RetryActivity(ctx context.Context, entry domain.ActivityEntry) error {
	return r.store.RetryImport(ctx, entry.ID, entry.Subject.Key)
}

// Scan retries start a full manual scan; reject an active scan or missing folders, and close the
// failed activity only when the new scan completes successfully.
func (s *Scans) RetryActivity(ctx context.Context, _ domain.ActivityEntry) error {
	_, started, err := s.StartScan(ctx)
	switch {
	case errors.Is(err, domain.ErrNoMediaFolders):
		return domain.ErrActivityNotRetryable
	case err != nil:
		return err
	case !started:
		return domain.ErrScanRunning
	}
	return nil
}
