package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeAutoTagQueue は待ち行列を記憶だけで真似る。
type fakeAutoTagQueue struct {
	mu         sync.Mutex
	queued     []string
	videos     map[string]domain.Video
	candidates []domain.AutoTagCandidate
	finished   map[string][]int64
	failed     map[string]string
	queuedIDs  []int64
	modes      []domain.AutoTagQueueMode
	done       chan string
}

func newFakeAutoTagQueue() *fakeAutoTagQueue {
	return &fakeAutoTagQueue{
		videos:   map[string]domain.Video{},
		finished: map[string][]int64{},
		failed:   map[string]string{},
		done:     make(chan string, 10),
	}
}

func (q *fakeAutoTagQueue) QueueVideos(_ context.Context, ids []int64, mode domain.AutoTagQueueMode) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queuedIDs = append(q.queuedIDs, ids...)
	q.modes = append(q.modes, mode)
	return len(ids), nil
}

func (q *fakeAutoTagQueue) QueueLibrary(_ context.Context, mode domain.AutoTagQueueMode) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.modes = append(q.modes, mode)
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

func (q *fakeAutoTagQueue) Subject(_ context.Context, key string) (domain.Video, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	video, ok := q.videos[key]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return video, nil
}

func (q *fakeAutoTagQueue) Candidates(context.Context) ([]domain.AutoTagCandidate, error) {
	return q.candidates, nil
}

