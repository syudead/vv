package app

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func assertArtifactLockMode[K comparable](t *testing.T, locks *keyedLocks[K], key K, shared bool) {
	t.Helper()
	entry := assertKeyedLockHeld(t, locks, key)
	readable := entry.TryRLock()
	if readable {
		entry.RUnlock()
	}
	if readable != shared {
		t.Fatalf("lock for %v: shared = %v, want %v", key, readable, shared)
	}
}

func assertGenerationLockModes(t *testing.T, a *artifacts, key string, kind artifactKind) {
	t.Helper()
	assertArtifactLockMode(t, &a.kinds, artifactKey{contentKey: key, kind: kind}, false)
	assertArtifactLockMode(t, &a.contents, key, true)
}

type artifactContractStore struct {
	IngestStore
	record    func(artifactKind, string)
	reference func(context.Context, string) (bool, error)
}

func (s *artifactContractStore) ContentKeyReferenced(ctx context.Context, key string) (bool, error) {
	if s.reference != nil {
		return s.reference(ctx, key)
	}
	return s.IngestStore.ContentKeyReferenced(ctx, key)
}

func (s *artifactContractStore) SetThumbnailStateForJob(ctx context.Context, job domain.Job,
	state domain.ThumbnailState, substitution domain.Substitution,
) (bool, error) {
	s.record(artifactThumbnail, job.ContentKey)
	return s.IngestStore.SetThumbnailStateForJob(ctx, job, state, substitution)
}

func (s *artifactContractStore) SetSeekThumbnailStateForJob(ctx context.Context, job domain.Job,
	state domain.SeekThumbnailState, substitution domain.Substitution,
) (bool, error) {
	s.record(artifactSeekThumbnails, job.ContentKey)
	return s.IngestStore.SetSeekThumbnailStateForJob(ctx, job, state, substitution)
}

func (s *artifactContractStore) CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error) {
	s.record(artifactPreview, job.ContentKey)
	return s.IngestStore.CompletePreviewForContent(ctx, job)
}

type artifactContractFiles struct {
	ArtifactStore
	remove func(string) error
}

func (f artifactContractFiles) RemoveContent(key string) error { return f.remove(key) }

// Each caller holds a kind-specific exclusive lock and a shared content lock
// from generation through recording. Other kinds can share the content lock.
func TestIngestGenerationHoldsLocksThroughRecording(t *testing.T) {
	for _, tc := range []struct {
		job  domain.JobKind
		kind artifactKind
		call string
	}{
		{domain.JobThumbnail, artifactThumbnail, "thumbnail"},
		{domain.JobSeekThumbnail, artifactSeekThumbnails, "seek"},
		{domain.JobPreview, artifactPreview, "preview"},
	} {
		t.Run(string(tc.job), func(t *testing.T) {
			video := probedVideo(1, "a")
			store := &artifactContractStore{IngestStore: newFakeIngestStore(video)}
			generator := &fakeGenerator{}
			ingest := NewIngest(IngestOptions{
				Store: store, Generator: generator, Artifacts: fakeArtifactStore{generator}, Logger: discardLogger(),
			})
			var observed []string
			generator.gate = func(call string) {
				if call != tc.call {
					t.Fatalf("generation = %q, want %q", call, tc.call)
				}
				assertGenerationLockModes(t, ingest.artifacts, video.ContentKey, tc.kind)
				observed = append(observed, "generate")
			}
			store.record = func(kind artifactKind, key string) {
				if kind != tc.kind || key != video.ContentKey {
					t.Fatalf("record kind/key = %v/%q", kind, key)
				}
				assertGenerationLockModes(t, ingest.artifacts, key, kind)
				observed = append(observed, "record")
			}
			if err := ingest.Handler(tc.job)(context.Background(), jobFor(tc.job, video)); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(observed, []string{"generate", "record"}) {
				t.Fatalf("operations = %v", observed)
			}
		})
	}
}

// Exercise the public release entry point: both the reference check and removal
// must run under the content's exclusive lock, and referenced content stays.
func TestReleaseArtifactsRemovesOnlyUnreferencedContent(t *testing.T) {
	store := &artifactContractStore{IngestStore: newFakeIngestStore()}
	files := artifactContractFiles{}
	var ingest *Ingest
	var observed []string
	store.reference = func(_ context.Context, key string) (bool, error) {
		assertArtifactLockMode(t, &ingest.artifacts.contents, key, false)
		observed = append(observed, "reference:"+key)
		return key == "kept", nil
	}
	files.remove = func(key string) error {
		assertArtifactLockMode(t, &ingest.artifacts.contents, key, false)
		observed = append(observed, "remove:"+key)
		return nil
	}
	ingest = NewIngest(IngestOptions{Store: store, Artifacts: files, Logger: discardLogger()})
	ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{"kept", "released"}})
	ingest.Wait()
	if want := []string{"reference:kept", "reference:released", "remove:released"}; !slices.Equal(observed, want) {
		t.Fatalf("operations = %v, want %v", observed, want)
	}
}

// Cleanup releases generation locks before checking references. If unreferenced,
// it takes the exclusive content lock and checks again before removing anything.
func TestArtifactCleanupRechecksReferencesAfterGeneration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		references []bool
		removed    bool
	}{
		{"still referenced", []bool{true}, false},
		{"unreferenced", []bool{false, false}, true},
		{"referenced again", []bool{false, true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var a *artifacts
			reads := 0
			store := &artifactContractStore{}
			store.reference = func(_ context.Context, key string) (bool, error) {
				if key != "a" || reads >= len(tc.references) {
					t.Fatalf("unexpected reference check: key = %q, reads = %d", key, reads)
				}
				if len(a.kinds.locks) != 0 {
					t.Fatal("cleanup kept the generation kind lock")
				}
				if reads == 0 {
					if len(a.contents.locks) != 0 {
						t.Fatal("cleanup did not release the generation content lock")
					}
				} else {
					assertArtifactLockMode(t, &a.contents, key, false)
				}
				value := tc.references[reads]
				reads++
				return value, nil
			}
			var removed []string
			files := artifactContractFiles{remove: func(key string) error {
				assertArtifactLockMode(t, &a.contents, key, false)
				removed = append(removed, key)
				return nil
			}}
			a = newArtifacts(store, files, discardLogger())
			if err := a.generate(context.Background(), "a", artifactPreview, func() (bool, error) {
				assertGenerationLockModes(t, a, "a", artifactPreview)
				return true, nil
			}); err != nil {
				t.Fatal(err)
			}
			var wantRemoved []string
			if tc.removed {
				wantRemoved = []string{"a"}
			}
			if reads != len(tc.references) || !slices.Equal(removed, wantRemoved) {
				t.Fatalf("reference checks = %d, removed = %v", reads, removed)
			}
		})
	}
}
