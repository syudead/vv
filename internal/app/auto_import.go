package app

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	// autoImportQuiet は最後の変更から取り込みを始めるまでの静止の長さである
	// （specs/042-folder-watch-import/research.md R-3）。
	autoImportQuiet = 2 * time.Second
	// autoImportSettle はファイルの更新が止まってから取り込むまでの長さである。
	autoImportSettle = 10 * time.Second
)

// FolderWatcher はメディアフォルダの監視である。internal/watcher の *Watcher が満たす。
// 変更と問題の報告は、cmd/mdm が AutoImport.Changed と AutoImport.WatchProblem へつなぐ。
type FolderWatcher interface {
	// Arm は監視を roots の下のすべてのディレクトリへ張り替える。ディレクトリの一覧だけを読む。
	Arm(roots []string) error
	// Disarm は監視をすべて外す。
	Disarm()
}

// WatchScans は監視の走査の開始と、走っているものの打ち切りである。*Scans が満たす。
type WatchScans interface {
	StartWatchScan(ctx context.Context, dirs []domain.DirtyDirectory) (domain.Scan, bool, error)
	SupersedeWatchScan(ctx context.Context) bool
}

// AutoImportStore は自動の取り込みの入と切の保存先である。internal/store の
// *SettingsStore がこれを満たす。
type AutoImportStore interface {
	// AutoImport は保存された選択を返す。未設定は入である。
	AutoImport(ctx context.Context) (bool, error)
	SaveAutoImport(ctx context.Context, enabled bool) error
}

// ChangeProbe は dirs の読む範囲にあるファイルの、いちばん新しい更新の時刻を返す。
// ファイルが無いか、ディレクトリが無ければゼロ値を返す。
type ChangeProbe func(dir domain.DirtyDirectory) (time.Time, error)

// Clock は時刻と、時間の経過で動く関数の予約である。試験は偽の時計に差し替える。
type Clock interface {
	Now() time.Time
	// AfterFunc は d のあとに f を別の goroutine で呼ぶ。返した Timer で取り消せる。
	AfterFunc(d time.Duration, f func()) ClockTimer
}

// ClockTimer は AfterFunc の予約である。
type ClockTimer interface {
	Stop() bool
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) AfterFunc(d time.Duration, f func()) ClockTimer { return time.AfterFunc(d, f) }

