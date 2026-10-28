package app

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The in-memory fake keeps the inputs of claimable pairs and the results applied to them.
type fakeVersionJudgeStore struct {
	mu        sync.Mutex
	inputs    map[[2]string]domain.VersionJudgeJob
	results   []domain.VersionJudgeResult
	recovered int
	reset     int

	inputErr error
	applyErr error
	// resetErr fails the next reset, once.
	resetErr error
}

func newFakeVersionJudgeStore(jobs ...domain.VersionJudgeJob) *fakeVersionJudgeStore {
	s := &fakeVersionJudgeStore{inputs: map[[2]string]domain.VersionJudgeJob{}}
	for _, job := range jobs {
		s.inputs[job.Keys] = job
	}
	return s
}

func (s *fakeVersionJudgeStore) JudgementInput(_ context.Context, keys [2]string) (domain.VersionJudgeJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inputErr != nil {
		return domain.VersionJudgeJob{}, s.inputErr
	}
	return s.inputs[keys], nil
}

func (s *fakeVersionJudgeStore) ApplyJudgement(_ context.Context, _ [2]string, result domain.VersionJudgeResult) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applyErr != nil {
		return false, s.applyErr
	}
	s.results = append(s.results, result)
	return true, nil
}

func (s *fakeVersionJudgeStore) RecoverRunningJudgements(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recovered++
	return 1, nil
}

func (s *fakeVersionJudgeStore) ResetNoConnectionJudgements(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.resetErr; err != nil {
		s.resetErr = nil
		return 0, err
	}
	s.reset++
	return 1, nil
}

func (s *fakeVersionJudgeStore) lastResult(t *testing.T) domain.VersionJudgeResult {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		t.Fatal("結果を反映していない")
	}
	return s.results[len(s.results)-1]
}

func (s *fakeVersionJudgeStore) resultCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.results)
}

type fakeVersionJudgeSettings struct {
	mu        sync.Mutex
	enabled   bool
	connected bool
	// savedAfterRead turns connected on after the first read, as a save made while a judgement runs does.
	savedAfterRead bool
}

func (s *fakeVersionJudgeSettings) VersionJudgingEnabled(context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled, nil
}

func (s *fakeVersionJudgeSettings) SaveVersionJudgingEnabled(_ context.Context, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	return nil
}

func (s *fakeVersionJudgeSettings) OllamaConnectionSaved(context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	connected := s.connected
	if s.savedAfterRead {
		s.connected = true
	}
	return connected, nil
}

type fakeVersionFrames struct {
	keys  [2]string
	pairs []domain.FramePair
}

func (f *fakeVersionFrames) SeekFramePairs(keyA, keyB string, pairs []domain.FramePair) ([][]byte, error) {
	f.keys, f.pairs = [2]string{keyA, keyB}, pairs
	images := make([][]byte, len(pairs))
	for i := range pairs {
		images[i] = []byte{byte(i)}
	}
	return images, nil
}

// Only differing frame indexes have distance 33 in the twenty-frame fingerprints.
func versionJudgeJob(differing ...int) domain.VersionJudgeJob {
	a := domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: 1000}
	b := domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: 1000}
	for i := range 20 {
		a.Frames = append(a.Frames, domain.FrameHash{})
		var hash uint64
		for _, d := range differing {
			if d == i {
				hash = 1<<33 - 1
			}
		}
		b.Frames = append(b.Frames, domain.FrameHash{Hash: hash})
	}
	return domain.VersionJudgeJob{Keys: [2]string{"key-a", "key-b"}, Fingerprints: [2]domain.Fingerprint{a, b}}
}

func newTestVersionJudge(store *fakeVersionJudgeStore, classifier *fakeClassifier) (*VersionJudge, *fakeVersionJudgeSettings, *fakeVersionFrames) {
	settings := &fakeVersionJudgeSettings{enabled: true, connected: true}
	frames := &fakeVersionFrames{}
	judge := NewVersionJudge(VersionJudgeOptions{Store: store, Settings: settings, Classifier: classifier, Frames: frames})
	return judge, settings, frames
}

