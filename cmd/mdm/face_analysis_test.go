package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
	"github.com/syudead/vv/internal/jobs"
	"github.com/syudead/vv/internal/mediafs"
	"github.com/syudead/vv/internal/store"
)

// The face jobs turn frames and faces into vectors and the store matches them to people, so the parent Issue's
// acceptance 2, 3 and 4 cross both; frames and the helper are fakes scripted per video.

// faceScripts lists, per video file, the sampled frames that show the one face. A and B show it in two tracks of
// 10 s; C shows it in one track of 1 s.
var faceScripts = map[string]func(frame int) bool{
	"a.mp4": func(frame int) bool { return frame < 20 || frame >= 23 },
	"b.mp4": func(frame int) bool { return frame < 20 || frame >= 23 },
	"c.mp4": func(frame int) bool { return frame < 3 },
}

const scriptedFrames = 43

var faceVideos = []string{"a.mp4", "b.mp4", "c.mp4"}

// scriptedFaceFrames writes the video's index in faceVideos and the frame's index into each frame's first bytes.
func scriptedFaceFrames(_ context.Context, path string, each func([]byte) error) error {
	video := -1
	for i, name := range faceVideos {
		if filepath.Base(path) == name {
			video = i
		}
	}
	if video < 0 {
		return errors.New("unexpected video " + path)
	}
	pix := make([]byte, domain.FaceFrameSide*domain.FaceFrameSide*3)
	for frame := range scriptedFrames {
		pix[0], pix[1] = byte(video), byte(frame)
		if err := each(pix); err != nil {
			return err
		}
	}
	return nil
}

// scriptedFaceHelper shows the one face in the scripted frames and in every face image.
type scriptedFaceHelper struct{}

func (scriptedFaceHelper) Detect(_ context.Context, req faceproto.DetectRequest) (faceproto.Detections, error) {
	if req.Width == domain.FaceFrameSide && !faceScripts[faceVideos[req.Pix[0]]](int(req.Pix[1])) {
		return faceproto.Detections{}, nil
	}
	vector := make([]float32, 128)
	vector[0] = 1
	return faceproto.Detections{Faces: []faceproto.Detection{{
		X: 50, Y: 30, W: 100, H: 100, Score: 0.95, Vector: vector,
		Landmarks: [10]float32{80, 60, 120, 60, 100, 80, 85, 105, 115, 105},
	}}}, nil
}

func (scriptedFaceHelper) Close() error { return nil }

type readyFaceRuntime struct{}

func (readyFaceRuntime) Status(domain.FaceRecognizer) domain.FaceRuntimeStatus {
	return domain.FaceRuntimeStatus{State: domain.FaceRuntimeReady}
}

// faceAnalysisFixture is the store with the scripted videos queued, and a scheduler running the face jobs that has
// analysed them.
type faceAnalysisFixture struct {
	db         *store.DB
	recognizer *app.FaceRecognizer
	images     *storedFaceImages
}

// storedFaceImages reports each face image vector the face_image job stored.
type storedFaceImages struct {
	*store.FaceStore
	stored chan int64
}

func (s *storedFaceImages) StoreFaceImageVector(ctx context.Context, job domain.FaceImageJob, vector []float32) (
	domain.FaceImageDecision, error,
) {
	decision, err := s.FaceStore.StoreFaceImageVector(ctx, job, vector)
	if err == nil {
		s.stored <- job.PersonID
	}
	return decision, err
}

