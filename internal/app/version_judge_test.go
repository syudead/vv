package app

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Empty state means pending in the in-memory fake.
type fakeVersionJudgeStore struct {
	mu      sync.Mutex
	jobs    []domain.VersionJudgeJob
	states  map[[2]string]domain.VersionVerdict
	results []domain.VersionJudgeResult
	claims  int
	retryAt time.Time

	applyErr error
}

func newFakeVersionJudgeStore(jobs ...domain.VersionJudgeJob) *fakeVersionJudgeStore {
	return &fakeVersionJudgeStore{jobs: jobs, states: map[[2]string]domain.VersionVerdict{}}
}

func (s *fakeVersionJudgeStore) ClaimJudgement(context.Context) (domain.VersionJudgeJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims++
	for _, job := range s.jobs {
		if _, judged := s.states[job.Keys]; !judged {
			s.states[job.Keys] = 0
			return job, nil
		}
	}
	return domain.VersionJudgeJob{}, domain.ErrNoJob
}

func (s *fakeVersionJudgeStore) ApplyJudgement(_ context.Context, keys [2]string, result domain.VersionJudgeResult) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applyErr != nil {
		return false, s.applyErr
	}
	s.states[keys] = result.Verdict
	s.results = append(s.results, result)
	return true, nil
}

func (s *fakeVersionJudgeStore) RecoverRunningJudgements(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recovered := 0
	for keys, verdict := range s.states {
		if verdict == 0 {
			delete(s.states, keys)
			recovered++
		}
	}
	return recovered, nil
}

func (s *fakeVersionJudgeStore) NextJudgementRetry(context.Context) (time.Time, bool, error) {
	return s.retryAt, !s.retryAt.IsZero(), nil
}

func (s *fakeVersionJudgeStore) ResetNoConnectionJudgements(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reset := 0
	for keys, verdict := range s.states {
		if verdict == domain.VersionNoConnection {
			delete(s.states, keys)
			reset++
		}
	}
	return reset, nil
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

type fakeVersionJudgeSettings struct {
	mu        sync.Mutex
	enabled   bool
	connected bool
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
	return s.connected, nil
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

func TestVersionJudgeMatchingPairBundlesWithoutClassifier(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob())
	classifier := &fakeClassifier{}
	judge, _, _ := newTestVersionJudge(store, classifier)

	if wait := judge.step(context.Background()); wait != 0 {
		t.Fatalf("判定のあとの待ち = %v, want 0", wait)
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

			judge.step(context.Background())

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

func TestVersionJudgeClassifierFailures(t *testing.T) {
	for name, classifier := range map[string]*fakeClassifier{
		"誤り":   {err: errors.New("cannot reach Ollama")},
		"答えなし": {answers: map[string]float64{"other": 0.9}},
	} {
		t.Run(name, func(t *testing.T) {
			store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
			judge, _, _ := newTestVersionJudge(store, classifier)
			judge.step(context.Background())
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
	settings.connected = false
	ctx := context.Background()

	judge.step(ctx)
	if got := store.lastResult(t); got.Verdict != domain.VersionNoConnection {
		t.Fatalf("結果 = %+v, want no_connection", got)
	}
	if len(classifier.requests) != 0 {
		t.Fatalf("接続の無いときに分類器を呼んだ")
	}
	if wait := judge.step(ctx); wait >= 0 {
		t.Fatalf("no_connection の組を取り出した（待ち %v）", wait)
	}

	settings.connected = true
	judge.ConnectionSaved(ctx)
	select {
	case <-judge.wake:
	default:
		t.Fatal("接続の保存で起こさない")
	}
	judge.step(ctx)
	if got := store.lastResult(t); got.Verdict != domain.VersionSame || len(classifier.requests) != 1 {
		t.Errorf("接続の保存のあと = %+v（問い合わせ %d）, want same（1）", got, len(classifier.requests))
	}
}

func TestVersionJudgeConnectionSavedDuringAJudgement(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
	classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: 0.9}}
	judge, settings, _ := newTestVersionJudge(store, classifier)
	settings.connected = false
	ctx := context.Background()

	// Save connection settings between reading no_connection and applying that result.
	job, err := store.ClaimJudgement(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.connected = true
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	judge.ConnectionSaved(cancelled)
	if _, err := store.ApplyJudgement(ctx, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionNoConnection}); err != nil {
		t.Fatal(err)
	}

	judge.step(ctx)
	if got := store.lastResult(t); got.Verdict != domain.VersionSame || len(classifier.requests) != 1 {
		t.Errorf("接続の保存のあと = %+v（問い合わせ %d）, want same（1）", got, len(classifier.requests))
	}
}

func TestVersionJudgeRecoverResumesPairsWaitingForAConnection(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
	store.states[versionJudgeJob().Keys] = domain.VersionNoConnection
	classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: 0.9}}
	judge, _, _ := newTestVersionJudge(store, classifier)
	ctx := context.Background()

	if err := judge.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	judge.step(ctx)
	if got := store.lastResult(t); got.Verdict != domain.VersionSame {
		t.Errorf("起動のあと = %+v, want same", got)
	}
}

func TestVersionJudgeRequeuesAJudgementItCannotRecord(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob())
	store.applyErr = errors.New("database is locked")
	judge, _, _ := newTestVersionJudge(store, &fakeClassifier{})
	ctx := context.Background()

	if wait := judge.step(ctx); wait != versionJudgeRetryDelay {
		t.Fatalf("保存できなかったあとの待ち = %v, want %v", wait, versionJudgeRetryDelay)
	}
	store.mu.Lock()
	store.applyErr = nil
	store.mu.Unlock()
	if wait := judge.step(ctx); wait != 0 {
		t.Fatalf("判定し直しの待ち = %v, want 0", wait)
	}
	if got := store.lastResult(t); got.Verdict != domain.VersionSame {
		t.Errorf("判定し直した結果 = %+v, want same", got)
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

func TestVersionJudgeDiscardsAJudgementTurnedOffMidway(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
	answers := map[string]float64{domain.VersionJudgeQuestionKey: 0.9}
	classifier := &switchingClassifier{fakeClassifier: fakeClassifier{answers: answers}}
	judge, settings, _ := newTestVersionJudge(store, &classifier.fakeClassifier)
	judge.classifier = classifier
	classifier.judge = judge
	ctx := context.Background()

	if wait := judge.step(ctx); wait != 0 {
		t.Fatalf("判定のあとの待ち = %v, want 0", wait)
	}
	store.mu.Lock()
	results := len(store.results)
	verdict, claimed := store.states[versionJudgeJob().Keys]
	store.mu.Unlock()
	if results != 0 {
		t.Fatalf("切ったあとに結果を反映した: %+v", store.lastResult(t))
	}
	if claimed {
		t.Fatalf("組が待ちに戻っていない（判定 %v）", verdict)
	}
	if wait := judge.step(ctx); wait >= 0 {
		t.Fatalf("切のあいだに取り出した（待ち %v）", wait)
	}

	if err := settings.SaveVersionJudgingEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	judge.classifier = &classifier.fakeClassifier
	judge.step(ctx)
	if got := store.lastResult(t); got.Verdict != domain.VersionSame {
		t.Errorf("入れ直したあと = %+v, want same", got)
	}
}

func TestVersionJudgeIdlesWhileOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newFakeVersionJudgeStore()
		judge, settings, _ := newTestVersionJudge(store, &fakeClassifier{})
		settings.enabled = false
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			judge.Run(ctx)
			close(done)
		}()
		defer func() {
			cancel()
			// Release the wait even if its cancellation branch regresses.
			judge.Wake()
			<-done
		}()
		claims := func() int {
			store.mu.Lock()
			defer store.mu.Unlock()
			return store.claims
		}

		synctest.Wait()
		if got := claims(); got != 0 {
			t.Fatalf("claims while disabled = %d, want 0", got)
		}
		if err := judge.SaveEnabled(ctx, true); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if got := claims(); got != 1 {
			t.Fatalf("claims after enabling = %d, want 1", got)
		}

		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("worker did not stop when cancelled while idle")
		}
	})
}