func handleVersionJudge(t *testing.T, judge *VersionJudge, keys [2]string) error {
	t.Helper()
	return judge.HandleJob(context.Background(),
		domain.Job{Kind: domain.JobVersionJudge, ContentKey: keys[0], ContentKeyB: keys[1], Attempts: 1})
}

func TestVersionJudgeMatchingPairBundlesWithoutClassifier(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob())
	classifier := &fakeClassifier{}
	judge, _, _ := newTestVersionJudge(store, classifier)

	if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); err != nil {
		t.Fatal(err)
	}
	// Rule-based results omit probability.
	if got := store.lastResult(t); got.Verdict != domain.VersionSame || got.Decider != domain.ActivityByRule ||
		got.Probability != nil {
		t.Errorf("結果 = %+v, want rule の same", got)
	}
	if len(classifier.requests) != 0 {
		t.Errorf("分類器を呼んだ: %d 回", len(classifier.requests))
	}
}

func TestVersionJudgeDifferingPairAsksAboutTheRun(t *testing.T) {
	for _, c := range []struct {
		probability float64
		want        domain.VersionVerdict
	}{
		{0.85, domain.VersionSame},
		{0.1, domain.VersionDifferent},
		{0.5, domain.VersionUndecided},
	} {
		t.Run(c.want.String(), func(t *testing.T) {
			store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
			classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: c.probability}}
			judge, _, frames := newTestVersionJudge(store, classifier)

			if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); err != nil {
				t.Fatal(err)
			}

			got := store.lastResult(t)
			if got.Verdict != c.want || got.Probability == nil || *got.Probability != c.probability {
				t.Fatalf("結果 = %+v, want %v（確率 %v）", got, c.want, c.probability)
			}
			// AI decisions retain probabilities in activity records.
			if got.Decider != domain.ActivityByAI {
				t.Errorf("判断したもの = %q, want ai", got.Decider)
			}
			if e := got.Evidence; e == nil || e.Rule != "frames_differ" || e.Differing != 4 || e.RunStartMs != 8000 ||
				e.RunEndMs != 12000 || len(e.Frames) != 4 {
				t.Errorf("根拠 = %+v, want 8〜12 秒の連なりと 4 コマ", e)
			}
			wantPairs := []domain.FramePair{{A: 8, B: 8, Distance: 33}, {A: 9, B: 9, Distance: 33},
				{A: 10, B: 10, Distance: 33}, {A: 11, B: 11, Distance: 33}}
			if frames.keys != [2]string{"key-a", "key-b"} || !reflect.DeepEqual(frames.pairs, wantPairs) {
				t.Errorf("頼んだコマ = %v %v, want 連なりの 8〜11", frames.keys, frames.pairs)
			}
			if len(classifier.requests) != 1 {
				t.Fatalf("分類器への問い合わせ = %d, want 1", len(classifier.requests))
			}
			req := classifier.requests[0]
			if len(req.Input.Images) != 4 || len(req.Input.Facts) != 0 {
				t.Errorf("画像 %d 枚、手がかり %v, want 4 枚で手がかりなし", len(req.Input.Images), req.Input.Facts)
			}
			if len(req.Questions) != 1 || req.Questions[0].Text != domain.VersionJudgeQuestion {
				t.Errorf("問い = %+v", req.Questions)
			}
		})
	}
}

// A classifier that fails or does not answer is recorded as a failed judgement, whose job the store retries after
// the kind's delay.
func TestVersionJudgeClassifierFailures(t *testing.T) {
	for name, classifier := range map[string]*fakeClassifier{
		"誤り":   {err: errors.New("cannot reach Ollama")},
		"答えなし": {answers: map[string]float64{"other": 0.9}},
	} {
		t.Run(name, func(t *testing.T) {
			store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
			judge, _, _ := newTestVersionJudge(store, classifier)
			if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); err != nil {
				t.Fatal(err)
			}
			if got := store.lastResult(t); got.Verdict != domain.VersionFailed || got.Error == "" || got.Probability != nil {
				t.Errorf("結果 = %+v, want 理由のある failed", got)
			}
		})
	}
}

