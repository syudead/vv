package app

import (
	"context"
	"log/slog"
	"sync"
)

// Serialize generation by artifact kind.
type artifactKind int

const (
	artifactThumbnail artifactKind = iota
	artifactSeekThumbnails
	artifactPreview
)

// Per-content shared generation locks and exclusive deletion locks keep files consistent with
// references and completion records; per-kind locks serialize duplicate generation while allowing
// different kinds concurrently. Always acquire kind before content.
type artifacts struct {
	index  ContentIndex
	files  ArtifactRemover
	logger *slog.Logger

	contents keyedLocks[string]
	kinds    keyedLocks[artifactKey]

	// Wait for background deletion during shutdown.
	releasing sync.WaitGroup
}

type artifactKey struct {
	contentKey string
	kind       artifactKind
}

func newArtifacts(index ContentIndex, files ArtifactRemover, logger *slog.Logger) *artifacts {
	return &artifacts{index: index, files: files, logger: logger}
}

// Different kinds may generate concurrently, but none overlap deletion of the same content.
func (a *artifacts) lockGeneration(contentKey string, kind artifactKind) func() {
	unlockKind := a.kinds.lock(artifactKey{contentKey: contentKey, kind: kind})
	unlockContent := a.contents.rlock(contentKey)
	return func() {
		unlockContent()
		unlockKind()
	}
}

// Preserve lock queue order while allowing cancellation; release asynchronously acquired locks
// immediately if the caller has already stopped waiting.
func (a *artifacts) lockGenerationContext(ctx context.Context, contentKey string, kind artifactKind) (func(), error) {
	acquired := make(chan func(), 1)
	go func() {
		acquired <- a.lockGeneration(contentKey, kind)
	}()
	select {
	case unlock := <-acquired:
		if err := ctx.Err(); err != nil {
			unlock()
			return nil, err
		}
		return unlock, nil
	case <-ctx.Done():
		go func() {
			unlock := <-acquired
			unlock()
		}()
		return nil, ctx.Err()
	}
}

// Keep generation and completion under the content lock; requested cleanup runs afterward under an
// exclusive lock so it cannot overlap another kind's completion.
func (a *artifacts) generate(
	ctx context.Context, contentKey string, kind artifactKind, run func() (cleanup bool, err error),
) error {
	return a.runGeneration(ctx, contentKey, a.lockGeneration(contentKey, kind), run)
}

// Cancel lock waits without running generation, preserving request deadlines.
func (a *artifacts) generateContext(
	ctx context.Context, contentKey string, kind artifactKind, run func() (cleanup bool, err error),
) error {
	unlock, err := a.lockGenerationContext(ctx, contentKey, kind)
	if err != nil {
		return err
	}
	return a.runGeneration(ctx, contentKey, unlock, run)
}

func (a *artifacts) runGeneration(
	ctx context.Context, contentKey string, unlock func(), run func() (cleanup bool, err error),
) error {
	cleanup, err := func() (bool, error) {
		defer unlock()
		return run()
	}()
	if cleanup {
		if cleanupErr := a.removeIfUnreferencedAfterGeneration(context.WithoutCancel(ctx), contentKey); cleanupErr != nil {
			return cleanupErr
		}
	}
	return err
}

// Skip the exclusive lock while references remain so unrelated generation need not finish; a later
// deletion event handles references removed afterward.
func (a *artifacts) removeIfUnreferencedAfterGeneration(ctx context.Context, contentKey string) error {
	referenced, err := a.index.ContentKeyReferenced(ctx, contentKey)
	if err != nil || referenced {
		return err
	}
	return a.removeIfUnreferenced(ctx, contentKey)
}

// Caller must hold the content lock exclusively.
func (a *artifacts) removeIfUnreferencedLocked(ctx context.Context, contentKey string) error {
	referenced, err := a.index.ContentKeyReferenced(ctx, contentKey)
	if err != nil {
		return err
	}
	if referenced {
		return nil
	}
	return a.files.RemoveContent(contentKey)
}

func (a *artifacts) removeIfUnreferenced(ctx context.Context, contentKey string) error {
	unlock := a.contents.lock(contentKey)
	defer unlock()
	return a.removeIfUnreferencedLocked(ctx, contentKey)
}

// Delete only the released content's unreferenced artifacts in the background; wait joins cleanup
// during shutdown.
func (a *artifacts) release(contentKeys []string) {
	a.releasing.Add(1)
	go func() {
		defer a.releasing.Done()
		ctx := context.Background()
		for _, key := range contentKeys {
			if err := a.removeIfUnreferenced(ctx, key); err != nil {
				a.logger.Warn("could not delete artifacts of a removed video",
					slog.String("contentKey", key), slog.Any("error", err))
			}
		}
	}()
}

func (a *artifacts) wait() {
	a.releasing.Wait()
}

// Discard unused keyed locks.
type keyedLocks[K comparable] struct {
	mu    sync.Mutex
	locks map[K]*keyedLock
}

type keyedLock struct {
	sync.RWMutex
	users int
}

func (l *keyedLocks[K]) lock(key K) func() {
	entry := l.acquire(key)
	entry.Lock()
	return func() {
		entry.Unlock()
		l.release(key, entry)
	}
}

func (l *keyedLocks[K]) rlock(key K) func() {
	entry := l.acquire(key)
	entry.RLock()
	return func() {
		entry.RUnlock()
		l.release(key, entry)
	}
}

func (l *keyedLocks[K]) acquire(key K) *keyedLock {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locks == nil {
		l.locks = map[K]*keyedLock{}
	}
	entry, ok := l.locks[key]
	if !ok {
		entry = &keyedLock{}
		l.locks[key] = entry
	}
	entry.users++
	return entry
}

func (l *keyedLocks[K]) release(key K, entry *keyedLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.users--
	if entry.users == 0 {
		delete(l.locks, key)
	}
}