func TestVersionJudgeWaitsUntilTheNextRetry(t *testing.T) {
	store := newFakeVersionJudgeStore()
	now := time.Unix(1_700_000_000, 0)
	judge := NewVersionJudge(VersionJudgeOptions{
		Store: store, Settings: &fakeVersionJudgeSettings{enabled: true}, Classifier: &fakeClassifier{},
		Frames: &fakeVersionFrames{}, Now: func() time.Time { return now },
	})
	if wait := judge.step(context.Background()); wait >= 0 {
		t.Errorf("失敗の組が無いときの待ち = %v, want 起こされるまで", wait)
	}
	store.retryAt = now.Add(10 * time.Minute)
	if wait := judge.step(context.Background()); wait != 10*time.Minute+time.Second {
		t.Errorf("待ち = %v, want 10m1s", wait)
	}
}

func TestVersionJudgeLetsAMatchingWorkCodeDecideAnUnsureAnswer(t *testing.T) {
	job := versionJudgeJob(8, 9, 10, 11)
	job.Facts = [2]domain.VersionFacts{{WorkCode: "abc-1"}, {WorkCode: "abc-1"}}
	store := newFakeVersionJudgeStore(job)
	classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: 0.5}}
	judge, _, _ := newTestVersionJudge(store, classifier)

	judge.step(context.Background())

	got := store.lastResult(t)
	if got.Verdict != domain.VersionSame || got.Decider != domain.ActivityByRule || got.Probability == nil ||
		got.Evidence == nil || !got.Evidence.ByFacts || len(got.Evidence.Facts) != 1 {
		t.Errorf("結果 = %+v（根拠 %+v）, want 作品コードで決めた same、AI の答えは残す", got, got.Evidence)
	}
}
