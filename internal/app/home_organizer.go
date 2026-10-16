package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// HomeOrganizerStore は整理役の待ちの実行・最後の実行・案の元と、案の適用の保存先である。
// internal/store の *HomeStore が満たす（specs/057-home/data-model.md「Settings」、research.md R-8）。
type HomeOrganizerStore interface {
	// PendingOrganize は待っている実行を返す。無ければ found は偽である。
	PendingOrganize(ctx context.Context) (run domain.HomeOrganizeRun, found bool, err error)
	SavePendingOrganize(ctx context.Context, run domain.HomeOrganizeRun) error
	ClearPendingOrganize(ctx context.Context) error
	// LastOrganize は最後に終わった実行を返す。無ければ found は偽である。
	LastOrganize(ctx context.Context) (last domain.HomeOrganizeLast, found bool, err error)
	SaveLastOrganize(ctx context.Context, last domain.HomeOrganizeLast) error
	// ShelfCandidates は所有者に見せる動画（unshelvedOnly なら棚にない動画だけ）から、棚になりうる
	// タグ・人物・団体を数える。
	ShelfCandidates(ctx context.Context, unshelvedOnly bool) (domain.ShelfCandidates, error)
	// ApplyProposal は案を 1 つの取引で適用する。
	ApplyProposal(ctx context.Context, run domain.HomeOrganizeRun, proposal []domain.ProposedShelf) (domain.HomeApplied, error)
}

// AutoTagCounter は自動タグ付けの待ち行列の件数を読む口である。internal/store の *AutoTagStore が満たす。
type AutoTagCounter interface {
	Counts(ctx context.Context) (domain.AutoTagCounts, error)
}

// 整理役の既定の時間（specs/057-home/research.md R-9）。
const (
	// homeQueueSettle は走査が終わったあと、job と自動タグ付けの待ち行列が空のまま続く長さで、これだけ
	// 続いたら extend か最初の rebuild を待ちに入れる。
	homeQueueSettle = 2 * time.Minute
	// homeQueuePoll は待ち行列が空かを見る間隔である。
	homeQueuePoll = 10 * time.Second
)

// HomeOrganizerOptions は整理役に必要な依存である。
type HomeOrganizerOptions struct {
	Store HomeOrganizerStore
	// Jobs と AutoTags は走査のあとに空になるのを待つ待ち行列で、nil なら空とみなす。
	Jobs     UnfinishedJobStore
	AutoTags AutoTagCounter
	// Publisher は状態が変わるたびに domain.HomeChanged を出す先で、nil なら出さない。
	Publisher Publisher
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
	// Settle と Poll は 0 なら homeQueueSettle と homeQueuePoll である。試験が短くする。
	Settle time.Duration
	Poll   time.Duration
	// Now は nil なら time.Now である。
	Now func() time.Time
}

// HomeOrganizer は規則の整理役でホームの棚を組み直し、足す（specs/057-home/research.md R-1、R-2、R-9、
// R-10）。待ちの実行は settings の home.organize.pending に 1 つだけ持ち、Run の goroutine で
// 1 つずつ走らせる。走っているあいだの依頼は、終わったあとに走る。
type HomeOrganizer struct {
	store     HomeOrganizerStore
	jobs      UnfinishedJobStore
	autoTags  AutoTagCounter
	publisher Publisher
	logger    *slog.Logger
	settle    time.Duration
	poll      time.Duration
	now       func() time.Time
	wake      chan struct{}

	mu sync.Mutex
	// pending は次に走らせる実行で、running は今走っている実行である。どちらも無ければ空である。
	// settings の値は、両方を重ねた（domain.MergeHomeOrganizeRun）実行で、走っている途中で止めても
	// 次の起動で続きを走らせる。
	pending, running domain.HomeOrganizeRun
	// afterScan は、done で終わった走査のあと、待ち行列が空になるのを待っているかである。emptySince は
	// 待ち行列が空と分かった最初の時刻で、空でないと分かるたびに零へ戻す。
	afterScan  bool
	emptySince time.Time
	// reload は settings の待ちの実行を読み直す必要があるかである。Recover が読めなかったときと、終わった
	// 実行を記録できなかったときに立ち、Run が Poll ごとに読み直す。
	reload bool
}