func TestVersionJudgeWaitsForASavedConnection(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
	classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: 0.9}}
	judge, settings, _ := newTestVersionJudge(store, classifier)
	woken := 0
	judge.Waker = func() { woken++ }
	settings.connected = false
	ctx := context.Background()

	if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); err != nil {
		t.Fatal(err)
	}
	if got := store.lastResult(t); got.Verdict != domain.VersionNoConnection {
		t.Fatalf("結果 = %+v, want no_connection", got)
	}
	if len(classifier.requests) != 0 {
		t.Fatalf("接続の無いときに分類器を呼んだ")
	}

	settings.connected = true
	judge.ConnectionSaved(ctx)
	if store.reset != 1 || woken != 1 {
		t.Fatalf("接続の保存で待ちに戻した回数 %d、起こした回数 %d, want 1, 1", store.reset, woken)
	}
}

// A reset that fails after the connection is saved stays owed: the scheduler is woken after domain.WorkerRetry and its
// next round retries the reset once.
func TestVersionJudgeRetriesAFailedResetAfterTheConnectionIsSaved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newFakeVersionJudgeStore()
		store.resetErr = errors.New("database is locked")
		judge, _, _ := newTestVersionJudge(store, &fakeClassifier{})
		var woken atomic.Int32
		judge.Waker = func() { woken.Add(1) }
		paused := PausedJobKinds(judge)
		ctx := context.Background()

		judge.ConnectionSaved(ctx)
		if store.reset != 0 || woken.Load() != 1 {
			t.Fatalf("失敗した保存: 待ちに戻した回数 %d、起こした回数 %d, want 0, 1", store.reset, woken.Load())
		}
		time.Sleep(time.Duration(domain.WorkerRetry))
		synctest.Wait()
		if woken.Load() != 2 {
			t.Fatalf("待ちの後に起こした回数 = %d, want 2", woken.Load())
		}
		for range 2 {
			if _, err := paused(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if store.reset != 1 {
			t.Errorf("スケジューラーの巡回で待ちに戻した回数 = %d, want 1", store.reset)
		}
	})
}

// A no_connection result decided before the connection was saved snoozes instead of being recorded, so the pair is
// judged again with the saved connection.
func TestVersionJudgeConnectionSavedDuringAJudgement(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
	classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: 0.9}}
	judge, settings, _ := newTestVersionJudge(store, classifier)
	settings.connected, settings.savedAfterRead = false, true

	if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); !errors.Is(err, domain.ErrJobSnoozed) {
		t.Fatalf("HandleJob = %v, want domain.ErrJobSnoozed", err)
	}
	if n := store.resultCount(); n != 0 {
		t.Fatalf("保存のあとに no_connection を記録した: %+v", store.lastResult(t))
	}
}

func TestVersionJudgeRecoverResumesPairsWaitingForAConnection(t *testing.T) {
	store := newFakeVersionJudgeStore()
	judge, settings, _ := newTestVersionJudge(store, &fakeClassifier{})
	ctx := context.Background()

	settings.connected = false
	if err := judge.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if store.recovered != 1 || store.reset != 0 {
		t.Fatalf("接続の無い起動: 回復 %d、待ちに戻す %d, want 1, 0", store.recovered, store.reset)
	}
	settings.connected = true
	if err := judge.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if store.reset != 1 {
		t.Errorf("接続のある起動で待ちに戻した回数 = %d, want 1", store.reset)
	}
}

// An input or a result that cannot be read or written goes back to the scheduler, which records the failure.
func TestVersionJudgeReturnsStoreErrors(t *testing.T) {
	cause := errors.New("database is locked")
	for name, set := range map[string]func(*fakeVersionJudgeStore){
		"読めない": func(s *fakeVersionJudgeStore) { s.inputErr = cause },
		"書けない": func(s *fakeVersionJudgeStore) { s.applyErr = cause },
	} {
		t.Run(name, func(t *testing.T) {
			store := newFakeVersionJudgeStore(versionJudgeJob())
			set(store)
			judge, _, _ := newTestVersionJudge(store, &fakeClassifier{})
			if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); !errors.Is(err, cause) {
				t.Fatalf("HandleJob = %v, want %v", err, cause)
			}
		})
	}
}

