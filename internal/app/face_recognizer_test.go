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

// faceFrames is the number of sampled frames each fake video has: a face in frames 0–9 and 13–22 forms two tracks
// with 20 good seconds at 1 fps, one main person.
const faceFrames = 23

func faceOnScreen(frame int) bool { return frame < 10 || frame >= 13 }

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

// fakeFaceAnalysisQueue mirrors FaceStore: FaceJob reads the current recognizer and generation, and Store refuses a
// result of an older generation.
type fakeFaceAnalysisQueue struct {
	settings *fakeFaceSettingsState
	mu       sync.Mutex
	queued   []string
	stored   []domain.FaceAnalysis
	// helperWork is what HasHelperWork reports.
	helperWork bool
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

func (q *fakeFaceAnalysisQueue) results() []domain.FaceAnalysis {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.stored)
}

func (q *fakeFaceAnalysisQueue) queuedKeys() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.queued)
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

func (q *fakeFaceAnalysisQueue) ClearPeople(context.Context) (domain.PeopleCleared, error) {
	return domain.PeopleCleared{}, nil
}

func (q *fakeFaceAnalysisQueue) FaceJob(_ context.Context, key string) (domain.FaceJob, error) {
	q.mu.Lock()
	err := q.faults["FaceJob"]
	q.mu.Unlock()
	if err != nil {
		return domain.FaceJob{}, err
	}
	q.settings.mu.Lock()
	defer q.settings.mu.Unlock()
	return domain.FaceJob{ContentKey: key, Recognizer: q.settings.value.Recognizer, Generation: q.settings.generation}, nil
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
	return nil
}

func (q *fakeFaceAnalysisQueue) HasHelperWork(context.Context) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.helperWork, nil
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

func (h *fakeFaceHelpers) set(change func(h *fakeFaceHelpers)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	change(h)
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
	wakes    *int
}

func newFaceWorkerFixture() *faceWorkerFixture {
	settings := &fakeFaceSettingsState{value: domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8}}
	wakes := 0
	f := &faceWorkerFixture{
		settings: settings,
		queue:    &fakeFaceAnalysisQueue{settings: settings},
		images:   newFakeFaceImageStore(settings),
		runtime:  &fakeFaceRuntimeState{state: domain.FaceRuntimeReady},
		helpers:  &fakeFaceHelpers{holdAt: -1, release: make(chan struct{})},
		frames:   &fakeFaceVideoFrames{},
		folders:  &fakeMediaFolders{},
		wakes:    &wakes,
	}
	f.rec = NewFaceRecognizer(FaceRecognizerOptions{
		Queue: f.queue, Images: f.images, Settings: settings, Runtime: f.runtime, Helper: f.helpers.helper,
		Frames: f.frames.source(), MediaFolders: f.folders, MediaFiles: fakeMediaFiles{},
		Wake: func() { wakes++ },
	})
	return f
}

// faceJob is the claimed face job of key, at its location under the fake media folder.
func faceJob(key string) domain.Job {
	return domain.Job{Kind: domain.JobFace, ContentKey: key, LocationPath: "/media/" + key + ".mp4"}
}