func (q *fakeAutoTagQueue) Finish(_ context.Context, key string, tagIDs []int64) error {
	q.mu.Lock()
	q.finished[key] = tagIDs
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

// fakeClassifier は質問の名前ごとに決めた確率を返し、受け取った問い合わせを覚える。
type fakeClassifier struct {
	mu       sync.Mutex
	answers  map[string]float64
	err      error
	requests []domain.AutoTagRequest
}

func (c *fakeClassifier) Classify(_ context.Context, req domain.AutoTagRequest) (map[string]float64, error) {
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

type fakeThumbnails map[string][]byte

func (f fakeThumbnails) ThumbnailJPEG(key string) ([]byte, error) {
	data, ok := f[key]
	if !ok {
		return nil, errors.New("no thumbnail")
	}
	return data, nil
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

// 確率が閾値以上のタグを付け、候補が上限を超えれば分けて問い合わせ、サムネイルと題名・
// ファイル名・フォルダを渡す。
func TestAutoTaggerSelectsTagsAcrossChunks(t *testing.T) {
	queue := newFakeAutoTagQueue()
	queue.queued = []string{"k1"}
	queue.videos["k1"] = domain.Video{
		Title: "Kyoto", Path: "/media/travel/bamboo.mp4", ThumbnailState: domain.ThumbnailStateDone,
	}
	for id := int64(1); id <= domain.AutoTagMaxQuestions+2; id++ {
		queue.candidates = append(queue.candidates, domain.AutoTagCandidate{ID: id, Name: "t"})
	}
	classifier := &fakeClassifier{answers: map[string]float64{
		"tag_1": 0.95, "tag_2": 0.5, domain.AutoTagQuestionKey(domain.AutoTagMaxQuestions + 2): 0.9,
	}}
	settings := &fakeAutoTagSettings{settings: domain.AutoTagSettings{Endpoint: "http://o", Model: "clef-flash", Threshold: 0.8}}
	tagger := NewAutoTagger(AutoTaggerOptions{
		Queue: queue, Settings: settings, Classifier: classifier, Thumbnails: fakeThumbnails{"k1": []byte("jpeg")},
	})
	runAutoTagger(t, tagger)

	waitAutoTagDone(t, queue)
	if got := queue.finished["k1"]; !reflect.DeepEqual(got, []int64{1, domain.AutoTagMaxQuestions + 2}) {
		t.Fatalf("付けたタグ = %v", got)
	}
	if len(classifier.requests) != 2 {
		t.Fatalf("問い合わせの回数 = %d", len(classifier.requests))
	}
	req := classifier.requests[0]
	want := domain.AutoTagSubject{Title: "Kyoto", FileName: "bamboo.mp4", Folder: "/media/travel", Thumbnail: []byte("jpeg")}
	if req.Endpoint != "http://o" || req.Model != "clef-flash" || !reflect.DeepEqual(req.Subject, want) {
		t.Fatalf("問い合わせ = %+v", req)
	}
}

// 判定モデルに問い合わせられなければ失敗として理由を残し、次の判定へ進む。サムネイルが
// 無い動画は文字だけで判定する。
func TestAutoTaggerRecordsFailuresAndContinues(t *testing.T) {
	queue := newFakeAutoTagQueue()
	queue.queued = []string{"k1", "k2"}
	queue.videos["k1"] = domain.Video{Title: "a", Path: "/m/a.mp4"}
	queue.videos["k2"] = domain.Video{Title: "b", Path: "/m/b.mp4"}
	queue.candidates = []domain.AutoTagCandidate{{ID: 1, Name: "x"}}
	classifier := &fakeClassifier{err: errors.New("cannot reach Ollama")}
	tagger := NewAutoTagger(AutoTaggerOptions{
		Queue: queue, Settings: &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()},
		Classifier: classifier, Thumbnails: fakeThumbnails{},
	})
	runAutoTagger(t, tagger)

	waitAutoTagDone(t, queue)
	waitAutoTagDone(t, queue)
	if queue.failed["k1"] != "cannot reach Ollama" || queue.failed["k2"] != "cannot reach Ollama" {
		t.Fatalf("失敗の記録 = %v", queue.failed)
	}
	if classifier.requests[0].Subject.Thumbnail != nil {
		t.Fatal("サムネイルの無い動画に画像を渡した")
	}
}

// 消えた動画とタグの無いライブラリは、問い合わせずに終える。
func TestAutoTaggerSkipsWithoutSubjectOrCandidates(t *testing.T) {
	queue := newFakeAutoTagQueue()
	queue.queued = []string{"gone", "k1"}
	queue.videos["k1"] = domain.Video{Title: "a", Path: "/m/a.mp4"}
	classifier := &fakeClassifier{}
	tagger := NewAutoTagger(AutoTaggerOptions{
		Queue: queue, Settings: &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}, Classifier: classifier,
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
}

// 取り込みのあとの自動の判定は、入のときだけ、まだ判定していない動画を積む。
func TestAutoTaggerQueuesAfterThumbnailOnlyWhenEnabled(t *testing.T) {
	queue := newFakeAutoTagQueue()
	settings := &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: queue, Settings: settings, Classifier: &fakeClassifier{}})
	ctx := context.Background()

	tagger.VideoThumbnailFinished(ctx, 5)
	if len(queue.queuedIDs) != 0 {
		t.Fatal("切なのに積んだ")
	}
	settings.settings.Enabled = true
	tagger.VideoThumbnailFinished(ctx, 5)
	if !slices.Equal(queue.queuedIDs, []int64{5}) || queue.modes[0] != domain.AutoTagQueueIfNew {
		t.Fatalf("積んだ = %v, %v", queue.queuedIDs, queue.modes)
	}
	if queued, err := tagger.QueueVideo(ctx, 6); err != nil || !queued || queue.modes[1] != domain.AutoTagQueueAgain {
		t.Fatalf("QueueVideo = %v, %v, %v", queued, err, queue.modes)
	}
	if _, err := tagger.QueueLibrary(ctx, domain.AutoTagScope("bogus")); err == nil {
		t.Fatal("未知の範囲を受け付けた")
	}
	if n, err := tagger.QueueLibrary(ctx, domain.AutoTagScopeMissing); err != nil || n != 3 || queue.modes[2] != domain.AutoTagQueueUnlessDone {
		t.Fatalf("QueueLibrary = %d, %v, %v", n, err, queue.modes)
	}
}

// 設定は確かめてから保存し、規則に合わなければ保存しない。確かめは保存した設定で問い合わせる。
func TestAutoTaggerSaveSettingsAndCheck(t *testing.T) {
	settings := &fakeAutoTagSettings{settings: domain.DefaultAutoTagSettings()}
	classifier := &fakeClassifier{}
	tagger := NewAutoTagger(AutoTaggerOptions{Queue: newFakeAutoTagQueue(), Settings: settings, Classifier: classifier})
	ctx := context.Background()

	if _, err := tagger.SaveSettings(ctx, domain.AutoTagSettings{Endpoint: "nope", Model: "clef", Threshold: 0.5}); !errors.Is(err, domain.ErrInvalidAutoTagSettings) {
		t.Fatalf("err = %v", err)
	}
	status, err := tagger.SaveSettings(ctx, domain.AutoTagSettings{Enabled: true, Endpoint: "http://h:1/", Model: "clef", Threshold: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.AutoTagSettings{Enabled: true, Endpoint: "http://h:1", Model: "clef", Threshold: 0.5}
	if status.Settings != want || status.Counts.Done != 1 {
		t.Fatalf("status = %+v", status)
	}
	if err := tagger.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if classifier.requests[0].Endpoint != "http://h:1" || classifier.requests[0].Model != "clef" || settings.settings != want {
		t.Fatalf("確かめが保存した設定を使わない: %+v / %+v", classifier.requests[0], settings.settings)
	}
}
