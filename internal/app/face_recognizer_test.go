package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// faceFrames is the number of sampled frames each fake video has: a face in frames 0–19 and 23–42 forms two tracks
// with 20 good seconds, one main person.
const faceFrames = 43

func faceOnScreen(frame int) bool { return frame < 20 || frame >= 23 }

// fakeFaceSettingsState mirrors the store: a recognizer switch increases the generation.
type fakeFaceSettingsState struct {
	mu         sync.Mutex
	value      domain.FaceSettings
	generation int64
}

func (s *fakeFaceSettingsState) FaceSettings(context.Context) (domain.FaceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, nil
}

func (s *fakeFaceSettingsState) set(v domain.FaceSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.Recognizer != s.value.Recognizer {
		s.generation++
	}
	s.value = v
}

// fakeFaceAnalysisQueue mirrors FaceStore: Claim reads the current recognizer and generation, and Store refuses a
// result of an older generation.
type fakeFaceAnalysisQueue struct {
	settings *fakeFaceSettingsState
	mu       sync.Mutex
	queued   []string
	running  string
	claims   int
	requeues int
	stored   []domain.FaceAnalysis
	failed   []string
	// faults fails the named method with its error until the test clears it.
	faults map[string]error
}

func (q *fakeFaceAnalysisQueue) setFault(method string, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.faults == nil {
		q.faults = map[string]error{}
	}
	q.faults[method] = err
}

// fakeFaceQueueState is what a test reads of the queue, copied under its lock.
type fakeFaceQueueState struct {
	queued           []string
	running          string
	claims, requeues int
	stored           []domain.FaceAnalysis
	failed           []string
}

func (q *fakeFaceAnalysisQueue) state() fakeFaceQueueState {
	q.mu.Lock()
	defer q.mu.Unlock()
	return fakeFaceQueueState{
		queued: slices.Clone(q.queued), running: q.running, claims: q.claims, requeues: q.requeues,
		stored: slices.Clone(q.stored), failed: slices.Clone(q.failed),
	}
}

func (q *fakeFaceAnalysisQueue) QueueLibrary(context.Context, domain.FaceQueueScope, domain.ActivityDecider) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queued = append(q.queued, "queued-by-run")
	return 1, nil
}

func (q *fakeFaceAnalysisQueue) RetryVideo(_ context.Context, key string, _ int64) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queued = append(q.queued, key)
	return 1, nil
}

func (q *fakeFaceAnalysisQueue) Claim(context.Context) (domain.FaceJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queued) == 0 {
		return domain.FaceJob{}, domain.ErrNoJob
	}
	q.claims++
	q.running, q.queued = q.queued[0], q.queued[1:]
	q.settings.mu.Lock()
	defer q.settings.mu.Unlock()
	return domain.FaceJob{ContentKey: q.running, Recognizer: q.settings.value.Recognizer, Generation: q.settings.generation}, nil
}

func (q *fakeFaceAnalysisQueue) VideoPaths(_ context.Context, key string) ([]string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.faults["VideoPaths"]; err != nil {
		return nil, err
	}
	return []string{"/media/" + key + ".mp4"}, nil
}

func (q *fakeFaceAnalysisQueue) Store(_ context.Context, result domain.FaceAnalysis) error {
	q.settings.mu.Lock()
	generation := q.settings.generation
	q.settings.mu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.faults["Store"]; err != nil {
		return err
	}
	if result.Generation != generation {
		return domain.ErrFaceGenerationChanged
	}
	q.stored = append(q.stored, result)
	q.running = ""
	return nil
}

func (q *fakeFaceAnalysisQueue) Fail(_ context.Context, _ string, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.faults["Fail"]; err != nil {
		return err
	}
	q.failed = append(q.failed, reason)
	q.running = ""
	return nil
}

func (q *fakeFaceAnalysisQueue) RequeueRunning(context.Context) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.faults["RequeueRunning"]; err != nil {
		return 0, err
	}
	if q.running == "" {
		return 0, nil
	}
	q.requeues++
	q.queued, q.running = append([]string{q.running}, q.queued...), ""
	return 1, nil
}

type fakeFaceRuntimeState struct {
	mu    sync.Mutex
	state domain.FaceRuntimeState
}

