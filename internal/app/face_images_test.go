package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// detectImage answers a face image with imageFaces good faces side by side, holding it while holdImage is set.
func (h *fakeFaceHelpers) detectImage(ctx context.Context, r domain.FaceRecognizer) (faceproto.Detections, error) {
	h.mu.Lock()
	h.images++
	n, hold, release, err := h.imageFaces, h.holdImage, h.release, h.err
	h.mu.Unlock()
	if err != nil {
		return faceproto.Detections{}, err
	}
	if hold {
		select {
		case <-ctx.Done():
			return faceproto.Detections{}, ctx.Err()
		case <-release:
		}
	}
	var d faceproto.Detections
	for i := range n {
		d.Faces = append(d.Faces, scriptedFace(r, float32(10+200*i)))
	}
	return d, nil
}

func (h *fakeFaceHelpers) set(change func(h *fakeFaceHelpers)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	change(h)
}

func (h *fakeFaceHelpers) imageRequests() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.images
}

// faceJPEG is a stored face: a JPEG wide enough for the fake helper's faces.
func faceJPEG(t *testing.T) []byte {
	t.Helper()
	img, err := encodePersonImage(image.NewRGBA(image.Rect(0, 0, 600, 400)))
	if err != nil {
		t.Fatal(err)
	}
	return img.JPEG
}

type fakeFaceVector struct {
	revision   int64
	recognizer domain.FaceRecognizer
	vector     []float32
}

// fakeStoredFaceImage is one accepted StoreFaceImageVector call.
type fakeStoredFaceImage struct {
	job    domain.FaceImageJob
	vector []float32
}

// fakeFaceImageStore mirrors FaceStore: jobs carry the current recognizer and generation, and a vector of an older
// generation is refused.
type fakeFaceImageStore struct {
	settings *fakeFaceSettingsState
	mu       sync.Mutex
	faces    map[int64][]byte
	vectors  map[int64]fakeFaceVector
	stored   []fakeStoredFaceImage
	attempts int
	fault    error
	// refusal, when set, is returned after each accepted vector, as a merge refused for good is.
	refusal error
	// onStore, when set, runs with each accepted vector.
	onStore func()
}

func newFakeFaceImageStore(settings *fakeFaceSettingsState) *fakeFaceImageStore {
	return &fakeFaceImageStore{settings: settings, faces: map[int64][]byte{}, vectors: map[int64]fakeFaceVector{}}
}

func (s *fakeFaceImageStore) current() (domain.FaceRecognizer, int64) {
	s.settings.mu.Lock()
	defer s.settings.mu.Unlock()
	return s.settings.value.Recognizer, s.settings.generation
}

func (s *fakeFaceImageStore) job(id int64) domain.FaceImageJob {
	recognizer, generation := s.current()
	job := domain.FaceImageJob{PersonID: id, Revision: 1, Recognizer: recognizer, Generation: generation, JPEG: s.faces[id]}
	if v, ok := s.vectors[id]; ok && v.recognizer == recognizer {
		job.Embedded, job.Vector = true, v.vector
	}
	return job
}

func (s *fakeFaceImageStore) FaceImage(_ context.Context, id int64) (domain.FaceImageJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.faces[id]; !ok {
		return domain.FaceImageJob{}, domain.ErrNotFound
	}
	return s.job(id), nil
}

func (s *fakeFaceImageStore) StaleFaceImage(context.Context) (domain.FaceImageJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	for id := range s.faces {
		if job := s.job(id); !job.Embedded {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return domain.FaceImageJob{}, domain.ErrNoJob
	}
	return s.job(slices.Min(ids)), nil
}

func (s *fakeFaceImageStore) StoreFaceImageVector(_ context.Context, job domain.FaceImageJob, vector []float32) (
	domain.FaceImageDecision, error,
) {
	_, generation := s.current()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	if s.fault != nil {
		return domain.FaceImageDecision{}, s.fault
	}
	if job.Generation != generation {
		return domain.FaceImageDecision{}, domain.ErrFaceGenerationChanged
	}
	s.vectors[job.PersonID] = fakeFaceVector{revision: job.Revision, recognizer: job.Recognizer, vector: vector}
	s.stored = append(s.stored, fakeStoredFaceImage{job: job, vector: vector})
	if s.onStore != nil {
		s.onStore()
	}
	return domain.FaceImageDecision{}, s.refusal
}

func (s *fakeFaceImageStore) add(id int64, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faces[id] = data
}

func (s *fakeFaceImageStore) setFault(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fault = err
}

func (s *fakeFaceImageStore) read() ([]fakeStoredFaceImage, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.stored), s.attempts
}

