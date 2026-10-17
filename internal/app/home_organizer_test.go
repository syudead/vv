package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 規則でホームを整理する（docs/design-docs/home.md）。案の適用そのもの（owner-kept
// の棚を残す、extend が名前を変えない）は保存層と cmd/mdm の結合の試験が確かめる。

// fakeHomeStore は整理役の保存先の代わりで、待ちと最後の実行をメモリに持ち、適用した案を記録する。
type fakeHomeStore struct {
	mu sync.Mutex
	// pending と last は settings の home.organize.pending と home.organize.last の代わりである。
	pending domain.HomeOrganizeRun
	last    *domain.HomeOrganizeLast
	// all と unshelved は ShelfCandidates が返す元である。
	all, unshelved domain.ShelfCandidates
	applied        []appliedProposal
	// started は ApplyProposal が始まるたびに送る。block があれば、ApplyProposal はそれを受けるまで戻らない。
	started chan domain.HomeOrganizeRun
	block   chan struct{}
	// failPendingReads、failLastReads、failSavePending、failSaveLast は、それぞれの呼び出しを
	// その回数だけ errHomeStore で失敗させる。
	failPendingReads, failLastReads, failSavePending, failSaveLast atomic.Int32
}

var errHomeStore = errors.New("the home store is unavailable")

// fail は counter が正なら 1 減らして errHomeStore を返す。
func fail(counter *atomic.Int32) error {
	if counter.Add(-1) >= 0 {
		return errHomeStore
	}
	counter.Store(0)
	return nil
}

type appliedProposal struct {
	run   domain.HomeOrganizeRun
	names []string
}

func (s *fakeHomeStore) PendingOrganize(context.Context) (domain.HomeOrganizeRun, bool, error) {
	if err := fail(&s.failPendingReads); err != nil {
		return "", false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending, s.pending != "", nil
}

func (s *fakeHomeStore) SavePendingOrganize(_ context.Context, run domain.HomeOrganizeRun) error {
	if err := fail(&s.failSavePending); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = run
	return nil
}

func (s *fakeHomeStore) ClearPendingOrganize(ctx context.Context) error {
	return s.SavePendingOrganize(ctx, "")
}

func (s *fakeHomeStore) LastOrganize(context.Context) (domain.HomeOrganizeLast, bool, error) {
	if err := fail(&s.failLastReads); err != nil {
		return domain.HomeOrganizeLast{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return domain.HomeOrganizeLast{}, false, nil
	}
	return *s.last, true, nil
}

func (s *fakeHomeStore) SaveLastOrganize(_ context.Context, last domain.HomeOrganizeLast) error {
	if err := fail(&s.failSaveLast); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = &last
	return nil
}

func (s *fakeHomeStore) ShelfCandidates(_ context.Context, unshelvedOnly bool) (domain.ShelfCandidates, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if unshelvedOnly {
		return s.unshelved, nil
	}
	return s.all, nil
}

func (s *fakeHomeStore) ApplyProposal(ctx context.Context, run domain.HomeOrganizeRun, proposal []domain.ProposedShelf) (domain.HomeApplied, error) {
	if s.started != nil {
		s.started <- run
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return domain.HomeApplied{}, ctx.Err()
		}
	}
	names := make([]string, len(proposal))
	for i, shelf := range proposal {
		names[i] = shelf.Name
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applied = append(s.applied, appliedProposal{run: run, names: names})
	return domain.HomeApplied{Added: len(proposal), Changed: len(proposal) > 0}, nil
}

func (s *fakeHomeStore) appliedRuns() []appliedProposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.applied)
}

func (s *fakeHomeStore) storedPending() domain.HomeOrganizeRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending
}

// fakeQueues は job の待ち行列の代わりで、busy のあいだは空でなく、broken のあいだは読めない。
type fakeQueues struct{ busy, broken atomic.Bool }

func (q *fakeQueues) HasUnfinishedJobs(context.Context) (bool, error) {
	if q.broken.Load() {
		return false, errHomeStore
	}
	return q.busy.Load(), nil
}

// fakeAutoTagBacklog は自動タグ付けの待ち行列の件数だけを返す。
type fakeAutoTagBacklog struct{ queued atomic.Int32 }

func (q *fakeAutoTagBacklog) Counts(context.Context) (domain.AutoTagCounts, error) {
	return domain.AutoTagCounts{Queued: int(q.queued.Load())}, nil
}

type homeEvents struct{ count atomic.Int32 }

func (p *homeEvents) Publish(events ...domain.Event) {
	for _, event := range events {
		if _, ok := event.(domain.HomeChanged); ok {
			p.count.Add(1)
		}
	}
}