func (r *fakeFaceRuntimeState) Status(domain.FaceRecognizer) domain.FaceRuntimeStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return domain.FaceRuntimeStatus{State: r.state}
}

func (r *fakeFaceRuntimeState) set(s domain.FaceRuntimeState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = s
}

// fakeFaceHelpers hands out helpers that answer each frame with the scripted face; a frame at holdAt blocks until
// the request's context ends or release is closed, and err, when set, fails every request.
type fakeFaceHelpers struct {
	mu      sync.Mutex
	created []domain.FaceRecognizer
	closed  int
	frames  []int
	holdAt  int
	release chan struct{}
	err     error
	// imageFaces is the number of good faces in every face image; holdImage blocks a face image like holdAt.
	imageFaces int
	holdImage  bool
	images     int
}

func (h *fakeFaceHelpers) helper(r domain.FaceRecognizer) FaceHelper {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.created = append(h.created, r)
	return &fakeFaceHelper{helpers: h, recognizer: r}
}

type fakeFaceHelper struct {
	helpers    *fakeFaceHelpers
	recognizer domain.FaceRecognizer
}

func (h *fakeFaceHelper) Detect(ctx context.Context, req faceproto.DetectRequest) (faceproto.Detections, error) {
	if req.Width != domain.FaceFrameSide {
		return h.helpers.detectImage(ctx, h.recognizer)
	}
	frame := int(req.Pix[0])
	hs := h.helpers
	hs.mu.Lock()
	hs.frames = append(hs.frames, frame)
	hold, release, err := frame == hs.holdAt, hs.release, hs.err
	hs.mu.Unlock()
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
	if !faceOnScreen(frame) {
		return faceproto.Detections{}, nil
	}
	return faceproto.Detections{Faces: []faceproto.Detection{scriptedFace(h.recognizer, 500)}}, nil
}

// scriptedFace is a good face at x whose vector lies along the first axis.
func scriptedFace(r domain.FaceRecognizer, x float32) faceproto.Detection {
	spec, _ := domain.LookupFaceRecognizer(r)
	vector := make([]float32, spec.Dimensions)
	vector[0] = 1
	return faceproto.Detection{
		X: x, Y: 300, W: 100, H: 100, Score: 0.95, Vector: vector,
		Landmarks: [10]float32{x + 30, 330, x + 70, 330, x + 50, 350, x + 35, 375, x + 65, 375},
	}
}

func (h *fakeFaceHelper) Close() error {
	h.helpers.mu.Lock()
	defer h.helpers.mu.Unlock()
	h.helpers.closed++
	return nil
}

func (h *fakeFaceHelpers) made() ([]domain.FaceRecognizer, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.created), h.closed
}

func (h *fakeFaceHelpers) seen() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.frames)
}

// fakeFaceVideoFrames writes the frame's index into its first byte and records the paths it reads.
type fakeFaceVideoFrames struct {
	mu    sync.Mutex
	paths []string
}

func (v *fakeFaceVideoFrames) read() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.Clone(v.paths)
}

func (v *fakeFaceVideoFrames) source() FaceFrameSource {
	return func(ctx context.Context, path string, each func([]byte) error) error {
		v.mu.Lock()
		v.paths = append(v.paths, path)
		v.mu.Unlock()
		pix := make([]byte, domain.FaceFrameSide*domain.FaceFrameSide*3)
		for i := range faceFrames {
			if err := ctx.Err(); err != nil {
				return err
			}
			pix[0] = byte(i)
			if err := each(pix); err != nil {
				return err
			}
		}
		return nil
	}
}

type fakeMediaFolders struct {
	mu  sync.Mutex
	err error
}

func (m *fakeMediaFolders) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *fakeMediaFolders) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return []domain.MediaFolder{{Path: "/media"}}, nil
}

type fakeMediaFiles struct{}

func (fakeMediaFiles) ResolveMediaFile(roots []string, path string) (string, error) {
	if len(roots) != 1 || !strings.HasPrefix(path, roots[0]) {
		return "", domain.ErrMediaFileOutsideRoot
	}
	return "resolved:" + path, nil
}

type faceWorkerFixture struct {
	rec      *FaceRecognizer
	settings *fakeFaceSettingsState
	queue    *fakeFaceAnalysisQueue
	images   *fakeFaceImageStore
	runtime  *fakeFaceRuntimeState
	helpers  *fakeFaceHelpers
	frames   *fakeFaceVideoFrames
	folders  *fakeMediaFolders
}