// NewHomeOrganizer は整理役を組み立てる。待ちの実行は Recover で読み、Run で走らせる。
func NewHomeOrganizer(opts HomeOrganizerOptions) *HomeOrganizer {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	o := &HomeOrganizer{
		store: opts.Store, jobs: opts.Jobs, autoTags: opts.AutoTags, publisher: opts.Publisher,
		logger: logger.With(slog.String("component", "home-organizer")),
		settle: opts.Settle, poll: opts.Poll, now: opts.Now,
		// 容量 1 で、起こす知らせを 1 つだけ貯める。
		wake: make(chan struct{}, 1),
	}
	if o.settle <= 0 {
		o.settle = homeQueueSettle
	}
	if o.poll <= 0 {
		o.poll = homeQueuePoll
	}
	if o.now == nil {
		o.now = time.Now
	}
	return o
}

// Recover は settings に残った待ちの実行を読み、Run がそれを走らせるようにする（R-9）。Run の前に
// 1 度だけ呼ぶ。読めなければ、Run が Poll ごとに読み直す。
func (o *HomeOrganizer) Recover(ctx context.Context) error {
	err := o.loadPending(ctx)
	if err != nil {
		o.mu.Lock()
		o.reload = true
		o.mu.Unlock()
	}
	return err
}

// loadPending は settings の待ちの実行を読み、今の待ちに重ねる。読めたら reload を下ろす。
func (o *HomeOrganizer) loadPending(ctx context.Context) error {
	run, found, err := o.store.PendingOrganize(ctx)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.reload = false
	if found {
		o.pending = domain.MergeHomeOrganizeRun(o.pending, run)
	}
	o.mu.Unlock()
	if found {
		o.signal()
	}
	return nil
}

// reloadPending は reload が立っていれば settings の待ちの実行を読み直す。
func (o *HomeOrganizer) reloadPending(ctx context.Context) {
	o.mu.Lock()
	reload := o.reload
	o.mu.Unlock()
	if !reload {
		return
	}
	if err := o.loadPending(ctx); err != nil && ctx.Err() == nil {
		o.logger.Warn("cannot read the pending home organizer run", slog.Any("error", err))
	}
}

// Reorganize は rebuild を待ちに入れ、入れたあとの状態を返す（POST /api/home/organize）。
func (o *HomeOrganizer) Reorganize(ctx context.Context) (domain.HomeOrganizerState, error) {
	if err := o.request(ctx, domain.HomeRebuild); err != nil {
		return domain.HomeOrganizerState{}, err
	}
	return o.State(ctx)
}

// State は今の状態（走っているか、待ちの実行、最後に終わった実行）を返す。
func (o *HomeOrganizer) State(ctx context.Context) (domain.HomeOrganizerState, error) {
	o.mu.Lock()
	state := domain.HomeOrganizerState{Running: o.running != "", Pending: o.pending}
	o.mu.Unlock()
	last, found, err := o.store.LastOrganize(ctx)
	if err != nil {
		return domain.HomeOrganizerState{}, err
	}
	if found {
		state.Last = &last
	}
	return state, nil
}

// ScanFinished は走査が閉じたことを受ける（Scans.OnFinish）。done で終わったときだけ、待ち行列が
// 空のまま Settle だけ続くのを待ち始める。走査の錠を持って呼ばれるので、待つことはせずすぐ戻る。
func (o *HomeOrganizer) ScanFinished(_ domain.ScanOrigin, state domain.ScanState) {
	if state != domain.ScanDone {
		return
	}
	o.mu.Lock()
	o.afterScan = true
	o.emptySince = time.Time{}
	o.mu.Unlock()
	o.signal()
}