// startHomeOrganizer は store の整理役を Recover して Run で動かし、試験の終わりに止める。
func startHomeOrganizer(t *testing.T, store *fakeHomeStore, opts HomeOrganizerOptions) *HomeOrganizer {
	t.Helper()
	opts.Store = store
	if opts.Settle == 0 {
		opts.Settle = time.Millisecond
	}
	opts.Poll = time.Millisecond
	organizer := NewHomeOrganizer(opts)
	if err := organizer.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		organizer.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return organizer
}

// waitApplied は store に n 回の適用が記録されるまで待ち、その記録を返す。
func waitApplied(t *testing.T, store *fakeHomeStore, n int) []appliedProposal {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if applied := store.appliedRuns(); len(applied) >= n {
			return applied
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("applied = %+v, want %d runs", store.appliedRuns(), n)
	return nil
}

// waitIdle は整理役が走っておらず、待ちも無くなるまで待つ。
func waitIdle(t *testing.T, organizer *HomeOrganizer) domain.HomeOrganizerState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := organizer.State(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !state.Running && state.Pending == "" && state.Last != nil {
			return state
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the organizer did not become idle")
	return domain.HomeOrganizerState{}
}

func tagCandidate(id int64, name string, videos int) domain.ShelfCandidate {
	return domain.ShelfCandidate{Kind: domain.ShelfSourceTag, ID: id, Name: name, Videos: videos}
}

// Reorganize は rebuild で、見せる動画 5 本以上のタグ・人物・団体ごとに棚を 1 つ作り、4 本のものには
// 作らない。終わりを最後の実行に書き、状態が変わるたびに HomeChanged を出す。
func TestHomeOrganizerRebuildMakesShelvesFromRules(t *testing.T) {
	store := &fakeHomeStore{all: domain.ShelfCandidates{Videos: 30, Candidates: []domain.ShelfCandidate{
		tagCandidate(1, "Cats", 5),
		tagCandidate(2, "Few", 4),
		{Kind: domain.ShelfSourcePerson, ID: 3, Name: "Hanako", Videos: 6},
		{Kind: domain.ShelfSourceOrganization, ID: 4, Name: "Maker", Videos: 7},
	}}}
	events := &homeEvents{}
	organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{Publisher: events})

	state, err := organizer.Reorganize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != domain.HomeRebuild && !state.Running && state.Last == nil {
		t.Errorf("state after Reorganize = %+v, want a queued or running rebuild", state)
	}
	applied := waitApplied(t, store, 1)
	if applied[0].run != domain.HomeRebuild || !slices.Equal(applied[0].names, []string{"Cats", "Hanako", "Maker"}) {
		t.Errorf("applied = %+v, want a rebuild of Cats, Hanako, Maker", applied[0])
	}
	state = waitIdle(t, organizer)
	if state.Last.Run != domain.HomeRebuild || state.Last.Outcome != domain.HomeOrganizeDone || state.Last.Added != 3 {
		t.Errorf("last = %+v", state.Last)
	}
	if store.storedPending() != "" {
		t.Errorf("pending setting = %q, want cleared", store.storedPending())
	}
	// 待ちに入れた、走り始めた、終わったの 3 回。
	if got := events.count.Load(); got < 3 {
		t.Errorf("HomeChanged = %d, want at least 3", got)
	}
}

// 棚にない動画が 20 本でスキャンが終わると、待ち行列が空になってから extend が走り、棚にない動画の元だけで
// 案を作る。19 本では何も走らない。
func TestHomeOrganizerExtendsAfterScanWhenTwentyUnshelved(t *testing.T) {
	for _, c := range []struct {
		unshelved int
		want      bool
	}{{20, true}, {19, false}} {
		last := domain.HomeOrganizeLast{Run: domain.HomeRebuild, Outcome: domain.HomeOrganizeDone, FinishedAt: 1}
		store := &fakeHomeStore{
			last: &last,
			all:  domain.ShelfCandidates{Videos: 100, Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 50)}},
			unshelved: domain.ShelfCandidates{Videos: c.unshelved, Candidates: []domain.ShelfCandidate{
				tagCandidate(2, "Dogs", c.unshelved),
			}},
		}
		jobs := &fakeQueues{}
		jobs.busy.Store(true)
		autoTags := &fakeAutoTagBacklog{}
		autoTags.queued.Store(1)
		organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{Jobs: jobs, AutoTags: autoTags})

		organizer.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
		time.Sleep(30 * time.Millisecond)
		if applied := store.appliedRuns(); len(applied) != 0 {
			t.Fatalf("%d unshelved: applied %+v while the job queue is busy", c.unshelved, applied)
		}
		jobs.busy.Store(false)
		time.Sleep(30 * time.Millisecond)
		if applied := store.appliedRuns(); len(applied) != 0 {
			t.Fatalf("%d unshelved: applied %+v while the auto-tag queue is busy", c.unshelved, applied)
		}
		autoTags.queued.Store(0)
		if !c.want {
			time.Sleep(50 * time.Millisecond)
			if applied := store.appliedRuns(); len(applied) != 0 {
				t.Errorf("19 unshelved: applied %+v, want nothing", applied)
			}
			continue
		}
		applied := waitApplied(t, store, 1)
		if applied[0].run != domain.HomeExtend || !slices.Equal(applied[0].names, []string{"Dogs"}) {
			t.Errorf("applied = %+v, want an extend of Dogs", applied[0])
		}
	}
}

