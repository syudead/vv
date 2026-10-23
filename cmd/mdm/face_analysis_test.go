package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
	"github.com/syudead/vv/internal/mediafs"
	"github.com/syudead/vv/internal/store"
)

// The face worker turns frames and faces into vectors and the store matches them to people, so the parent Issue's
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

// idleFaceQueue reports when the worker finds the queue empty, which is when every queued video is done.
type idleFaceQueue struct {
	*store.FaceStore
	idle chan struct{}
}

func (q idleFaceQueue) Claim(ctx context.Context) (domain.FaceJob, error) {
	job, err := q.FaceStore.Claim(ctx)
	if errors.Is(err, domain.ErrNoJob) {
		select {
		case q.idle <- struct{}{}:
		default:
		}
	}
	return job, err
}

// faceAnalysisFixture is the store with the scripted videos queued, and a worker that has analysed them.
type faceAnalysisFixture struct {
	db         *store.DB
	recognizer *app.FaceRecognizer
	queue      idleFaceQueue
	images     *storedFaceImages
}

// storedFaceImages reports each face image vector the worker stored.
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

// newFaceAnalysisFixture analyses the scripted videos; the worker keeps running until the test ends.
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
	f := faceAnalysisFixture{
		db: db, queue: idleFaceQueue{FaceStore: db.Faces(), idle: make(chan struct{}, 1)},
		images: &storedFaceImages{FaceStore: db.Faces(), stored: make(chan int64, 16)},
	}
	f.recognizer = app.NewFaceRecognizer(app.FaceRecognizerOptions{
		Queue: f.queue, Images: f.images, Settings: db.Settings(), Runtime: readyFaceRuntime{},
		Helper: func(domain.FaceRecognizer) app.FaceHelper { return scriptedFaceHelper{} },
		Frames: scriptedFaceFrames, MediaFolders: db.Settings(), MediaFiles: mediafs.New(),
	})
	if n, err := f.recognizer.QueueLibrary(ctx, domain.FaceQueueMissing); err != nil || n != 3 {
		t.Fatalf("QueueLibrary = %d, %v; want 3", n, err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.recognizer.Run(runCtx)
	}()
	t.Cleanup(func() {
		stop()
		<-done
	})
	<-f.queue.idle
	return f
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

func TestFaceAnalysisLinksOneProvisionalPersonToBothVideosAndIgnoresAShortFace(t *testing.T) {
	f := newFaceAnalysisFixture(t)
	people := provisionalPeople(t, f.db)
	if len(people) != 1 {
		t.Fatalf("provisional people = %v, want one for the face of A and B", people)
	}
	got := performersOf(t, f.db, "key-a.mp4", "key-b.mp4", "key-c.mp4")
	if want := map[string][]int64{"key-a.mp4": people, "key-b.mp4": people}; !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("performers = %v, want %v", got, want)
	}
	counts, err := f.db.Faces().Counts(context.Background())
	if err != nil || counts.Done != 3 {
		t.Fatalf("counts = %+v, %v; want all three videos done", counts, err)
	}
}

// Acceptance 4: the owner's photo of a named person merges the provisional person of A and B into them, through the
// face event and the worker.
func TestAFaceImageMergesTheProvisionalPersonOfBothVideos(t *testing.T) {
	ctx := context.Background()
	f := newFaceAnalysisFixture(t)
	bus := newTestBus()
	subscriptions := subscribeEvents(bus, eventSubscribers{PersonImageChanged: f.recognizer.PersonImageChanged})
	t.Cleanup(subscriptions.StopWorkers)
	f.db.PublishTo(bus)
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