func newFaceWorkerFixture(keys ...string) *faceWorkerFixture {
	settings := &fakeFaceSettingsState{value: domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8}}
	f := &faceWorkerFixture{
		settings: settings,
		queue:    &fakeFaceAnalysisQueue{settings: settings, queued: keys},
		images:   newFakeFaceImageStore(settings),
		runtime:  &fakeFaceRuntimeState{state: domain.FaceRuntimeReady},
		helpers:  &fakeFaceHelpers{holdAt: -1, release: make(chan struct{})},
		frames:   &fakeFaceVideoFrames{},
		folders:  &fakeMediaFolders{},
	}
	f.rec = NewFaceRecognizer(FaceRecognizerOptions{
		Queue: f.queue, Images: f.images, Settings: settings, Runtime: f.runtime, Helper: f.helpers.helper,
		Frames: f.frames.source(), MediaFolders: f.folders, MediaFiles: fakeMediaFiles{},
	})
	return f
}

// run starts the worker inside the synctest bubble and stops it when the test ends.
func (f *faceWorkerFixture) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.rec.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func (f *faceWorkerFixture) change(s domain.FaceSettings) {
	f.settings.set(s)
	f.rec.SettingsChanged(s)
}

func framesUpTo(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestFaceRecognizerStoresTheMainPeopleOfTheResolvedVideo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture("key-a")
		f.run(t)
		synctest.Wait()

		if len(f.queue.state().stored) != 1 {
			t.Fatalf("stored = %+v, failed = %v; want one result", f.queue.state().stored, f.queue.state().failed)
		}
		got := f.queue.state().stored[0]
		if got.ContentKey != "key-a" || got.Recognizer != domain.FaceRecognizerSFaceInt8 || got.Generation != 0 {
			t.Fatalf("result = %s %s %d", got.ContentKey, got.Recognizer, got.Generation)
		}
		if len(got.People) != 1 || len(got.People[0].Spans) != 2 || got.People[0].GoodSeconds != 20 ||
			len(got.People[0].Crop.JPEG) == 0 || got.People[0].Crop.Width != 200 || got.People[0].Crop.Height != 200 {
			t.Fatalf("people = %+v, want one person in two tracks with a 200 px crop", got.People)
		}
		if paths := f.frames.read(); !slices.Equal(paths, []string{"resolved:/media/key-a.mp4"}) {
			t.Fatalf("frames read from %v, want the location resolved through the media folders", paths)
		}
		if _, closed := f.helpers.made(); closed != 1 {
			t.Fatalf("helper closed %d times, want once when the queue ran empty", closed)
		}
	})
}

func TestFaceRecognizerWaitsForTheRuntime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture("key-a")
		f.runtime.set(domain.FaceRuntimeDownloading)
		f.run(t)
		synctest.Wait()
		if f.queue.state().claims != 0 {
			t.Fatalf("claims = %d while the runtime downloads, want 0", f.queue.state().claims)
		}

		f.runtime.set(domain.FaceRuntimeReady)
		f.rec.Wake()
		synctest.Wait()
		if len(f.queue.state().stored) != 1 {
			t.Fatalf("stored = %d after the runtime became ready, want 1", len(f.queue.state().stored))
		}
	})
}

func TestFaceRecognizerTurnedOffMidVideoRequeuesAndStartsAgainFromTheFirstFrame(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture("key-a")
		f.helpers.holdAt = 2
		f.run(t)
		synctest.Wait()

		f.change(domain.FaceSettings{Enabled: false, Recognizer: domain.FaceRecognizerSFaceInt8})
		synctest.Wait()
		if f.queue.state().requeues != 1 || len(f.queue.state().stored) != 0 || len(f.queue.state().failed) != 0 ||
			!slices.Equal(f.queue.state().queued, []string{"key-a"}) {
			t.Fatalf("after turning off: requeues %d, stored %d, failed %v, queued %v; want the video queued again",
				f.queue.state().requeues, len(f.queue.state().stored), f.queue.state().failed, f.queue.state().queued)
		}

		f.helpers.mu.Lock()
		f.helpers.holdAt = -1
		f.helpers.mu.Unlock()
		f.change(domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8})
		synctest.Wait()
		if len(f.queue.state().stored) != 1 {
			t.Fatalf("stored = %d after turning on again, want 1", len(f.queue.state().stored))
		}
		if want := append([]int{0, 1, 2}, framesUpTo(faceFrames)...); !slices.Equal(f.helpers.seen(), want) {
			t.Fatalf("frames = %v, want the first run's 0–2 and then every frame from 0", f.helpers.seen())
		}
	})
}