// Run は ctx が終わるまで、待ちの実行を 1 つずつ走らせ、走査のあとの待ち行列を見る。
func (o *HomeOrganizer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if run, ok := o.claim(); ok {
			o.execute(ctx, run)
			continue
		}
		awaiting, reload := o.waiting()
		if awaiting && o.checkQueues(ctx) {
			continue
		}
		var tick <-chan time.Time
		if awaiting || reload {
			tick = time.After(o.poll)
		}
		select {
		case <-ctx.Done():
			return
		case <-o.wake:
		case <-tick:
			o.reloadPending(ctx)
		}
	}
}

// request は run を待ちに入れる。rebuild は extend を吸収する。待ちが変わったら settings に書き、
// domain.HomeChanged を出して Run を起こす。
func (o *HomeOrganizer) request(ctx context.Context, run domain.HomeOrganizeRun) error {
	o.mu.Lock()
	merged := domain.MergeHomeOrganizeRun(o.pending, run)
	changed := merged != o.pending
	if changed {
		if err := o.store.SavePendingOrganize(ctx, domain.MergeHomeOrganizeRun(o.running, merged)); err != nil {
			o.mu.Unlock()
			return err
		}
		o.pending = merged
	}
	o.mu.Unlock()
	if changed {
		o.changed()
		o.signal()
	}
	return nil
}

// claim は走っている実行が無ければ待ちの実行を取り出し、走っている実行にする。
func (o *HomeOrganizer) claim() (domain.HomeOrganizeRun, bool) {
	o.mu.Lock()
	if o.running != "" || o.pending == "" {
		o.mu.Unlock()
		return "", false
	}
	run := o.pending
	o.running, o.pending = run, ""
	o.mu.Unlock()
	o.changed()
	return run, true
}

// execute は run を走らせ、終わりを home.organize.last に書く。走っているあいだに頼まれた実行は
// 待ちに残し、無ければ待ちを消す。止められて途切れた実行は記録せず、settings の待ちを残して次の起動に
// 回す。
func (o *HomeOrganizer) execute(ctx context.Context, run domain.HomeOrganizeRun) {
	applied, err := o.organize(ctx, run)
	if err != nil && ctx.Err() != nil {
		o.mu.Lock()
		o.running = ""
		o.mu.Unlock()
		return
	}
	last := domain.HomeOrganizeLast{Run: run, Outcome: domain.HomeOrganizeDone, FinishedAt: o.now().UnixMilli()}
	switch {
	case err != nil:
		o.logger.Error("the home organizer failed", slog.String("run", string(run)), slog.Any("error", err))
		last.Outcome = domain.HomeOrganizeFailed
	case !applied.Changed:
		last.Outcome = domain.HomeOrganizeUnchanged
	default:
		last.Added, last.Removed = applied.Added, applied.Removed
	}
	if err := o.store.SaveLastOrganize(ctx, last); err != nil {
		// 待ちの setting には run が残っているので、消さずに読み直して run を走らせ直す。
		o.logger.Error("cannot record the home organizer run; it runs again", slog.Any("error", err))
		o.mu.Lock()
		o.running, o.reload = "", true
		o.mu.Unlock()
		o.changed()
		return
	}
	o.mu.Lock()
	o.running = ""
	if o.pending != "" {
		err = o.store.SavePendingOrganize(ctx, o.pending)
	} else {
		err = o.store.ClearPendingOrganize(ctx)
	}
	o.mu.Unlock()
	if err != nil {
		o.logger.Error("cannot update the pending home organizer run", slog.Any("error", err))
	}
	o.changed()
}

// organize は規則の整理役で案を作り、適用する（R-2、R-10）。extend は棚にない動画だけから案を作る。
func (o *HomeOrganizer) organize(ctx context.Context, run domain.HomeOrganizeRun) (domain.HomeApplied, error) {
	candidates, err := o.store.ShelfCandidates(ctx, run == domain.HomeExtend)
	if err != nil {
		return domain.HomeApplied{}, err
	}
	return o.store.ApplyProposal(ctx, run, domain.OrganizeByRules(candidates.Candidates))
}

// waiting は、走査のあとの待ち行列を待っているかと、settings の待ちを読み直す必要があるかを返す。
func (o *HomeOrganizer) waiting() (awaiting, reload bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.afterScan, o.reload
}