// start runs the job in the background, as the scheduler does, and returns its result once it ends.
func (f *faceWorkerFixture) start(job domain.Job) <-chan error {
	done := make(chan error, 1)
	go func() { done <- f.rec.HandleJob(context.Background(), job) }()
	return done
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

func TestFaceRecognizerStoresTheMainPeopleOfTheClaimedLocation(t *testing.T) {
	f := newFaceWorkerFixture()
	if err := f.rec.HandleJob(context.Background(), faceJob("key-a")); err != nil {
		t.Fatal(err)
	}

	stored := f.queue.results()
	if len(stored) != 1 {
		t.Fatalf("stored = %+v; want one result", stored)
	}
	got := stored[0]
	if got.ContentKey != "key-a" || got.Recognizer != domain.FaceRecognizerSFaceInt8 || got.Generation != 0 {
		t.Fatalf("result = %s %s %d", got.ContentKey, got.Recognizer, got.Generation)
	}
	if len(got.People) != 1 || len(got.People[0].Spans) != 2 || got.People[0].GoodSeconds != 20 ||
		len(got.People[0].Crop.JPEG) == 0 || got.People[0].Crop.Width != 200 || got.People[0].Crop.Height != 200 {
		t.Fatalf("people = %+v, want one person in two tracks with a 200 px crop", got.People)
	}
	if paths := f.frames.read(); !slices.Equal(paths, []string{"resolved:/media/key-a.mp4"}) {
		t.Fatalf("frames read from %v, want the claimed location resolved through the media folders", paths)
	}
}

// The helper holds the models in memory, so it stays only while another analysis or embedding is due, and the next
// one starts it again.
func TestFaceRecognizerStopsTheHelperAfterTheLastFaceJob(t *testing.T) {
	f := newFaceWorkerFixture()
	f.queue.helperWork = true
	if err := f.rec.HandleJob(context.Background(), faceJob("key-a")); err != nil {
		t.Fatal(err)
	}
	if created, closed := f.helpers.made(); len(created) != 1 || closed != 0 {
		t.Fatalf("helpers created %v, closed %d; want one kept for the due job", created, closed)
	}

	f.queue.helperWork = false
	if err := f.rec.HandleJob(context.Background(), faceJob("key-b")); err != nil {
		t.Fatal(err)
	}
	if created, closed := f.helpers.made(); len(created) != 1 || closed != 1 {
		t.Fatalf("helpers created %v, closed %d; want the same helper closed after the last job", created, closed)
	}

	if err := f.rec.HandleJob(context.Background(), faceJob("key-c")); err != nil {
		t.Fatal(err)
	}
	if created, closed := f.helpers.made(); len(created) != 2 || closed != 2 {
		t.Fatalf("helpers created %v, closed %d; want a new helper for the next job, closed again", created, closed)
	}
}

// The face lane waits, its jobs keeping their attempts, while the feature is off or the runtime is not ready; a saved
// setting wakes the scheduler, and so does the runtime's Ready, which cmd/mdm wires to the same wakeup.
func TestFaceRecognizerPausesTheFaceLaneUntilTheFeatureAndRuntimeAreReady(t *testing.T) {
	f := newFaceWorkerFixture()
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		enabled bool
		runtime domain.FaceRuntimeState
		paused  bool
	}{
		{"off", false, domain.FaceRuntimeReady, true},
		{"downloading", true, domain.FaceRuntimeDownloading, true},
		{"failed", true, domain.FaceRuntimeFailed, true},
		{"ready", true, domain.FaceRuntimeReady, false},
	} {
		f.settings.set(domain.FaceSettings{Enabled: tc.enabled, Recognizer: domain.FaceRecognizerSFaceInt8})
		f.runtime.set(tc.runtime)
		paused, err := f.rec.PausedJobKinds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []domain.JobKind(nil)
		if tc.paused {
			want = domain.FaceLaneJobKinds
		}
		if !slices.Equal(paused, want) {
			t.Errorf("%s: paused = %v, want %v", tc.name, paused, want)
		}
	}
	f.change(domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8})
	if *f.wakes != 1 {
		t.Fatalf("wakes after saving the settings = %d, want 1", *f.wakes)
	}
}

func TestFaceRecognizerTurnedOffMidVideoSnoozesAndStartsAgainFromTheFirstFrame(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture()
		f.helpers.holdAt = 2
		done := f.start(faceJob("key-a"))
		synctest.Wait()

		f.change(domain.FaceSettings{Enabled: false, Recognizer: domain.FaceRecognizerSFaceInt8})
		if err := <-done; !errors.Is(err, domain.ErrJobSnoozed) || len(f.queue.results()) != 0 {
			t.Fatalf("after turning off: %v, stored %d; want a snooze and nothing stored", err, len(f.queue.results()))
		}
		if _, closed := f.helpers.made(); closed != 1 {
			t.Fatalf("helper closed %d times after turning off, want once", closed)
		}

		f.helpers.set(func(h *fakeFaceHelpers) { h.holdAt = -1 })
		f.change(domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8})
		if err := <-f.start(faceJob("key-a")); err != nil || len(f.queue.results()) != 1 {
			t.Fatalf("after turning on again: %v, stored %d; want the video stored", err, len(f.queue.results()))
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
				f := newFaceWorkerFixture()
				f.helpers.holdAt = 2
				done := f.start(faceJob("key-a"))
				synctest.Wait()

				tc.switchTo(f)
				if err := <-done; !errors.Is(err, domain.ErrJobSnoozed) {
					t.Fatalf("old recognizer's run = %v, want a snooze", err)
				}
				f.helpers.set(func(h *fakeFaceHelpers) { h.holdAt = -1 })
				if err := <-f.start(faceJob("key-a")); err != nil {
					t.Fatal(err)
				}
				stored := f.queue.results()
				if len(stored) != 1 || stored[0].Recognizer != domain.FaceRecognizerArcFaceR50 || stored[0].Generation != 1 ||
					len(stored[0].People) != 1 || len(stored[0].People[0].Centroid) != 512 {
					t.Fatalf("stored %+v, want only ArcFace's result of generation 1", stored)
				}
				want := []domain.FaceRecognizer{domain.FaceRecognizerSFaceInt8, domain.FaceRecognizerArcFaceR50}
				if created, _ := f.helpers.made(); !slices.Equal(created, want) {
					t.Fatalf("helpers = %v, want %v", created, want)
				}
			})
		})
	}
}