// 待ち行列が空のまま Settle 続かないうちは走らない。
func TestHomeOrganizerWaitsForQueuesToStayEmpty(t *testing.T) {
	store := &fakeHomeStore{all: domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}}}
	organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{Settle: time.Hour})
	organizer.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
	time.Sleep(30 * time.Millisecond)
	if applied := store.appliedRuns(); len(applied) != 0 {
		t.Errorf("applied = %+v before the queues stayed empty", applied)
	}
}

// 1 度も終わっていなければ、スキャンのあと待ち行列が空なら rebuild。失敗で終わったスキャンでは走らない。
func TestHomeOrganizerFirstRunAfterScanIsRebuild(t *testing.T) {
	store := &fakeHomeStore{all: domain.ShelfCandidates{Videos: 5, Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}}}
	organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{})

	organizer.ScanFinished(domain.ScanOriginManual, domain.ScanFailed)
	time.Sleep(30 * time.Millisecond)
	if applied := store.appliedRuns(); len(applied) != 0 {
		t.Fatalf("applied = %+v after a failed scan", applied)
	}
	organizer.ScanFinished(domain.ScanOriginWatch, domain.ScanDone)
	applied := waitApplied(t, store, 1)
	if applied[0].run != domain.HomeRebuild || !slices.Equal(applied[0].names, []string{"Cats"}) {
		t.Errorf("applied = %+v, want a rebuild of Cats", applied[0])
	}
}

// 待ちの実行は settings に残り、再起動のあとの Recover と Run で走る。
func TestHomeOrganizerPendingRunSurvivesRestart(t *testing.T) {
	store := &fakeHomeStore{all: domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}}}
	stopped := NewHomeOrganizer(HomeOrganizerOptions{Store: store})
	if _, err := stopped.Reorganize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := store.storedPending(); got != domain.HomeRebuild {
		t.Fatalf("pending setting = %q, want rebuild", got)
	}

	restarted := startHomeOrganizer(t, store, HomeOrganizerOptions{})
	applied := waitApplied(t, store, 1)
	if applied[0].run != domain.HomeRebuild {
		t.Errorf("applied = %+v, want the rebuild", applied[0])
	}
	waitIdle(t, restarted)
	if got := store.storedPending(); got != "" {
		t.Errorf("pending setting = %q, want cleared", got)
	}
}

// 走っている途中で止めた実行は記録せず、settings の待ちを残す。
func TestHomeOrganizerInterruptedRunStaysPending(t *testing.T) {
	store := &fakeHomeStore{
		all:     domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}},
		started: make(chan domain.HomeOrganizeRun, 1),
		block:   make(chan struct{}),
	}
	organizer := NewHomeOrganizer(HomeOrganizerOptions{Store: store, Poll: time.Millisecond})
	if _, err := organizer.Reorganize(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		organizer.Run(ctx)
	}()
	<-store.started
	cancel()
	<-done
	if got := store.storedPending(); got != domain.HomeRebuild {
		t.Errorf("pending setting = %q, want rebuild kept for the next start", got)
	}
	if _, found, _ := store.LastOrganize(context.Background()); found {
		t.Error("an interrupted run was recorded")
	}
}