// Disable judging immediately before returning an answer to simulate a concurrent owner change.
type switchingClassifier struct {
	fakeClassifier
	judge *VersionJudge
}

func (c *switchingClassifier) Classify(ctx context.Context, req domain.ClassifierRequest) (map[string]float64, error) {
	if err := c.judge.SaveEnabled(ctx, false); err != nil {
		return nil, err
	}
	return c.fakeClassifier.Classify(ctx, req)
}

// A judgement finished after judging was turned off records nothing and snoozes, so the pair waits for judging to be
// turned on without spending an attempt.
func TestVersionJudgeDiscardsAJudgementTurnedOffMidway(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
	answers := map[string]float64{domain.VersionJudgeQuestionKey: 0.9}
	classifier := &switchingClassifier{fakeClassifier: fakeClassifier{answers: answers}}
	judge, _, _ := newTestVersionJudge(store, &classifier.fakeClassifier)
	judge.classifier = classifier
	classifier.judge = judge

	if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); !errors.Is(err, domain.ErrJobSnoozed) {
		t.Fatalf("HandleJob = %v, want domain.ErrJobSnoozed", err)
	}
	if n := store.resultCount(); n != 0 {
		t.Fatalf("切ったあとに結果を反映した: %+v", store.lastResult(t))
	}
}

// An input that cannot be read after judging was turned off snoozes like any other result, so the pair does not spend
// an attempt and wait out a retry delay.
func TestVersionJudgeSnoozesAnInputErrorWhileOff(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob())
	store.inputErr = errors.New("database is locked")
	judge, settings, _ := newTestVersionJudge(store, &fakeClassifier{})
	settings.enabled = false

	if err := handleVersionJudge(t, judge, versionJudgeJob().Keys); !errors.Is(err, domain.ErrJobSnoozed) {
		t.Fatalf("HandleJob = %v, want domain.ErrJobSnoozed", err)
	}
}

// While judging is off its jobs are paused, and turning it on wakes the scheduler to claim them.
func TestVersionJudgePausesItsJobsWhileOff(t *testing.T) {
	judge, settings, _ := newTestVersionJudge(newFakeVersionJudgeStore(), &fakeClassifier{})
	settings.enabled = false
	woken := 0
	judge.Waker = func() { woken++ }
	paused := PausedJobKinds(judge)
	ctx := context.Background()

	if got, err := paused(ctx); err != nil || !reflect.DeepEqual(got, map[domain.JobKind]bool{domain.JobVersionJudge: true}) {
		t.Fatalf("切の間に止めた種類 = %v, %v", got, err)
	}
	if err := judge.SaveEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if woken != 1 {
		t.Fatalf("入れたときに起こした回数 = %d, want 1", woken)
	}
	if got, err := paused(ctx); err != nil || len(got) != 0 {
		t.Fatalf("入れた後に止めた種類 = %v, %v", got, err)
	}
}

// Titled chapters at the same starts settle a pair whose frames differ, so the classifier is not asked.
func TestVersionJudgeLetsMatchingChaptersSkipTheClassifier(t *testing.T) {
	job := versionJudgeJob(8, 9, 10, 11)
	titled := []domain.MediaChapter{{StartMs: 0, Title: "Opening"}, {StartMs: 5000, Title: "Middle"}, {StartMs: 15000, Title: "End"}}
	job.Properties = [2]domain.VersionProperties{{Chapters: titled}, {Chapters: titled}}
	store := newFakeVersionJudgeStore(job)
	classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: 0.1}}
	judge, _, _ := newTestVersionJudge(store, classifier)

	if err := handleVersionJudge(t, judge, job.Keys); err != nil {
		t.Fatal(err)
	}

	got := store.lastResult(t)
	if got.Verdict != domain.VersionSame || got.Decider != domain.ActivityByRule || got.Evidence == nil ||
		!got.Evidence.ByProperties || len(got.Evidence.Properties) != 1 {
		t.Errorf("結果 = %+v（根拠 %+v）, want チャプターで決めた same", got, got.Evidence)
	}
	if len(classifier.requests) != 0 {
		t.Errorf("分類器に %d 回聞いた, want 0", len(classifier.requests))
	}
}