func TestFaceRecognizerSwitchedMidVideoStoresOnlyTheNewRecognizersResult(t *testing.T) {
	arcface := domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50}
	for _, tc := range []struct {
		name string
		// switchTo changes the settings while the helper holds a frame of the old recognizer.
		switchTo func(f *faceWorkerFixture)
	}{
		{"the switch cancels the video", func(f *faceWorkerFixture) { f.change(arcface) }},
		{"the old result reaches the store after the switch", func(f *faceWorkerFixture) {
			f.settings.set(arcface)
			close(f.helpers.release)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFaceWorkerFixture("key-a")
				f.helpers.holdAt = 2
				f.run(t)
				synctest.Wait()

				tc.switchTo(f)
				f.helpers.mu.Lock()
				f.helpers.holdAt = -1
				f.helpers.mu.Unlock()
				f.rec.Wake()
				synctest.Wait()
				if len(f.queue.state().stored) != 1 || len(f.queue.state().failed) != 0 {
					t.Fatalf("stored %+v, failed %v; want only one result", f.queue.state().stored, f.queue.state().failed)
				}
				if got := f.queue.state().stored[0]; got.Recognizer != domain.FaceRecognizerArcFaceR50 || got.Generation != 1 ||
					len(got.People) != 1 || len(got.People[0].Centroid) != 512 {
					t.Fatalf("stored %s generation %d with %d people, want ArcFace's result of generation 1",
						got.Recognizer, got.Generation, len(got.People))
				}
				want := []domain.FaceRecognizer{domain.FaceRecognizerSFaceInt8, domain.FaceRecognizerArcFaceR50}
				if created, _ := f.helpers.made(); !slices.Equal(created, want) {
					t.Fatalf("helpers = %v, want %v", created, want)
				}
			})
		})
	}
}

func TestFaceRecognizerClassifiesHelperErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		// startFails marks the runtime failed when the helper is made, as the runtime does on a failed start.
		startFails bool
	}{
		{"start", fmt.Errorf("%w: no Hello", domain.ErrFaceHelperStart), true},
		{"crash mid-video", errors.New("the face helper exited: unexpected EOF"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFaceWorkerFixture("key-a")
				f.helpers.err = tc.err
				if tc.startFails {
					f.rec.newHelper = func(r domain.FaceRecognizer) FaceHelper {
						f.runtime.set(domain.FaceRuntimeFailed)
						return f.helpers.helper(r)
					}
				}
				f.run(t)
				synctest.Wait()
				if !tc.startFails {
					if len(f.queue.state().failed) != 1 || !strings.Contains(f.queue.state().failed[0], "exited") || f.queue.state().requeues != 0 {
						t.Fatalf("failed %v, requeues %d; want the video failed with the helper's message",
							f.queue.state().failed, f.queue.state().requeues)
					}
					return
				}
				// The video waits in the queue for the runtime instead of failing.
				if len(f.queue.state().failed) != 0 || f.queue.state().requeues != 1 || f.queue.state().claims != 1 ||
					!slices.Equal(f.queue.state().queued, []string{"key-a"}) {
					t.Fatalf("failed %v, requeues %d, claims %d, queued %v; want the video queued again",
						f.queue.state().failed, f.queue.state().requeues, f.queue.state().claims, f.queue.state().queued)
				}
			})
		})
	}
}

func TestFaceRecognizerFailsAVideoWhoseHelperStopsAnswering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture("key-a")
		f.helpers.holdAt = 0
		f.run(t)
		synctest.Wait()
		if len(f.queue.state().failed) != 0 {
			t.Fatalf("failed before the timeout: %v", f.queue.state().failed)
		}

		time.Sleep(faceDetectTimeout)
		synctest.Wait()
		if len(f.queue.state().failed) != 1 || !strings.Contains(f.queue.state().failed[0], "did not answer") {
			t.Fatalf("failed = %v, want the video failed after %s", f.queue.state().failed, faceDetectTimeout)
		}
	})
}