// 走っているあいだに頼んだ rebuild は、終わったあとに走る。
func TestHomeOrganizerRebuildRequestedDuringRunRunsAfter(t *testing.T) {
	store := &fakeHomeStore{
		all:     domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}},
		started: make(chan domain.HomeOrganizeRun, 2),
		block:   make(chan struct{}),
	}
	organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{})
	if _, err := organizer.Reorganize(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-store.started
	state, err := organizer.Reorganize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Running || state.Pending != domain.HomeRebuild {
		t.Errorf("state during the run = %+v, want running with a pending rebuild", state)
	}
	if got := store.storedPending(); got != domain.HomeRebuild {
		t.Errorf("pending setting during the run = %q, want rebuild", got)
	}
	store.block <- struct{}{}
	<-store.started
	store.block <- struct{}{}
	applied := waitApplied(t, store, 2)
	if applied[0].run != domain.HomeRebuild || applied[1].run != domain.HomeRebuild {
		t.Errorf("applied = %+v, want two rebuilds", applied)
	}
	waitIdle(t, organizer)
	if got := len(store.appliedRuns()); got != 2 {
		t.Errorf("applied %d runs, want 2", got)
	}
}

// 起動時の Recover が読めなくても、Run が読み直して待ちの実行を走らせる。
func TestHomeOrganizerRetriesRecoverInRun(t *testing.T) {
	store := &fakeHomeStore{
		pending: domain.HomeRebuild,
		all:     domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}},
	}
	store.failPendingReads.Store(2)
	organizer := NewHomeOrganizer(HomeOrganizerOptions{Store: store, Poll: time.Millisecond})
	if err := organizer.Recover(context.Background()); !errors.Is(err, errHomeStore) {
		t.Fatalf("Recover = %v, want the store error", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		organizer.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	applied := waitApplied(t, store, 1)
	if applied[0].run != domain.HomeRebuild {
		t.Errorf("applied = %+v, want the recovered rebuild", applied[0])
	}
	waitIdle(t, organizer)
}

// 終わった実行を記録できなければ、待ちの setting を残して走らせ直し、記録できてから待ちを消す。
func TestHomeOrganizerRerunsWhenTheLastRunCannotBeRecorded(t *testing.T) {
	store := &fakeHomeStore{all: domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}}}
	store.failSaveLast.Store(1)
	organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{})
	if _, err := organizer.Reorganize(context.Background()); err != nil {
		t.Fatal(err)
	}
	applied := waitApplied(t, store, 2)
	if applied[0].run != domain.HomeRebuild || applied[1].run != domain.HomeRebuild {
		t.Errorf("applied = %+v, want the rebuild run twice", applied)
	}
	state := waitIdle(t, organizer)
	if state.Last.Run != domain.HomeRebuild {
		t.Errorf("last = %+v, want the rebuild recorded", state.Last)
	}
	if got := store.storedPending(); got != "" {
		t.Errorf("pending setting = %q, want cleared", got)
	}
}

// 待ち行列が空になったあとの読み書きが 1 度失敗しても、その走査のあとの実行は次の Poll で待ちに入る。
func TestHomeOrganizerRetriesTheRunAfterScanOnStoreErrors(t *testing.T) {
	for name, arm := range map[string]func(*fakeHomeStore){
		"read the last run":    func(s *fakeHomeStore) { s.failLastReads.Store(1) },
		"save the pending run": func(s *fakeHomeStore) { s.failSavePending.Store(1) },
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeHomeStore{all: domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}}}
			arm(store)
			organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{})
			organizer.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
			applied := waitApplied(t, store, 1)
			if applied[0].run != domain.HomeRebuild {
				t.Errorf("applied = %+v, want the first rebuild", applied[0])
			}
		})
	}
}

// 待ち行列を読めなかった間は空と数えず、読めてから Settle だけ空が続くまで走らない。
func TestHomeOrganizerDoesNotCountUnreadQueuesAsEmpty(t *testing.T) {
	store := &fakeHomeStore{all: domain.ShelfCandidates{Candidates: []domain.ShelfCandidate{tagCandidate(1, "Cats", 5)}}}
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	jobs := &fakeQueues{}
	organizer := startHomeOrganizer(t, store, HomeOrganizerOptions{Jobs: jobs, Settle: time.Minute, Now: clock.Now})
	organizer.ScanFinished(domain.ScanOriginManual, domain.ScanDone)
	time.Sleep(20 * time.Millisecond)
	jobs.broken.Store(true)
	time.Sleep(20 * time.Millisecond)
	clock.Advance(2 * time.Minute)
	time.Sleep(20 * time.Millisecond)
	jobs.broken.Store(false)
	time.Sleep(30 * time.Millisecond)
	if applied := store.appliedRuns(); len(applied) != 0 {
		t.Fatalf("applied = %+v, counting the unread time as empty", applied)
	}
	clock.Advance(time.Minute)
	if applied := waitApplied(t, store, 1); applied[0].run != domain.HomeRebuild {
		t.Errorf("applied = %+v, want the first rebuild", applied[0])
	}
}
