package app

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Observe the real caller's locks at dependency boundaries. Mutex waits cannot
// be observed by synctest; TryLock checks exclusion without scheduling a rival.
func assertThumbnailLocksHeld(t *testing.T, artifacts *artifacts, key string) {
	t.Helper()
	assertKeyedLockHeld(t, &artifacts.kinds, artifactKey{contentKey: key, kind: artifactThumbnail})
	assertKeyedLockHeld(t, &artifacts.contents, key)
}

func assertKeyedLockHeld[K comparable](t *testing.T, locks *keyedLocks[K], key K) {
	t.Helper()
	locks.mu.Lock()
	entry := locks.locks[key]
	locks.mu.Unlock()
	if entry == nil {
		t.Fatalf("no lock for %v", key)
	}
	if entry.TryLock() {
		entry.Unlock()
		t.Fatalf("lock for %v is not held", key)
	}
}

type thumbnailLockStore struct {
	IngestStore
	check func(string)
}

func (s *thumbnailLockStore) SetThumbnailPosition(ctx context.Context, videoID int64, position *int64) (domain.Video, error) {
	s.check("record-position")
	return s.IngestStore.SetThumbnailPosition(ctx, videoID, position)
}

func (s *thumbnailLockStore) ApplyThumbnailImage(ctx context.Context, videoID int64, key string, image domain.AppliedThumbnailImage) (domain.Video, error) {
	s.check("record-image")
	return s.IngestStore.ApplyThumbnailImage(ctx, videoID, key, image)
}