// AutoImportOptions は自動の取り込みの組み立てに必要な依存である。
type AutoImportOptions struct {
	Store   AutoImportStore
	Folders ActivityFolderStore
	Watcher FolderWatcher
	Scans   WatchScans
	// Probe はファイルの更新が止まったかを調べる。nil なら調べず、静止のあとすぐ取り込む。
	Probe ChangeProbe
	// Clock は nil なら実時間を使う。
	Clock Clock
	// Quiet と Settle は 0 なら既定（2 秒と 10 秒）を使う。
	Quiet  time.Duration
	Settle time.Duration
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// AutoImport はメディアフォルダの変更の報告を変わったディレクトリの集合にまとめ、静止した
// あとで 1 つの監視の走査として取り込む（specs/042-folder-watch-import/research.md R-2・R-3・R-5）。
// 起動のときも、入にしたときも、走査は始めない。
type AutoImport struct {
	store   AutoImportStore
	folders ActivityFolderStore
	watcher FolderWatcher
	scans   WatchScans
	probe   ChangeProbe
	clock   Clock
	quiet   time.Duration
	settle  time.Duration
	logger  *slog.Logger

	// setMu は入と切の切り替えと、フォルダの変更に伴う張り直しを 1 つずつにする。
	setMu sync.Mutex

	mu      sync.Mutex
	ctx     context.Context
	started bool
	stopped bool
	enabled bool
	state   domain.FolderWatchState
	problem *domain.FolderWatchProblem
	// armGen は張り直しのたびに増え、古い張り込みの結果を捨てるのに使う。
	armGen int
	arms   sync.WaitGroup

	// dirty は読み直す必要のあるディレクトリである。
	dirty []domain.DirtyDirectory
	// inflight は走っている監視の走査が読んでいるディレクトリである。
	inflight []domain.DirtyDirectory
	timer    ClockTimer
	timerSeq int
	// batching は取り込みの判断が走っていることを、rerun はその間に新しい予約があったことを表す。
	batching bool
	rerun    bool
	// holding はメディアフォルダの変更の途中にある数で、0 でない間は監視の走査を始めない。
	holding int
	// finishes は走査が閉じた回数。判断の途中で走査が閉じたかを見るのに使う。
	finishes int
}

// NewAutoImport は自動の取り込みを組み立てる。Start を呼ぶまで何も監視しない。
func NewAutoImport(opts AutoImportOptions) *AutoImport {
	a := &AutoImport{
		store:   opts.Store,
		folders: opts.Folders,
		watcher: opts.Watcher,
		scans:   opts.Scans,
		probe:   opts.Probe,
		clock:   opts.Clock,
		quiet:   opts.Quiet,
		settle:  opts.Settle,
		logger:  opts.Logger,
		enabled: true,
		state:   domain.FolderWatchOff,
	}
	if a.clock == nil {
		a.clock = systemClock{}
	}
	if a.quiet <= 0 {
		a.quiet = autoImportQuiet
	}
	if a.settle <= 0 {
		a.settle = autoImportSettle
	}
	if a.logger == nil {
		a.logger = slog.Default()
	}
	return a
}

// Start は保存された選択を読み、入なら監視を張る。張るのは背後で行うので、すぐ戻る。
// 走査は始めない。ctx は監視の走査を始めるときに使う、寿命の長い context である。
func (a *AutoImport) Start(ctx context.Context) error {
	enabled, err := a.store.AutoImport(ctx)
	if err != nil {
		return err
	}
	a.setMu.Lock()
	defer a.setMu.Unlock()
	a.mu.Lock()
	a.ctx = ctx
	a.started = true
	a.enabled = enabled
	a.mu.Unlock()
	if enabled {
		a.arm(ctx)
	}
	return nil
}

// Stop は監視を外し、予約を取り消し、張り込みの終わりを待つ。走っている監視の走査は止めない
// （走査の寿命の context の取り消しが止める）。Stop のあとは何も始めない。
func (a *AutoImport) Stop() {
	a.mu.Lock()
	a.stopped = true
	a.cancelTimerLocked()
	a.armGen++
	a.mu.Unlock()
	a.watcher.Disarm()
	a.arms.Wait()
}

// Status は保存された選択と、監視の今の状態を返す。
func (a *AutoImport) Status() domain.AutoImportStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.statusLocked()
}

func (a *AutoImport) statusLocked() domain.AutoImportStatus {
	status := domain.AutoImportStatus{Enabled: a.enabled, State: a.state}
	if !a.enabled {
		status.State = domain.FolderWatchOff
	}
	if status.State == domain.FolderWatchLimited && a.problem != nil {
		problem := *a.problem
		status.Problem = &problem
	}
	return status
}

// SetEnabled は選択を保存し、入なら監視を張り、切なら外して変わったディレクトリの集合を捨てる。
// 走査は始めない。今と同じ値なら何もしない。
func (a *AutoImport) SetEnabled(ctx context.Context, enabled bool) (domain.AutoImportStatus, error) {
	a.setMu.Lock()
	defer a.setMu.Unlock()
	a.mu.Lock()
	same := a.enabled == enabled
	a.mu.Unlock()
	if same {
		return a.Status(), nil
	}
	if !enabled {
		// 走っている取り込みを止めてから切る。
		a.scans.SupersedeWatchScan(ctx)
	}
	if err := a.store.SaveAutoImport(ctx, enabled); err != nil {
		return domain.AutoImportStatus{}, err
	}
	a.mu.Lock()
	a.enabled = enabled
	a.problem = nil
	if !enabled {
		a.state = domain.FolderWatchOff
		a.dirty, a.inflight = nil, nil
		a.cancelTimerLocked()
		a.armGen++
	}
	a.mu.Unlock()
	if enabled {
		a.arm(context.WithoutCancel(ctx))
	} else {
		a.watcher.Disarm()
	}
	return a.Status(), nil
}