// checkQueues は待ち行列を見て、空のまま Settle だけ続いたら走査のあとの実行を待ちに入れる（R-9）。
// 決めて待ちに入れ終えたら真を返す。読み書きに失敗したら走査のあとの待ちに戻し、次の Poll で
// やり直す。
func (o *HomeOrganizer) checkQueues(ctx context.Context) bool {
	since, settled := o.queuesSettled(ctx)
	if !settled {
		return false
	}
	if err := o.queueAfterScan(ctx); err != nil {
		if ctx.Err() == nil {
			o.logger.Error("cannot queue the home organizer run after a scan", slog.Any("error", err))
		}
		o.mu.Lock()
		// 待つあいだに次の走査が終わっていれば、その待ちを優先する。
		if !o.afterScan {
			o.afterScan, o.emptySince = true, since
		}
		o.mu.Unlock()
		return false
	}
	return true
}

// queuesSettled は待ち行列を見て、空のまま Settle だけ続いたかを返す。続いたら走査のあとの待ちを
// 下ろし、空と分かった最初の時刻を返す。読めなかった間は空と数えず、emptySince を零へ戻す。
func (o *HomeOrganizer) queuesSettled(ctx context.Context) (time.Time, bool) {
	empty, err := o.queuesEmpty(ctx)
	if err != nil && ctx.Err() == nil {
		o.logger.Warn("cannot read the queues for the home organizer", slog.Any("error", err))
	}
	now := o.now()
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.afterScan {
		return time.Time{}, false
	}
	if err != nil || !empty {
		o.emptySince = time.Time{}
		return time.Time{}, false
	}
	if o.emptySince.IsZero() {
		o.emptySince = now
	}
	if now.Sub(o.emptySince) < o.settle {
		return time.Time{}, false
	}
	since := o.emptySince
	o.afterScan, o.emptySince = false, time.Time{}
	return since, true
}

// queueAfterScan は走査のあとに走らせる実行を決め、あれば待ちに入れる。
func (o *HomeOrganizer) queueAfterScan(ctx context.Context) error {
	run, err := o.runAfterScan(ctx)
	if err != nil || run == "" {
		return err
	}
	return o.request(ctx, run)
}

// runAfterScan は走査のあとに走らせる実行を決める（R-9、R-10）。1 度も終わっていなければ rebuild、
// 所有者に見えて棚にない動画が domain.HomeExtendMinUnshelved 本以上なら extend で、どちらでもなければ
// 空を返す。
func (o *HomeOrganizer) runAfterScan(ctx context.Context) (domain.HomeOrganizeRun, error) {
	_, found, err := o.store.LastOrganize(ctx)
	if err != nil {
		return "", err
	}
	if !found {
		return domain.HomeRebuild, nil
	}
	unshelved, err := o.store.ShelfCandidates(ctx, true)
	if err != nil {
		return "", err
	}
	if unshelved.Videos < domain.HomeExtendMinUnshelved {
		return "", nil
	}
	return domain.HomeExtend, nil
}

// queuesEmpty は job の待ち行列と自動タグ付けの待ち行列がどちらも空かを返す。
func (o *HomeOrganizer) queuesEmpty(ctx context.Context) (bool, error) {
	if o.jobs != nil {
		busy, err := o.jobs.HasUnfinishedJobs(ctx)
		if err != nil || busy {
			return false, err
		}
	}
	if o.autoTags != nil {
		counts, err := o.autoTags.Counts(ctx)
		if err != nil || counts.Queued+counts.Running > 0 {
			return false, err
		}
	}
	return true, nil
}

// changed は状態が変わったことを domain.HomeChanged で知らせる（R-16）。
func (o *HomeOrganizer) changed() {
	if o.publisher != nil {
		o.publisher.Publish(domain.HomeChanged{})
	}
}

// signal は Run を起こす。ブロックしない。
func (o *HomeOrganizer) signal() {
	select {
	case o.wake <- struct{}{}:
	default:
	}
}
