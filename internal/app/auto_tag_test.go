package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type fakeAutoTagQueue struct {
	mu            sync.Mutex
	queued        []string
	videos        map[string]domain.Video
	candidates    []domain.AutoTagCandidate
	candidatesFor []string
	finished      map[string]domain.AutoTagSelection
	failed        map[string]string
	queuedIDs     []int64
	modes         []domain.AutoTagQueueMode
	// queueDeciders track initiators; finishDeciders track decision sources.
	queueDeciders  []domain.ActivityDecider
	finishDeciders map[string]domain.ActivityDecider
	// retried holds {videoID, entryID} per RetryVideo call.
	retried [][2]int64
	done    chan string
}

func newFakeAutoTagQueue() *fakeAutoTagQueue {
	return &fakeAutoTagQueue{
		videos:         map[string]domain.Video{},
		finished:       map[string]domain.AutoTagSelection{},
		finishDeciders: map[string]domain.ActivityDecider{},
		failed:         map[string]string{},
		done:           make(chan string, 10),
	}
}

func (q *fakeAutoTagQueue) QueueVideos(
	_ context.Context, ids []int64, mode domain.AutoTagQueueMode, decider domain.ActivityDecider,
) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queuedIDs = append(q.queuedIDs, ids...)
	q.modes = append(q.modes, mode)
	q.queueDeciders = append(q.queueDeciders, decider)
	return len(ids), nil
}

func (q *fakeAutoTagQueue) RetryVideo(_ context.Context, videoID, entryID int64) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.retried = append(q.retried, [2]int64{videoID, entryID})
	return 1, nil
}

func (q *fakeAutoTagQueue) QueueLibrary(_ context.Context, mode domain.AutoTagQueueMode, decider domain.ActivityDecider) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.modes = append(q.modes, mode)
	q.queueDeciders = append(q.queueDeciders, decider)
	return 3, nil
}

func (q *fakeAutoTagQueue) Claim(context.Context) (domain.AutoTagJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queued) == 0 {
		return domain.AutoTagJob{}, domain.ErrNoJob
	}
	key := q.queued[0]
	q.queued = q.queued[1:]
	return domain.AutoTagJob{ContentKey: key}, nil
}

func (q *fakeAutoTagQueue) Subject(_ context.Context, key string) (domain.AutoTagVideo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	video, ok := q.videos[key]
	if !ok {
		return domain.AutoTagVideo{}, domain.ErrNotFound
	}
	return domain.AutoTagVideo{Video: video, MediaFolder: filepath.FromSlash("/media")}, nil
}

func (q *fakeAutoTagQueue) Candidates(_ context.Context, key string) ([]domain.AutoTagCandidate, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.candidatesFor = append(q.candidatesFor, key)
	return q.candidates, nil
}

func (q *fakeAutoTagQueue) ConfirmTentative(context.Context, int64, int64) error { return nil }
func (q *fakeAutoTagQueue) RejectTentative(context.Context, int64, int64) error  { return nil }

func (q *fakeAutoTagQueue) Finish(
	_ context.Context, key string, selection domain.AutoTagSelection, decider domain.ActivityDecider,
) error {
	q.mu.Lock()
	q.finished[key] = selection
	q.finishDeciders[key] = decider
	q.mu.Unlock()
	q.done <- key
	return nil
}

func (q *fakeAutoTagQueue) Fail(_ context.Context, key, reason string) error {
	q.mu.Lock()
	q.failed[key] = reason
	q.mu.Unlock()
	q.done <- key
	return nil
}

func (q *fakeAutoTagQueue) RequeueRunning(context.Context) (int, error) { return 0, nil }

func (q *fakeAutoTagQueue) Counts(context.Context) (domain.AutoTagCounts, error) {
	return domain.AutoTagCounts{Done: 1}, nil
}

type fakeAutoTagSettings struct {
	mu       sync.Mutex
	settings domain.AutoTagSettings
}

func (s *fakeAutoTagSettings) AutoTagSettings(context.Context) (domain.AutoTagSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings, nil
}