// FoldersChanging はメディアフォルダを変える前に呼ぶ。走っている監視の走査を止め、その
// ディレクトリを変わったものとして戻す（research.md R-5）。保存側は走査中のフォルダの変更を
// 断るので、変更の前に止める。
//
// 変更が終わる（FoldersChanged）まで、新しい監視の走査は始めない。
func (a *AutoImport) FoldersChanging(ctx context.Context) {
	a.mu.Lock()
	a.holding++
	batch := slices.Clone(a.inflight)
	a.mu.Unlock()
	if !a.scans.SupersedeWatchScan(ctx) {
		return
	}
	a.mu.Lock()
	a.dirty = append(a.dirty, batch...)
	if a.inflight != nil {
		a.inflight = nil
	}
	a.mu.Unlock()
}

// FoldersChanged はメディアフォルダの変更を試みたあとに、成否にかかわらず呼ぶ。監視を張り直し、
// 戻したディレクトリを取り込みの予約に載せる。
func (a *AutoImport) FoldersChanged(ctx context.Context) {
	a.setMu.Lock()
	defer a.setMu.Unlock()
	a.mu.Lock()
	a.holding--
	enabled := a.enabled && a.started && !a.stopped
	if enabled {
		a.problem = nil
		if len(a.dirty) > 0 {
			a.scheduleLocked(a.quiet)
		}
	}
	a.mu.Unlock()
	if enabled {
		a.arm(context.WithoutCancel(ctx))
	}
}

// Changed は watcher が報告した変更を受ける。変わったディレクトリの集合に足し、静止の予約を延ばす。
func (a *AutoImport) Changed(dir domain.DirtyDirectory) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled || !a.started || a.stopped {
		return
	}
	a.dirty = append(a.dirty, dir)
	a.scheduleLocked(a.quiet)
}

// WatchProblem は watcher が報告した問題を、設定画面に見せる状態へ記録する。
func (a *AutoImport) WatchProblem(problem domain.FolderWatchProblem) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled || a.stopped {
		return
	}
	a.problem = &problem
	a.state = domain.FolderWatchLimited
}

// arm は監視を背後で張り直す。setMu を持って呼ぶ。メディアフォルダが無ければ、張るものが無いので off になる。
func (a *AutoImport) arm(ctx context.Context) {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.armGen++
	gen := a.armGen
	a.state = domain.FolderWatchStarting
	a.problem = nil
	a.arms.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.arms.Done()
		a.armTree(ctx, gen)
	}()
}

func (a *AutoImport) armTree(ctx context.Context, gen int) {
	folders, err := a.folders.ListMediaFolders(ctx)
	if err != nil {
		a.logger.Warn("could not list the media folders to watch", slog.Any("error", err))
		a.mu.Lock()
		if gen == a.armGen {
			a.state = domain.FolderWatchLimited
			a.problem = &domain.FolderWatchProblem{Kind: domain.FolderWatchProblemFolderUnreachable}
		}
		a.mu.Unlock()
		return
	}
	roots := make([]string, 0, len(folders))
	for _, folder := range folders {
		roots = append(roots, folder.Path)
	}
	if len(roots) == 0 {
		a.watcher.Disarm()
		a.mu.Lock()
		if gen == a.armGen {
			a.state = domain.FolderWatchOff
		}
		a.mu.Unlock()
		return
	}
	if err := a.watcher.Arm(roots); err != nil {
		a.logger.Warn("could not watch the media folders", slog.Any("error", err))
	}
	a.mu.Lock()
	if gen != a.armGen {
		// 張り直しに替わったか、切った・止めたあとである。切った・止めたあとに監視が残らないようにする。
		off := !a.enabled || a.stopped
		a.mu.Unlock()
		if off {
			a.watcher.Disarm()
		}
		return
	}
	defer a.mu.Unlock()
	if a.problem == nil {
		a.state = domain.FolderWatchActive
	}
}

// ScanFinished は走査が閉じたことを受ける。監視の走査なら、読んでいたディレクトリを手放す。
// 取り込めずに残ったディレクトリがあれば、静止の予約を入れ直す。手動の走査が done で閉じたなら、
// 全体を読み直したので、通知を取りこぼした問題は解ける（contracts/screen-api.md）。
func (a *AutoImport) ScanFinished(origin domain.ScanOrigin, state domain.ScanState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finishes++
	if origin == domain.ScanOriginWatch {
		a.inflight = nil
	}
	if origin == domain.ScanOriginManual && state == domain.ScanDone &&
		a.problem != nil && a.problem.Kind == domain.FolderWatchProblemEventsLost {
		a.problem = nil
		if a.state == domain.FolderWatchLimited {
			a.state = domain.FolderWatchActive
		}
	}
	if a.enabled && !a.stopped && len(a.dirty) > 0 {
		a.scheduleLocked(a.quiet)
	}
}