// newFaceAnalysisFixture analyses the scripted videos through the scheduler, which keeps running until the test ends
// and wakes on the store's events.
func newFaceAnalysisFixture(t *testing.T) faceAnalysisFixture {
	t.Helper()
	ctx := context.Background()
	db, err := store.OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	root, err := db.Settings().AddMediaFolder(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range faceVideos {
		path := filepath.Join(root.Path, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: path, Title: name, ContentKey: "key-" + name, SizeBytes: 1, MTime: time.Unix(1, 0), Container: "mp4",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Settings().SaveFaceSettings(ctx, domain.FaceSettings{
		Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8,
	}); err != nil {
		t.Fatal(err)
	}
	f := faceAnalysisFixture{db: db, images: &storedFaceImages{FaceStore: db.Faces(), stored: make(chan int64, 16)}}
	var scheduler *jobs.Scheduler
	f.recognizer = app.NewFaceRecognizer(app.FaceRecognizerOptions{
		Queue: db.Faces(), Images: f.images, Settings: db.Settings(), Runtime: readyFaceRuntime{},
		Helper: func(domain.FaceRecognizer) app.FaceHelper { return scriptedFaceHelper{} },
		Frames: scriptedFaceFrames, MediaFolders: db.Settings(), MediaFiles: mediafs.New(), Cast: db.Faces(),
		Wake: func() { scheduler.Wake() },
	})
	finished := make(chan domain.Job, 64)
	handlers := map[domain.JobKind]jobs.Handler{}
	for _, kind := range []domain.JobKind{
		domain.JobFace, domain.JobFaceImage, domain.JobFaceCast, domain.JobQueueFaceImages,
	} {
		handlers[kind] = f.recognizer.HandleJob
	}
	scheduler = jobs.NewScheduler(jobs.Options{
		Queue: db.Ingest(), Folders: db.ScanIndex(), Disks: mediafs.New().ResolveDisk, Handlers: handlers,
		CPUSlots: domain.JobCPUSlots(1), Paused: app.PausedJobKinds(f.recognizer),
		Finished: func(job domain.Job) { finished <- job }, Logger: slog.New(slog.DiscardHandler),
	})
	bus := newTestBus()
	workers := map[domain.JobKind]waker{}
	for _, kind := range domain.ScheduledJobKinds {
		workers[kind] = scheduler
	}
	subscriptions := subscribeEvents(bus, eventSubscribers{Workers: workers})
	db.PublishTo(bus)
	if n, err := f.recognizer.QueueLibrary(ctx, domain.FaceQueueMissing); err != nil || n != 3 {
		t.Fatalf("QueueLibrary = %d, %v; want 3", n, err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		scheduler.Run(runCtx)
		f.recognizer.Close()
	}()
	t.Cleanup(func() {
		subscriptions.StopWorkers()
		stop()
		<-done
		bus.Close()
	})
	waitForFaceAnalyses(t, db, finished, 3)
	return f
}

// waitForFaceAnalyses returns once n videos are analysed, checking after each job the scheduler finishes.
func waitForFaceAnalyses(t *testing.T, db *store.DB, finished <-chan domain.Job, n int) {
	t.Helper()
	for {
		counts, err := db.Faces().Counts(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if counts.Done+counts.Failed >= n {
			return
		}
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Fatalf("face analysis counts = %+v, want %d videos analysed", counts, n)
		}
	}
}

func performersOf(t *testing.T, db *store.DB, keys ...string) map[string][]int64 {
	t.Helper()
	info, err := db.WorkInfo().ByUserKeys(context.Background(), keys)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]int64{}
	for _, key := range keys {
		for _, p := range info[key].People {
			if p.Role == domain.PersonPerformer {
				out[key] = append(out[key], p.Person.ID)
			}
		}
	}
	return out
}

func provisionalPeople(t *testing.T, db *store.DB) []int64 {
	t.Helper()
	provisional := true
	page, err := db.Credits().List(context.Background(), domain.CreditPerson, domain.CreditQuery{Provisional: &provisional})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, p := range page.Items {
		ids = append(ids, p.ID)
	}
	return ids
}

// The same face in A and B makes a provisional person per video, offered with each other rather than linked.
func TestFaceAnalysisOffersTheProvisionalPeopleOfOneFaceAndIgnoresAShortFace(t *testing.T) {
	f := newFaceAnalysisFixture(t)
	people := provisionalPeople(t, f.db)
	if len(people) != 2 {
		t.Fatalf("provisional people = %v, want one for the face in each of A and B", people)
	}
	got := performersOf(t, f.db, "key-a.mp4", "key-b.mp4", "key-c.mp4")
	if len(got) != 2 || len(got["key-a.mp4"]) != 1 || len(got["key-b.mp4"]) != 1 ||
		!slices.Contains(people, got["key-a.mp4"][0]) || !slices.Contains(people, got["key-b.mp4"][0]) ||
		got["key-a.mp4"][0] == got["key-b.mp4"][0] {
		t.Fatalf("performers = %v, want one of %v in each of A and B", got, people)
	}
	pairs, err := f.db.Credits().FaceCandidates(context.Background(), people[0])
	if err != nil || len(pairs) != 1 || pairs[0].Person.ID != people[1] {
		t.Fatalf("candidates of %d = %+v, %v; want %d", people[0], pairs, err, people[1])
	}
	counts, err := f.db.Faces().Counts(context.Background())
	if err != nil || counts.Done != 3 {
		t.Fatalf("counts = %+v, %v; want all three videos done", counts, err)
	}
}

// Acceptance 4: once the owner has joined the provisional people of A and B, the owner's photo of a named person
// merges that person into them, through the face_image job the upload queues and the scheduler.
func TestAFaceImageMergesTheProvisionalPersonOfBothVideos(t *testing.T) {
	ctx := context.Background()
	f := newFaceAnalysisFixture(t)
	if people := provisionalPeople(t, f.db); len(people) != 2 {
		t.Fatalf("provisional people = %v, want two", people)
	} else if _, err := f.db.Credits().Merge(ctx, domain.CreditPerson, people[0], people[1:]); err != nil {
		t.Fatal(err)
	}
	named, err := f.db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: "Named"})
	if err != nil {
		t.Fatal(err)
	}
	var photo bytes.Buffer
	if err := png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 300, 200))); err != nil {
		t.Fatal(err)
	}
	uploads := app.NewPersonImages(f.db.Credits(), app.NewIngest(app.IngestOptions{}))
	if _, err := uploads.UploadPersonImage(ctx, named.ID, photo.Bytes()); err != nil {
		t.Fatal(err)
	}
	for id := range f.images.stored {
		if id == named.ID {
			break
		}
	}

	if people := provisionalPeople(t, f.db); len(people) != 0 {
		t.Fatalf("provisional people = %v, want the face's merged", people)
	}
	got := performersOf(t, f.db, "key-a.mp4", "key-b.mp4")
	if want := map[string][]int64{"key-a.mp4": {named.ID}, "key-b.mp4": {named.ID}}; !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("performers = %v, want the named person in both", got)
	}
}