func (s *fakeAutoTagSettings) SaveAutoTagSettings(_ context.Context, settings domain.AutoTagSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return nil
}

func enabledAutoTagSettings() *fakeAutoTagSettings {
	settings := domain.DefaultAutoTagSettings()
	settings.Enabled = true
	return &fakeAutoTagSettings{settings: settings}
}

type fakeClassifier struct {
	mu       sync.Mutex
	answers  map[string]float64
	err      error
	requests []domain.ClassifierRequest
}

func (c *fakeClassifier) Classify(_ context.Context, req domain.ClassifierRequest) (map[string]float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if c.err != nil {
		return nil, c.err
	}
	out := map[string]float64{}
	for _, q := range req.Questions {
		if p, ok := c.answers[q.Key]; ok {
			out[q.Key] = p
		}
	}
	return out, nil
}

type fakeImages struct {
	thumbnails map[string][]byte
	frames     map[string][][]byte
	asked      []int
}

func (f *fakeImages) ThumbnailJPEG(key string) ([]byte, error) {
	data, ok := f.thumbnails[key]
	if !ok {
		return nil, errors.New("no thumbnail")
	}
	return data, nil
}

func (f *fakeImages) SeekFramesJPEG(key string, n int) ([][]byte, error) {
	f.asked = append(f.asked, n)
	frames, ok := f.frames[key]
	if !ok {
		return nil, errors.New("no frames")
	}
	return frames, nil
}

func runAutoTagger(t *testing.T, tagger *AutoTagger) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tagger.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func waitAutoTagDone(t *testing.T, q *fakeAutoTagQueue) string {
	t.Helper()
	select {
	case key := <-q.done:
		return key
	case <-time.After(5 * time.Second):
		t.Fatal("判定が終わらない")
		return ""
	}
}

// Send all candidate questions together; provider adapters own batching.
func TestAutoTaggerSelectsTags(t *testing.T) {
	queue := newFakeAutoTagQueue()
	queue.queued = []string{"k1"}
	queue.videos["k1"] = domain.Video{
		Title: "Kyoto", Path: filepath.FromSlash("/media/travel/bamboo.mp4"), ThumbnailState: domain.ThumbnailStateDone,
		SeekThumbnailState: domain.SeekThumbnailDone,
	}
	for id := int64(1); id <= 40; id++ {
		queue.candidates = append(queue.candidates, domain.AutoTagCandidate{ID: id, Name: "t"})
	}
	classifier := &fakeClassifier{answers: map[string]float64{
		"tag_1": 0.95, "tag_2": 0.5, "tag_3": 0.3, "tag_40": 0.9,
	}}
	settings := &fakeAutoTagSettings{settings: domain.AutoTagSettings{
		Enabled: true, Threshold: 0.8, TentativeThreshold: 0.5,
	}}
	images := &fakeImages{
		thumbnails: map[string][]byte{"k1": []byte("thumb")},
		frames:     map[string][][]byte{"k1": {[]byte("f1"), []byte("f2")}},
	}
	tagger := NewAutoTagger(AutoTaggerOptions{
		Queue: queue, Settings: settings, Classifier: classifier, Images: images,
	})
	runAutoTagger(t, tagger)

	waitAutoTagDone(t, queue)
	want := domain.AutoTagSelection{
		Confirmed: []int64{1, 40}, Tentative: []int64{2}, Probabilities: map[int64]float64{1: 0.95, 2: 0.5, 40: 0.9},
	}
	if got := queue.finished["k1"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("付けたタグ = %v", got)
	}
	if got := queue.finishDeciders["k1"]; got != domain.ActivityByAI {
		t.Fatalf("判断したもの = %q", got)
	}
	if len(classifier.requests) != 1 || len(classifier.requests[0].Questions) != 40 {
		t.Fatalf("問い合わせ = %d 件", len(classifier.requests))
	}
	if !slices.Equal(queue.candidatesFor, []string{"k1"}) {
		t.Fatalf("候補を聞いた動画 = %v", queue.candidatesFor)
	}
	wantInput := domain.ClassifierInput{
		Facts:  map[string]string{"title": "Kyoto", "file_name": "bamboo.mp4", "folder": "travel"},
		Images: [][]byte{[]byte("thumb"), []byte("f1"), []byte("f2")},
	}
	if got := classifier.requests[0].Input; !reflect.DeepEqual(got, wantInput) {
		t.Fatalf("入力 = %+v", got)
	}
	if !slices.Equal(images.asked, []int{domain.AutoTagFrameCount}) {
		t.Fatalf("コマを読んだ回数と数 = %v", images.asked)
	}
}