func TestFaceRecognizerRefusesRunsAndRetriesWhileOff(t *testing.T) {
	f := newFaceWorkerFixture()
	f.settings.set(domain.FaceSettings{Recognizer: domain.FaceRecognizerSFaceInt8})
	ctx := context.Background()

	if _, err := f.rec.QueueLibrary(ctx, domain.FaceQueueMissing); !errors.Is(err, domain.ErrFaceRecognitionOff) {
		t.Fatalf("QueueLibrary while off = %v, want ErrFaceRecognitionOff", err)
	}
	entry := domain.ActivityEntry{ID: 7, Kind: domain.ActivityFaceAnalysis,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: "key-a"}}
	if err := f.rec.RetryActivity(ctx, entry); !errors.Is(err, domain.ErrFaceRecognitionOff) {
		t.Fatalf("RetryActivity while off = %v, want ErrFaceRecognitionOff", err)
	}
	if len(f.queue.state().queued) != 0 {
		t.Fatalf("queued %v while off, want nothing", f.queue.state().queued)
	}
}

func TestFaceRecognizerRetriesAVideoAfterADatabaseError(t *testing.T) {
	dbErr := errors.New("database is locked")
	for _, tc := range []struct {
		name string
		// fault makes the first attempt hit dbErr; clear removes it before the back-off ends.
		fault, clear func(f *faceWorkerFixture)
		// failed is true when the retried video ends failed rather than stored.
		failed bool
	}{
		{name: "storing the result",
			fault: func(f *faceWorkerFixture) { f.queue.setFault("Store", dbErr) },
			clear: func(f *faceWorkerFixture) { f.queue.setFault("Store", nil) }},
		{name: "reading the video's locations",
			fault: func(f *faceWorkerFixture) { f.queue.setFault("VideoPaths", dbErr) },
			clear: func(f *faceWorkerFixture) { f.queue.setFault("VideoPaths", nil) }},
		{name: "reading the media folders",
			fault: func(f *faceWorkerFixture) { f.folders.setErr(dbErr) },
			clear: func(f *faceWorkerFixture) { f.folders.setErr(nil) }},
		{name: "recording the video's failure", failed: true,
			fault: func(f *faceWorkerFixture) {
				f.helpers.err = errors.New("the face helper exited")
				f.queue.setFault("Fail", dbErr)
			},
			clear: func(f *faceWorkerFixture) { f.queue.setFault("Fail", nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFaceWorkerFixture("key-a")
				tc.fault(f)
				f.run(t)
				synctest.Wait()
				if got := f.queue.state(); got.claims != 1 || got.requeues != 1 || got.running != "" ||
					!slices.Equal(got.queued, []string{"key-a"}) || len(got.stored)+len(got.failed) != 0 {
					t.Fatalf("after the error: %+v; want the video queued again and not retried before the back-off", got)
				}

				tc.clear(f)
				time.Sleep(faceRetryDelay)
				synctest.Wait()
				wantStored, wantFailed := 1, 0
				if tc.failed {
					wantStored, wantFailed = 0, 1
				}
				if got := f.queue.state(); got.claims != 2 || len(got.stored) != wantStored || len(got.failed) != wantFailed {
					t.Fatalf("after the back-off: %+v; want the video claimed again and %d stored, %d failed",
						got, wantStored, wantFailed)
				}
			})
		})
	}
}

func TestFaceRecognizerReturnsAStrandedVideoBeforeTheNextClaim(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture("key-a")
		f.queue.setFault("Store", errors.New("database is locked"))
		f.queue.setFault("RequeueRunning", errors.New("database is locked"))
		f.run(t)
		synctest.Wait()
		if got := f.queue.state(); got.running != "key-a" || got.claims != 1 {
			t.Fatalf("after the errors: %+v; want the video still running", got)
		}

		f.queue.setFault("Store", nil)
		f.queue.setFault("RequeueRunning", nil)
		time.Sleep(faceRetryDelay)
		synctest.Wait()
		if got := f.queue.state(); got.requeues != 1 || got.claims != 2 || len(got.stored) != 1 {
			t.Fatalf("after the back-off: %+v; want the video returned to the queue, claimed again and stored", got)
		}
	})
}
