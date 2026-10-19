package main

import (
	"context"
	"errors"
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

// The face worker turns frames into main people and the store matches them to people, so the parent Issue's
// acceptance 2 and 3 cross both; frames and the helper are fakes scripted per video.

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

type scriptedFaceHelper struct{}

func (scriptedFaceHelper) Detect(_ context.Context, req faceproto.DetectRequest) (faceproto.Detections, error) {
	if !faceScripts[faceVideos[req.Pix[0]]](int(req.Pix[1])) {
		return faceproto.Detections{}, nil
	}
	vector := make([]float32, 128)
	vector[0] = 1
	return faceproto.Detections{Faces: []faceproto.Detection{{
		X: 500, Y: 300, W: 100, H: 100, Score: 0.95, Vector: vector,
		Landmarks: [10]float32{530, 330, 570, 330, 550, 350, 535, 375, 565, 375},
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

func TestFaceAnalysisLinksOneProvisionalPersonToBothVideosAndIgnoresAShortFace(t *testing.T) {
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
	queue := idleFaceQueue{FaceStore: db.Faces(), idle: make(chan struct{}, 1)}
	recognizer := app.NewFaceRecognizer(app.FaceRecognizerOptions{
		Queue: queue, Settings: db.Settings(), Runtime: readyFaceRuntime{},
		Helper: func(domain.FaceRecognizer) app.FaceHelper { return scriptedFaceHelper{} },
		Frames: scriptedFaceFrames, MediaFolders: db.Settings(), MediaFiles: mediafs.New(),
	})
	if n, err := recognizer.QueueLibrary(ctx, domain.FaceQueueMissing); err != nil || n != 3 {
		t.Fatalf("QueueLibrary = %d, %v; want 3", n, err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		recognizer.Run(runCtx)
	}()
	<-queue.idle
	stop()
	<-done

	provisional := true
	page, err := db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{Provisional: &provisional})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("provisional people = %+v, want one for the face of A and B", page.Items)
	}
	person := page.Items[0].ID
	info, err := db.WorkInfo().ByUserKeys(ctx, []string{"key-a.mp4", "key-b.mp4", "key-c.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	for key, linked := range map[string]bool{"key-a.mp4": true, "key-b.mp4": true, "key-c.mp4": false} {
		var performers, want []int64
		if linked {
			want = []int64{person}
		}
		for _, p := range info[key].People {
			if p.Role == domain.PersonPerformer {
				performers = append(performers, p.Person.ID)
			}
		}
		if !slices.Equal(performers, want) {
			t.Fatalf("performers of %s = %v, want %v", key, performers, want)
		}
	}
	counts, err := db.Faces().Counts(ctx)
	if err != nil || counts.Done != 3 {
		t.Fatalf("counts = %+v, %v; want all three videos done", counts, err)
	}
}