func TestAutoTaggerRecordsFailuresAndContinues(t *testing.T) {
	queue := newFakeAutoTagQueue()
	queue.queued = []string{"k1", "k2"}
	queue.videos["k1"] = domain.Video{Title: "a", Path: "/m/a.mp4"}
	queue.videos["k2"] = domain.Video{Title: "b", Path: "/m/b.mp4"}
	queue.candidates = []domain.AutoTagCandidate{{ID: 1, Name: "x"}}
	classifier := &fakeClassifier{err: errors.New("cannot reach Ollama")}
	tagger := NewAutoTagger(AutoTaggerOptions{
		Queue: queue, Settings: enabledAutoTagSettings(),
		Classifier: classifier, Images: &fakeImages{},
	})
	runAutoTagger(t, tagger)

	waitAutoTagDone(t, queue)
	waitAutoTagDone(t, queue)
	if queue.failed["k1"] != "cannot reach Ollama" || queue.failed["k2"] != "cannot reach Ollama" {
		t.Fatalf("失敗の記録 = %v", queue.failed)
	}
	if classifier.requests[0].Input.Images != nil {
		t.Fatal("サムネイルもコマも無い動画に画像を渡した")
	}
}

func TestAutoTaggerSkipsWithoutSubjectOrCandidates(t *testing.T) {
	queue := newFakeAutoTagQueue()
	queue.queued = []string{"gone", "k1"}
	queue.videos["k1"] = domain.Video{Title: "a", Path: "/m/a.mp4"}
	classifier := &fakeClassifier{}
	tagger := NewAutoTagger(AutoTaggerOptions{
		Queue: queue, Settings: enabledAutoTagSettings(), Classifier: classifier,
	})
	runAutoTagger(t, tagger)
	waitAutoTagDone(t, queue)
	waitAutoTagDone(t, queue)
	if len(classifier.requests) != 0 {
		t.Fatalf("問い合わせた: %d", len(classifier.requests))
	}
	if _, ok := queue.finished["gone"]; !ok {
		t.Fatal("消えた動画の判定を終えていない")
	}
	// No classifier call means no decider change.
	if got := queue.finishDeciders["k1"]; got != "" {
		t.Fatalf("聞いていない実行の判断したもの = %q", got)
	}
}

// The seek stage's transaction queues after-import tagging, so the subscriber only wakes the worker.
func TestAutoTaggerVideoImagesFinishedOnlyWakes(t *testing.T) {
	queue := newFakeAutoTagQueue()
	settings := &fakeAutoTagSettings{settings: domain.AutoTagSettings{Enabled: true, AfterImport: true}}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: settings, Classifier: &fakeClassifier{}})

	tagger.VideoImagesFinished(context.Background(), 5)
	if len(tagger.wake) != 1 {
		t.Fatal("ワーカーを起こしていない")
	}
	if len(queue.queuedIDs) != 0 || len(queue.modes) != 0 {
		t.Fatalf("キューを呼んだ: %v, %v", queue.queuedIDs, queue.modes)
	}
}