func (a *AutoImport) cancelTimerLocked() {
	a.timerSeq++
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

// scheduleLocked は d のあとの取り込みの判断を予約し、前の予約は取り消す。
func (a *AutoImport) scheduleLocked(d time.Duration) {
	if a.stopped {
		return
	}
	a.cancelTimerLocked()
	seq := a.timerSeq
	a.timer = a.clock.AfterFunc(d, func() { a.fire(seq) })
}

// fire は静止のあとの判断である。更新が止まっていないファイルのあるディレクトリは変わったまま
// 残し、止まったディレクトリだけを 1 つの監視の走査として始める（research.md R-3）。
func (a *AutoImport) fire(seq int) {
	a.mu.Lock()
	if seq != a.timerSeq || a.stopped || !a.enabled || a.ctx == nil {
		a.mu.Unlock()
		return
	}
	a.timer = nil
	if a.batching {
		a.rerun = true
		a.mu.Unlock()
		return
	}
	if a.holding > 0 || a.inflight != nil || len(a.dirty) == 0 {
		// 走っている走査が閉じたとき（ScanFinished）と、フォルダの変更が終わったとき
		// （FoldersChanged）に、残りを予約し直す。
		a.mu.Unlock()
		return
	}
	a.batching = true
	taken := domain.NormalizeDirtyDirectories(a.dirty)
	a.dirty = nil
	ctx := a.ctx
	finishes := a.finishes
	a.mu.Unlock()

	settled, unsettled, wait := a.settledSplit(taken)

	a.mu.Lock()
	a.dirty = append(a.dirty, unsettled...)
	if len(unsettled) > 0 && !a.stopped && a.enabled {
		a.scheduleLocked(wait)
	}
	if len(settled) > 0 {
		a.inflight = settled
	}
	a.mu.Unlock()

	if len(settled) > 0 {
		a.start(ctx, settled, finishes)
	}

	a.mu.Lock()
	a.batching = false
	if a.rerun {
		a.rerun = false
		if len(a.dirty) > 0 {
			a.scheduleLocked(a.quiet)
		}
	}
	a.mu.Unlock()
}

// start は settled を監視の走査として始める。始められなければ、変わったままに戻す。
func (a *AutoImport) start(ctx context.Context, settled []domain.DirtyDirectory, finishes int) {
	_, started, err := a.scans.StartWatchScan(ctx, settled)
	if err == nil && started {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inflight = nil
	a.dirty = append(a.dirty, settled...)
	if a.stopped || !a.enabled {
		return
	}
	switch {
	case err != nil:
		a.logger.Warn("could not start a watch scan", slog.Any("error", err))
		a.scheduleLocked(a.settle)
	case a.finishes != finishes:
		// 断られている間に走っていた走査が閉じた。閉じる知らせは取り込みを予約し損ねている。
		a.scheduleLocked(a.quiet)
	default:
		// 別の走査が走っている。閉じる知らせ（ScanFinished）が予約する。
	}
}

// settledSplit は dirs を、更新が止まったものと、まだのものに分ける。まだのものについては、
// 次に調べるまでの待ち時間も返す。調べられないディレクトリは止まったものとして扱う
// （走査が読めない理由を問題として記録する）。
func (a *AutoImport) settledSplit(dirs []domain.DirtyDirectory) (settled, unsettled []domain.DirtyDirectory, wait time.Duration) {
	if a.probe == nil {
		return dirs, nil, 0
	}
	now := a.clock.Now()
	wait = a.settle
	for _, dir := range dirs {
		newest, err := a.probe(dir)
		if err != nil {
			a.logger.Warn("could not check whether a directory has settled",
				slog.String("path", dir.Path), slog.Any("error", err))
			settled = append(settled, dir)
			continue
		}
		age := now.Sub(newest)
		// 未来の時刻のファイルは、時計の違うところで書かれたものとして、止まったと見る。
		if newest.IsZero() || age >= a.settle || age < -a.settle {
			settled = append(settled, dir)
			continue
		}
		unsettled = append(unsettled, dir)
		wait = min(wait, max(a.settle-age, time.Second))
	}
	return settled, unsettled, wait
}
