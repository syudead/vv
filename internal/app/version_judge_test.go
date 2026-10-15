package app

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 重複の自動判定のワーカー（specs/054-auto-dup/research.md R-1〜R-3・R-5・R-11・R-13）。

// fakeVersionJudgeStore は候補の組を記憶だけで真似る。states は組ごとの判定（空は待ち）、results は
// 反映した結果である。
type fakeVersionJudgeStore struct {
	mu      sync.Mutex
	jobs    []domain.VersionJudgeJob
	states  map[[2]string]domain.VersionVerdict
	results []domain.VersionJudgeResult
	claims  int
	retryAt time.Time
	// applyErr は ApplyJudgement が返す誤りである。
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

// RecoverRunningJudgements は判定中（0）の組を待ちに戻す。
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

// lastResult は最後に反映した結果である。
func (s *fakeVersionJudgeStore) lastResult(t *testing.T) domain.VersionJudgeResult {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		t.Fatal("結果を反映していない")
	}
	return s.results[len(s.results)-1]
}

// fakeVersionJudgeSettings は入切と、Ollama の接続先を保存したかを持つ。
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

// fakeVersionFrames は頼まれた組を覚え、組ごとに 1 枚の画像を返す。
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

// versionJudgeJob は 20 コマの 2 本の指紋の組を作る。differing のコマだけ距離 33 で食い違う。
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

// newTestVersionJudge は入で接続を保存したワーカーを作る。
func newTestVersionJudge(store *fakeVersionJudgeStore, classifier *fakeClassifier) (*VersionJudge, *fakeVersionJudgeSettings, *fakeVersionFrames) {
	settings := &fakeVersionJudgeSettings{enabled: true, connected: true}
	frames := &fakeVersionFrames{}
	judge := NewVersionJudge(VersionJudgeOptions{Store: store, Settings: settings, Classifier: classifier, Frames: frames})
	return judge, settings, frames
}

// 一致する組は分類器を呼ばずに束ねる（R-1）。
func TestVersionJudgeMatchingPairBundlesWithoutClassifier(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob())
	classifier := &fakeClassifier{}
	judge, _, _ := newTestVersionJudge(store, classifier)

	if wait := judge.step(context.Background()); wait != 0 {
		t.Fatalf("判定のあとの待ち = %v, want 0", wait)
	}
	if got := store.lastResult(t); got.Verdict != domain.VersionSame {
		t.Errorf("結果 = %+v, want same", got)
	}
	if len(classifier.requests) != 0 {
		t.Errorf("分類器を呼んだ: %d 回", len(classifier.requests))
	}
}

// 食い違う組は連なりのコマを左右に並べて送り、確率で束ねる・却下する・残す（R-2・R-3・R-11）。
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

// 誤りや問いのキーのない返事は確率 0 ではなく失敗にする（R-3・R-5）。
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

// 接続が保存されていなければ、モデルの要る組は分類器を呼ばずに no_connection になり、接続を
// 保存したあとに判定される（R-13）。
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

// 判定中に接続が保存されて、その組が後から no_connection を書いても、次の判定の前に待ちに戻る。
// 保存の要求の context が取り消されていても戻る（R-13）。
func TestVersionJudgeConnectionSavedDuringAJudgement(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob(8, 9, 10, 11))
	classifier := &fakeClassifier{answers: map[string]float64{domain.VersionJudgeQuestionKey: 0.9}}
	judge, settings, _ := newTestVersionJudge(store, classifier)
	settings.connected = false
	ctx := context.Background()

	// 判定が接続の無いことを読んだあと、結果を反映する前に接続が保存された。
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

// 起動時に接続が保存されていれば、前回 no_connection のまま残った組を待ちに戻す（R-13）。
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

// 結果を保存できなければ、組を判定中のまま残さず、次の判定の前に待ちに戻して判定し直す。
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

// 設定が切のあいだは何も取り出さず、入れると起こされて取り出す。
func TestVersionJudgeIdlesWhileOff(t *testing.T) {
	store := newFakeVersionJudgeStore(versionJudgeJob())
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
		<-done
	}()

	time.Sleep(50 * time.Millisecond)
	store.mu.Lock()
	claims := store.claims
	store.mu.Unlock()
	if claims != 0 {
		t.Fatalf("切のあいだに取り出した: %d 回", claims)
	}

	if err := judge.SaveEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		store.mu.Lock()
		judged := len(store.results)
		store.mu.Unlock()
		if judged == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("入れたあとに判定しない")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// 取り出せる組が無ければ、次の失敗した組の再試行の時刻まで待つ（R-5）。無ければ起こされるまで待つ。
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