func TestAutoTaggerQueuesOnlyWhenEnabled(t *testing.T) {
	queue := newFakeAutoTagQueue()
	settings := &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: settings, Classifier: &fakeClassifier{}})
	ctx := context.Background()

	if _, err := tagger.QueueVideo(ctx, 6); !errors.Is(err, domain.ErrAutoTaggingOff) {
		t.Fatalf("QueueVideo err = %v", err)
	}
	if _, err := tagger.QueueLibrary(ctx, domain.AutoTagScopeMissing); !errors.Is(err, domain.ErrAutoTaggingOff) {
		t.Fatalf("QueueLibrary err = %v", err)
	}
	settings.settings = domain.AutoTagSettings{Enabled: true, Threshold: 0.8, TentativeThreshold: 0.5}
	if queued, err := tagger.QueueVideo(ctx, 6); err != nil || !queued || queue.modes[0] != domain.AutoTagQueueAgain {
		t.Fatalf("QueueVideo = %v, %v, %v", queued, err, queue.modes)
	}
	if _, err := tagger.QueueLibrary(ctx, domain.AutoTagScope("bogus")); err == nil {
		t.Fatal("未知の範囲を受け付けた")
	}
	if n, err := tagger.QueueLibrary(ctx, domain.AutoTagScopeMissing); err != nil || n != 3 || queue.modes[1] != domain.AutoTagQueueUnlessDone {
		t.Fatalf("QueueLibrary = %d, %v, %v", n, err, queue.modes)
	}
	// Settings and video-page requests use owner.
	wantDeciders := []domain.ActivityDecider{domain.ActivityByOwner, domain.ActivityByOwner}
	if !slices.Equal(queue.queueDeciders, wantDeciders) {
		t.Fatalf("始めたもの = %v", queue.queueDeciders)
	}
}

func TestAutoTaggerIdlesWhileOff(t *testing.T) {
	queue := newFakeAutoTagQueue()
	queue.queued = []string{"k1"}
	queue.videos["k1"] = domain.Video{Title: "a", Path: "/m/a.mp4"}
	queue.candidates = []domain.AutoTagCandidate{{ID: 1, Name: "x"}}
	settings := &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}
	classifier := &fakeClassifier{answers: map[string]float64{"tag_1": 0.9}}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: settings, Classifier: classifier})
	runAutoTagger(t, tagger)

	select {
	case key := <-queue.done:
		t.Fatalf("切なのに %s を判定した", key)
	case <-time.After(100 * time.Millisecond):
	}
	on := domain.DefaultAutoTagSettings()
	on.Enabled = true
	if _, err := tagger.SaveSettings(context.Background(), on); err != nil {
		t.Fatal(err)
	}
	if key := waitAutoTagDone(t, queue); key != "k1" {
		t.Fatalf("判定した = %s", key)
	}
}

func TestAutoTaggerSaveSettings(t *testing.T) {
	settings := &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: newFakeAutoTagQueue(), Settings: settings, Classifier: &fakeClassifier{}})
	ctx := context.Background()

	if _, err := tagger.SaveSettings(ctx, domain.AutoTagSettings{Threshold: 0.5, TentativeThreshold: 0.6}); !errors.Is(err, domain.ErrInvalidAutoTagSettings) {
		t.Fatalf("err = %v", err)
	}
	want := domain.AutoTagSettings{Enabled: true, AfterImport: true, Threshold: 0.5, TentativeThreshold: 0.3}
	status, err := tagger.SaveSettings(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if status.Settings != want || status.Counts.Done != 1 || settings.settings != want {
		t.Fatalf("status = %+v", status)
	}
}

func TestAutoTaggerSkipsWhenCandidatesVanish(t *testing.T) {
	queue := &vanishingQueue{fakeAutoTagQueue: newFakeAutoTagQueue()}
	queue.queued = []string{"k1"}
	queue.videos["k1"] = domain.Video{Title: "a", Path: "/m/a.mp4"}
	classifier := &fakeClassifier{}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: enabledAutoTagSettings(), Classifier: classifier})
	runAutoTagger(t, tagger)
	waitAutoTagDone(t, queue.fakeAutoTagQueue)
	if len(classifier.requests) != 0 {
		t.Fatalf("問い合わせた: %d", len(classifier.requests))
	}
}

type vanishingQueue struct{ *fakeAutoTagQueue }

func (vanishingQueue) Candidates(context.Context, string) ([]domain.AutoTagCandidate, error) {
	return nil, domain.ErrNotFound
}