func TestFaceRecognizerEmbedsAFaceBeforeTheNextVideo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		faces  int
		vector bool
	}{
		{"no face", 0, false},
		{"one face", 1, true},
		{"two faces", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFaceWorkerFixture("key-a")
				f.helpers.imageFaces = tc.faces
				f.images.add(7, faceJPEG(t))
				claimsAtStore := -1
				f.images.onStore = func() { claimsAtStore = f.queue.state().claims }
				f.run(t)
				synctest.Wait()

				if claimsAtStore != 0 {
					t.Fatalf("videos claimed before the face was stored = %d, want 0", claimsAtStore)
				}
				stored, _ := f.images.read()
				if len(stored) != 1 || stored[0].job.PersonID != 7 || (stored[0].vector != nil) != tc.vector {
					t.Fatalf("stored = %+v, want person 7's vector present %v", stored, tc.vector)
				}
				if tc.vector && (len(stored[0].vector) != 128 || stored[0].vector[0] != 1) {
					t.Fatalf("vector = %v, want the face's", stored[0].vector)
				}
				// The face, stored once even without a vector, is not embedded again before or after the video.
				if f.helpers.imageRequests() != 1 || len(f.queue.state().stored) != 1 {
					t.Fatalf("image requests %d, videos stored %d; want 1 and 1",
						f.helpers.imageRequests(), len(f.queue.state().stored))
				}
			})
		})
	}
}

func TestFaceRecognizerAppliesANamedFaceAgainWithoutTheHelper(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture()
		f.images.add(7, faceJPEG(t))
		f.images.vectors[7] = fakeFaceVector{revision: 1, recognizer: domain.FaceRecognizerSFaceInt8, vector: []float32{1}}
		f.run(t)
		synctest.Wait()

		f.rec.PersonImageChanged(domain.PersonImageChanged{PersonID: 8}) // gone since
		f.rec.PersonImageChanged(domain.PersonImageChanged{PersonID: 7})
		synctest.Wait()
		stored, _ := f.images.read()
		if len(stored) != 1 || !stored[0].job.Embedded || !slices.Equal(stored[0].vector, []float32{1}) {
			t.Fatalf("stored = %+v, want person 7's stored vector applied again", stored)
		}
		if f.helpers.imageRequests() != 0 {
			t.Fatalf("image requests = %d, want none", f.helpers.imageRequests())
		}
	})
}

// A vector computed for the recognizer the owner switched away from is never stored.
func TestFaceRecognizerSwitchedMidEmbeddingStoresOnlyTheNewRecognizersVector(t *testing.T) {
	arcface := domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50}
	for _, tc := range []struct {
		name     string
		switchTo func(f *faceWorkerFixture)
	}{
		{"the switch cancels the embedding", func(f *faceWorkerFixture) { f.change(arcface) }},
		{"the old vector reaches the store after the switch", func(f *faceWorkerFixture) {
			f.settings.set(arcface)
			close(f.helpers.release)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFaceWorkerFixture()
				f.helpers.imageFaces, f.helpers.holdImage = 1, true
				f.images.add(7, faceJPEG(t))
				f.run(t)
				synctest.Wait()

				tc.switchTo(f)
				f.helpers.set(func(h *fakeFaceHelpers) { h.holdImage = false })
				synctest.Wait()
				stored, _ := f.images.read()
				if len(stored) != 1 || stored[0].job.Recognizer != domain.FaceRecognizerArcFaceR50 ||
					stored[0].job.Generation != 1 || len(stored[0].vector) != 512 {
					t.Fatalf("stored = %+v, want only ArcFace's vector of generation 1", stored)
				}
			})
		})
	}
}

func TestFaceRecognizerRetriesAFaceAfterADatabaseError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture()
		f.helpers.imageFaces = 1
		f.images.add(7, faceJPEG(t))
		f.images.setFault(errors.New("database is locked"))
		f.run(t)
		synctest.Wait()
		if _, attempts := f.images.read(); attempts != 1 {
			t.Fatalf("attempts before the back-off = %d, want 1", attempts)
		}

		f.images.setFault(nil)
		time.Sleep(time.Duration(domain.WorkerRetry))
		synctest.Wait()
		if stored, _ := f.images.read(); len(stored) != 1 || stored[0].job.PersonID != 7 {
			t.Fatalf("stored = %+v, want person 7's vector after the back-off", stored)
		}
	})
}

func TestFaceRecognizerDoesNotRetryAMergeRefusedForGood(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture()
		f.helpers.imageFaces = 1
		f.images.add(7, faceJPEG(t))
		f.images.refusal = fmt.Errorf("%w: person 9 into person 7", domain.ErrFaceMergeRefused)
		f.run(t)
		synctest.Wait()

		time.Sleep(2 * time.Duration(domain.WorkerRetry))
		synctest.Wait()
		if _, attempts := f.images.read(); attempts != 1 {
			t.Fatalf("attempts = %d, want 1 with no retry", attempts)
		}
	})
}