// A helper that did not start and a database error leave the video to run again, the first without an attempt, since
// the runtime is at fault and the lane pauses; a helper that fails mid-video fails the attempt with its message.
func TestFaceRecognizerClassifiesFailures(t *testing.T) {
	dbErr := errors.New("database is locked")
	for _, tc := range []struct {
		name    string
		fault   func(f *faceWorkerFixture)
		snoozed bool
		message string
	}{
		{name: "helper start", snoozed: true, message: "no Hello", fault: func(f *faceWorkerFixture) {
			f.helpers.err = fmt.Errorf("%w: no Hello", domain.ErrFaceHelperStart)
		}},
		{name: "helper crash", message: "exited", fault: func(f *faceWorkerFixture) {
			f.helpers.err = errors.New("the face helper exited: unexpected EOF")
		}},
		{name: "storing the result", message: "locked", fault: func(f *faceWorkerFixture) {
			f.queue.setFault("Store", dbErr)
		}},
		{name: "reading the settings of the job", message: "locked", fault: func(f *faceWorkerFixture) {
			f.queue.setFault("FaceJob", dbErr)
		}},
		{name: "reading the media folders", message: "locked", fault: func(f *faceWorkerFixture) {
			f.folders.setErr(dbErr)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFaceWorkerFixture()
			tc.fault(f)
			err := f.rec.HandleJob(context.Background(), faceJob("key-a"))
			if err == nil || errors.Is(err, domain.ErrJobSnoozed) != tc.snoozed || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("HandleJob = %v; want snoozed %v with %q", err, tc.snoozed, tc.message)
			}
			if len(f.queue.results()) != 0 {
				t.Fatalf("stored %+v after a failure, want nothing", f.queue.results())
			}
		})
	}
}

func TestFaceRecognizerFailsAVideoWhoseHelperStopsAnswering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFaceWorkerFixture()
		f.helpers.holdAt = 0
		done := f.start(faceJob("key-a"))
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("ended before the timeout: %v", err)
		default:
		}

		time.Sleep(faceDetectTimeout)
		if err := <-done; err == nil || errors.Is(err, domain.ErrJobSnoozed) || !strings.Contains(err.Error(), "did not answer") {
			t.Fatalf("HandleJob = %v, want the video failed after %s", err, faceDetectTimeout)
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
	if queued := f.queue.queuedKeys(); len(queued) != 0 {
		t.Fatalf("queued %v while off, want nothing", queued)
	}
}

// fakeFaceCast records the videos the cast rule was applied to; err, when set, fails every call.
type fakeFaceCast struct {
	checked []int64
	err     error
}

func (c *fakeFaceCast) MergeByCast(_ context.Context, videoID int64) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	c.checked = append(c.checked, videoID)
	return true, nil
}

// The cast rule's job applies the rule to its video; a video gone and a merge refused for good end the job, since a
// retry cannot change them, and a database error is retried.
func TestFaceRecognizerAppliesTheCastRuleToTheJobsVideo(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		checked []int64
		failed  bool
	}{
		{name: "merged", checked: []int64{4}},
		{name: "video gone", err: domain.ErrNotFound},
		{name: "refused", err: fmt.Errorf("%w: too many external ids", domain.ErrFaceMergeRefused)},
		{name: "database error", err: errors.New("database is locked"), failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFaceWorkerFixture()
			cast := &fakeFaceCast{err: tc.err}
			f.rec.cast = cast
			err := f.rec.HandleJob(context.Background(), domain.Job{Kind: domain.JobFaceCast, VideoID: 4})
			if (err != nil) != tc.failed || !slices.Equal(cast.checked, tc.checked) {
				t.Fatalf("HandleJob = %v, checked %v; want failed %v and %v", err, cast.checked, tc.failed, tc.checked)
			}
		})
	}
}