func TestAutoTaggerCheckClassifier(t *testing.T) {
	settings := &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}
	answering := &fakeClassifier{answers: map[string]float64{"check": 0.9}}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: newFakeAutoTagQueue(), Settings: settings, Classifier: answering})
	if err := tagger.CheckClassifier(context.Background()); err != nil || len(answering.requests) != 1 || len(answering.requests[0].Questions) != 1 {
		t.Fatalf("CheckClassifier = %v, requests = %+v", err, answering.requests)
	}
	silent := NewAutoTagger(AutoTaggerOptions{Queue: newFakeAutoTagQueue(), Settings: settings, Classifier: &fakeClassifier{}})
	if err := silent.CheckClassifier(context.Background()); err == nil {
		t.Fatal("答えの無い判定を成功にした")
	}
	failing := NewAutoTagger(AutoTaggerOptions{Queue: newFakeAutoTagQueue(), Settings: settings,
		Classifier: &fakeClassifier{err: errors.New("model 'clef' not found")}})
	if err := failing.CheckClassifier(context.Background()); err == nil || err.Error() != "model 'clef' not found" {
		t.Fatalf("err = %v", err)
	}
}

type fakeAutoTagUndo struct{ undone [][2]int64 }

func (f *fakeAutoTagUndo) UndoAutoTag(_ context.Context, entryID, tagID int64) (domain.ActivityEntry, error) {
	f.undone = append(f.undone, [2]int64{entryID, tagID})
	return domain.ActivityEntry{ID: 50, Kind: domain.ActivityUndo, UndoesID: entryID}, nil
}

func TestAutoTaggerRetriesAndUndoesActivity(t *testing.T) {
	queue := newFakeAutoTagQueue()
	settings := &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}
	undo := &fakeAutoTagUndo{}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: settings, Undo: undo, Classifier: &fakeClassifier{}})
	activity := NewActivity(ActivityOptions{
		Store: &fakeActivityLog{entries: map[int64]domain.ActivityEntry{
			1: {ID: 1, Kind: domain.ActivityAutoTag, Status: domain.ActivityAttention, Reason: domain.ActivityFailed,
				VideoIDs: [2]int64{6}},
			2: {ID: 2, Kind: domain.ActivityAutoTag, Status: domain.ActivityAttention, Reason: domain.ActivityFailed},
			3: {ID: 3, Kind: domain.ActivityAutoTag, Status: domain.ActivityDone, Decider: domain.ActivityByAI,
				Tags: []domain.ActivityEntryTag{{TagID: 7}}},
		}},
		Retriers: map[domain.ActivityKind]ActivityRetrier{domain.ActivityAutoTag: tagger},
		Undoers:  map[domain.ActivityKind]ActivityUndoer{domain.ActivityAutoTag: tagger},
	})
	ctx := context.Background()

	if err := activity.RetryActivity(ctx, 1); !errors.Is(err, domain.ErrAutoTaggingOff) {
		t.Fatalf("切のやり直し = %v, want ErrAutoTaggingOff", err)
	}
	if len(queue.retried) != 0 {
		t.Fatalf("切なのに積んだ: %v", queue.retried)
	}
	settings.settings.Enabled = true
	if err := activity.RetryActivity(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// The retry names the retried entry so the store can join its pipelines.
	if !slices.Equal(queue.retried, [][2]int64{{6, 1}}) {
		t.Fatalf("やり直し = %v", queue.retried)
	}
	if err := activity.RetryActivity(ctx, 2); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Fatalf("動画の無い記録のやり直し = %v", err)
	}

	got, err := activity.UndoActivity(ctx, 3, domain.ActivityUndoTarget{TagID: 7})
	if err != nil || got.UndoesID != 3 || !slices.Equal(undo.undone, [][2]int64{{3, 7}}) {
		t.Fatalf("取り消し = %+v, %v, %v", got, err, undo.undone)
	}
	if _, err := activity.UndoActivity(ctx, 3, domain.ActivityUndoTarget{TagID: 8}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("付けていないタグの取り消し = %v", err)
	}
}
