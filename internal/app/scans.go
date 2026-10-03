package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// ScanStore は走査の記録先である。
type ScanStore interface {
	// StartScan は走査の行を running で作る。実行中のものがあれば作らずに
	// それを返し、started は false になる。
	StartScan(ctx context.Context) (scan domain.Scan, started bool, err error)
	// ResumeScan は StartScan と同じだが、中断で終わった走査 from の対象の動画の
	// 集合と、仕事の段階の問題（失敗と代用）を新しい走査へ持ち越す。
	ResumeScan(ctx context.Context, from int64) (scan domain.Scan, started bool, err error)
	CurrentScan(ctx context.Context) (domain.Scan, error)
	UpdateScanProgress(ctx context.Context, id int64, progress domain.ScanProgress) error
	// FinishScan は走査を閉じる。cause は走査そのものが失敗した理由（成功なら nil）で、
	// domain.ScanFailure で包まれた理由のコードと場所は保存側が取り出す。
	FinishScan(ctx context.Context, id int64, state domain.ScanState, cause error) error
	// FailInterruptedScans は running のまま残った走査を failed で閉じる。
	FailInterruptedScans(ctx context.Context) (int64, error)
	// RecordScanIssue は走査が1つのファイルで出会った失敗を、直近の取り込みの問題と
	// して記録する。
	RecordScanIssue(ctx context.Context, issue domain.ScanFileIssue) error
	// ScanIssues は走査 scanID の問題を、まとめて並べた形で返す。所在がどの登録
	// フォルダにも含まれない件は含めない。
	ScanIssues(ctx context.Context, scanID int64) ([]domain.ScanIssue, error)
}

// JobRecoveryStore は中断した取り込みジョブを待ち行列へ戻す保存先である。
type JobRecoveryStore interface {
	RequeueRunningJobs(ctx context.Context) (int64, error)
}

// FolderIndexStore はフォルダの索引（グループの割り当てと祖先フォルダ名）の
// 作り直し先である。作り直しに失敗したら、保存側が索引を古いと記録してから
// 失敗を返す（specs/017-folder-groups/data-model.md §3）。
type FolderIndexStore interface {
	RebuildFolderIndex(ctx context.Context) error
}

// Scanner は登録済みのメディアフォルダを1回走査する。internal/scanner の
// *Scanner がこれを満たす。
type Scanner interface {
	Scan(ctx context.Context) (domain.ScanResult, error)
}

// ScanReporter は走査の進捗・今のファイル・ファイルごとの失敗の報告先である。
// *Scans がこれを満たし、ScansOptions.NewScanner に渡される。
type ScanReporter interface {
	ReportScanProgress(ctx context.Context, result domain.ScanResult) error
	ReportScanIssue(ctx context.Context, issue domain.ScanFileIssue) error
	ReportScanFile(path string, videoID int64)
}

// ActivityFolderStore は、今の処理のファイルが置かれたフォルダを求めるための
// 登録フォルダの読み出し先である。
type ActivityFolderStore interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
}

// Publisher は状態の変化の発行先である。誰が受け取るか（画面への知らせ・
// ワーカーの起床など）は知らない。購読者の登録は cmd/mdm が行う。
type Publisher interface {
	Publish(events ...domain.Event)
}

// ScansOptions は走査の組み立てに必要な依存である。
type ScansOptions struct {
	Store ScanStore
	Jobs  JobRecoveryStore
	// FolderIndex はスキャンを閉じる直前と、中断したスキャンを閉じたときに
	// フォルダの索引を作り直す。nil なら作り直さない。
	FolderIndex FolderIndexStore
	// NewScanner は進捗の報告先を受け取って走査を組み立てる。走査は報告先を、
	// 報告先は走査を必要とするので、組み立てを関数で受け取る。
	NewScanner func(ScanReporter) Scanner
	// Context は走査に使う寿命の長い context。停止時に取り消され、走査は
	// failed で閉じる。nil なら context.Background を使う。
	Context context.Context
	// Folders は今の処理のフォルダを求めるのに使う。nil ならフォルダを省く。
	Folders ActivityFolderStore
	// Publisher は nil なら発行しない。
	Publisher Publisher
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Scans は走査の開始・実行・進捗の記録と、起動時の中断からの回復を受け持つ。
type Scans struct {
	store   ScanStore
	jobs    JobRecoveryStore
	folders FolderIndexStore
	roots   ActivityFolderStore
	scanner Scanner
	// lifetime は組み立て時に渡した寿命の長い context。取り消しは stopRuns が
	// 走査の context へ伝えるが、それは非同期なので、中断かどうかの判定では
	// こちらも直接見る。
	lifetime  context.Context
	publisher Publisher
	logger    *slog.Logger
	// activity は今の処理をメモリに持つ（specs/024-import-progress/research.md R-8）。
	activity *activities

	// mu は走査の起動が重ならないようにする。実行中かどうかの判断は
	// scans 表（部分ユニーク索引）が持つので、ここは goroutine を
	// 二重に起こさないためだけの錠である。
	mu      sync.Mutex
	running bool
	// cancelRun は走っている走査の context を取り消す。走っていなければ nil。
	cancelRun context.CancelFunc
	// stopped は組み立て時の context が取り消されたことを表す。以後に始める
	// 走査は、始めた直後に取り消す。
	stopped bool
	// done は背後で走っている走査の終わりを待つ。
	done sync.WaitGroup
}

// NewScans は走査を組み立てる。
func NewScans(opts ScansOptions) *Scans {
	s := &Scans{
		store:     opts.Store,
		jobs:      opts.Jobs,
		folders:   opts.FolderIndex,
		roots:     opts.Folders,
		lifetime:  opts.Context,
		publisher: opts.Publisher,
		logger:    opts.Logger,
	}
	if s.lifetime == nil {
		s.lifetime = context.Background()
	}
	context.AfterFunc(s.lifetime, s.stopRuns)
	if s.logger == nil {
		s.logger = slog.Default()
	}
	s.activity = newActivities(s.activityChanged)
	if opts.NewScanner != nil {
		s.scanner = opts.NewScanner(s)
	}
	return s
}

// StartScan は取り込みを開始する。実行中なら新しく始めず、実行中のものを返す。
// 応答は即座に返り、走査は背後で進む。started は走査の行を新しく作ったかで、
// 作るか実行中の行を返すかを 1 つのトランザクションで決めた保存先の答えをそのまま返す
// （specs/026-external-api/contracts/external-api.md §5）。
//
// ctx は要求のものなので、その取り消しを走査へは持ち込まない。要求が終わった
// 時点で走査が打ち切られてしまう。走査は組み立て時に渡した寿命の長い context の
// 取り消しでだけ止まる。
func (s *Scans) StartScan(ctx context.Context) (domain.Scan, bool, error) {
	return s.start(ctx, s.store.StartScan)
}

// start は open で走査の行を作り、新しく作ったなら背後で走らせる。
func (s *Scans) start(
	ctx context.Context, open func(context.Context) (domain.Scan, bool, error),
) (domain.Scan, bool, error) {
	scan, started, err := open(ctx)
	if err != nil {
		return domain.Scan{}, false, err
	}
	if !started {
		scan, err = s.withImport(ctx, scan)
		return scan, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		scan, err = s.withImport(ctx, scan)
		return scan, true, err
	}
	s.running = true
	s.done.Add(1)

	// 走査は要求の ctx の取り消しを受け継がない（WithoutCancel）。受け継ぐと
	// 応答を返した時点で走査が打ち切られる。止めるのは、組み立て時に渡した
	// 寿命の長い context が取り消されたときの stopRuns だけである。
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.cancelRun = cancel
	if s.stopped {
		cancel()
	}
	go s.run(runCtx, scan.ID)

	s.scanChanged()
	scan, err = s.withImport(ctx, scan)
	return scan, true, err
}

// CurrentScan は直近の走査を、利用者に見せる取り込みの状態（Scan.Import）を
// 組み立てて返す。
func (s *Scans) CurrentScan(ctx context.Context) (domain.Scan, error) {
	scan, err := s.store.CurrentScan(ctx)
	if err != nil {
		return domain.Scan{}, err
	}
	return s.withImport(ctx, scan)
}

// ListScanIssues は直近の取り込みの問題を、cursor の次から limit 件返す
// （specs/024-import-progress/contracts/scan-api.md §3）。一度も走査していなければ
// domain.ErrNotFound、カーソルが解釈できなければ domain.ErrInvalidCursor を返す。
func (s *Scans) ListScanIssues(ctx context.Context, cursor string, limit int) (domain.ScanIssuePage, error) {
	scan, err := s.store.CurrentScan(ctx)
	if err != nil {
		return domain.ScanIssuePage{}, err
	}
	issues, err := s.store.ScanIssues(ctx, scan.ID)
	if err != nil {
		return domain.ScanIssuePage{}, err
	}
	items, next, err := domain.PageScanIssues(issues, cursor, limit)
	if err != nil {
		return domain.ScanIssuePage{}, err
	}
	return domain.ScanIssuePage{ScanID: scan.ID, Items: items, NextCursor: next}, nil
}

// withImport は走査の記録と問題から取り込みの状態を組み立てる。状態の決め方は
// domain が持つ（specs/024-import-progress/research.md R-4・R-5）。本数は、問題を
// 動画（未登録ならファイル）ごとにまとめた件で数える。
func (s *Scans) withImport(ctx context.Context, scan domain.Scan) (domain.Scan, error) {
	issues, err := s.store.ScanIssues(ctx, scan.ID)
	if err != nil {
		return domain.Scan{}, err
	}
	scan.Issues = domain.CountScanIssues(issues)
	scan.Import = scan.Tally(scan.Issues.Unregistered, scan.Issues.Failed).Progress(scan.SettledAt)
	activity, err := s.currentActivity(ctx)
	if err != nil {
		return domain.Scan{}, err
	}
	scan.Activity = activity
	return scan, nil
}

// currentActivity は今の処理を、ファイルの置かれたフォルダを添えて返す。
func (s *Scans) currentActivity(ctx context.Context) (domain.ScanActivity, error) {
	activity := s.activity.current()
	if !activity.Active() || s.roots == nil {
		return activity, nil
	}
	roots, err := s.roots.ListMediaFolders(ctx)
	if err != nil {
		return domain.ScanActivity{}, err
	}
	return activity.Locate(roots), nil
}

// ReportScanFile は走査が登録を始めるファイルを、今の処理として記録する。videoID は
// 走査が知っている既存の動画（知らなければ 0）である。path が空なら、ファイルの
// 登録をすべて終えたことを表す。
func (s *Scans) ReportScanFile(path string, videoID int64) {
	if path == "" {
		s.activity.end(scanActivityKey)
		return
	}
	s.activity.begin(scanActivityKey, domain.ScanActivity{
		Kind: domain.ActivityRegistering, VideoID: videoID, Path: path,
	})
}

// JobStarted は仕事の処理が始まったことを、今の処理として記録する。ワーカーの
// Started に渡す。
func (s *Scans) JobStarted(job domain.Job) {
	kind, ok := domain.ActivityKindOf(job.Kind)
	if !ok || job.LocationPath == "" {
		return
	}
	s.activity.begin(jobActivityKey(job), domain.ScanActivity{
		Kind: kind, VideoID: job.VideoID, Path: job.LocationPath,
	})
}

// JobFinished は仕事の処理が終わったことを記録する。ワーカーの Finished に渡す。
func (s *Scans) JobFinished(job domain.Job) {
	s.activity.end(jobActivityKey(job))
}

// ReportScanIssue は走査が1つのファイルで出会った失敗を、直近の取り込みの問題として
// 記録する。本数が変わるので、走査の変化として知らせる。
func (s *Scans) ReportScanIssue(ctx context.Context, issue domain.ScanFileIssue) error {
	if err := s.store.RecordScanIssue(ctx, issue); err != nil {
		return err
	}
	s.scanChanged()
	return nil
}

// ReportScanProgress は走査の進捗を記録する。走査中も一覧・再生は通常どおり
// 応答するので、ここでは行を1つ書き換えるだけにする。
func (s *Scans) ReportScanProgress(ctx context.Context, result domain.ScanResult) error {
	if err := s.store.UpdateScanProgress(ctx, s.currentScanID(ctx), progressOf(result)); err != nil {
		return err
	}
	s.scanChanged()
	return nil
}

// Wait は背後で走っている走査の終わりを待つ。組み立て時の context を
// 取り消したあとに呼ぶ。
//
// cmd/mdm は停止時に、変化の配り先を閉じる前にこれを猶予つきで待つ。走査は
// 取り消しを見て止まるので、通常は長くは待たない。途中で配り先を閉じると、走査が
// 消した動画の知らせが捨てられ、その生成物が残り続ける。
func (s *Scans) Wait() {
	s.done.Wait()
}

// RecoverInterrupted は前回の停止で中途半端に残った状態を戻す。
//
// running のまま残った走査を閉じないと、「実行中は1件だけ」の制約が働いた
// まま二度と取り込みを始められなくなる。閉じた走査があれば、閉じる直前の
// フォルダの索引の作り直しも済んでいないので、ここで作り直す（索引の表だけを
// 読み、ファイルシステムは読まない）。running のまま残った仕事は queued へ
// 戻し、各段階のワーカーが続きから処理する。
func (s *Scans) RecoverInterrupted(ctx context.Context) error {
	closed, err := s.store.FailInterruptedScans(ctx)
	if err != nil {
		return err
	}
	if closed > 0 {
		s.logger.Info("closed interrupted scans", slog.Int64("count", closed))
		s.rebuildFolderIndex(ctx)
	}

	restored, err := s.jobs.RequeueRunningJobs(ctx)
	if err != nil {
		return err
	}
	if restored > 0 {
		s.logger.Info("requeued interrupted jobs", slog.Int64("count", restored))
	}
	return nil
}

// ResumeInterrupted は、最新の走査が中断（failed、理由 interrupted）で終わって
// いれば、新しい走査を 1 回始める（specs/037-windows-app/research.md R-9）。始めたら
// true を返す。
//
// interrupted は、停止の指示で打ち切った走査と、running のまま残って
// RecoverInterrupted が閉じた走査の両方に付く。利用者が走査を取り消す操作は無いので、
// どちらもプロセスの停止による。走査はサイズと mtime が変わらないファイルを何も
// しないで通るので、始め直しは続きからと同じ結果になる。ただし変わらないファイルの
// 上限まで失敗した仕事は積み直されないので、中断した走査の対象の動画の集合と
// 仕事の段階の問題は新しい走査へ持ち越す（ScanStore.ResumeScan）。最新の走査が done、
// interrupted 以外の理由の failed、または走査の記録が無いときは始めない。
//
// 起動時に、RecoverInterrupted のあと、ワーカーを動かしてから 1 度だけ呼ぶ。
func (s *Scans) ResumeInterrupted(ctx context.Context) (bool, error) {
	latest, err := s.store.CurrentScan(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if latest.State != domain.ScanFailed || latest.ErrorCode != domain.ScanErrorInterrupted {
		return false, nil
	}
	scan, started, err := s.start(ctx, func(ctx context.Context) (domain.Scan, bool, error) {
		return s.store.ResumeScan(ctx, latest.ID)
	})
	if err != nil {
		return false, err
	}
	if started {
		s.logger.Info("resumed the interrupted scan",
			slog.Int64("interrupted_scan", latest.ID), slog.Int64("scan", scan.ID))
	}
	return started, nil
}

// currentScanID は進捗の書き込み先を返す。取れない場合は 0 を返し、
// 書き込みは何にも当たらない（走査は続ける）。
func (s *Scans) currentScanID(ctx context.Context) int64 {
	scan, err := s.store.CurrentScan(ctx)
	if err != nil {
		return 0
	}
	return scan.ID
}

// stopRuns は組み立て時の context が取り消されたときに呼ばれ、走っている
// 走査を止める。以後に始める走査も、始めた直後に止まる。
func (s *Scans) stopRuns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.cancelRun != nil {
		s.cancelRun()
	}
}

// run は走査を最後まで走らせ、結果を記録する。
func (s *Scans) run(ctx context.Context, scanID int64) {
	defer s.done.Done()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.cancelRun()
		s.cancelRun = nil
		s.mu.Unlock()
	}()

	result, scanErr := func() (result domain.ScanResult, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("scan panicked: %v", recovered)
			}
		}()
		if s.scanner == nil {
			return domain.ScanResult{}, errors.New("scanner is not configured")
		}
		return s.scanner.Scan(ctx)
	}()
	// 走査が途中で止まっても、登録中のファイルを今の処理に残さない。
	s.activity.end(scanActivityKey)

	// 停止指示で打ち切った場合は、失敗として閉じる。次の起動で走り直せる。
	// 理由は interrupted で、走査が包んだ理由より優先する（data-model.md §2）。
	state := domain.ScanDone
	if scanErr != nil {
		state = domain.ScanFailed
		if ctx.Err() != nil || s.lifetime.Err() != nil {
			scanErr = domain.NewScanFailure(domain.ScanErrorInterrupted, "",
				fmt.Errorf("the scan was stopped before it finished: %w", scanErr))
		}
		s.logger.Warn("scan did not finish", slog.Any("error", scanErr))
	} else {
		s.logger.Info("scan finished",
			slog.Int("total", result.Total),
			slog.Int("added", result.Added),
			slog.Int("updated", result.Updated),
			slog.Int("moved", result.Moved),
			slog.Int("removed", result.Removed),
			slog.Int("failed", result.Failed),
		)
	}

	// 停止済みでも記録は残す。context を引き継ぐと、閉じる書き込み自体が
	// 打ち切られて running のまま残る。
	closeCtx := context.WithoutCancel(ctx)

	if err := s.store.UpdateScanProgress(closeCtx, scanID, progressOf(result)); err != nil {
		s.logger.Warn("could not record scan progress", slog.Any("error", err))
	}

	// 成功でも失敗でも、閉じる直前にフォルダの索引を作り直す。読むのは索引の
	// 表（SQLite）だけで、ファイルシステムは読まない。ファイルシステムを歩くのは
	// 利用者が取り込みを始めたときの走査だけである。
	s.rebuildFolderIndex(closeCtx)

	if err := s.store.FinishScan(closeCtx, scanID, state, scanErr); err != nil {
		s.logger.Warn("could not record the end of the scan", slog.Any("error", err))
	}
	s.scanChanged()
}

// rebuildFolderIndex はフォルダの索引を作り直す。失敗してもスキャンは失敗に
// せず、ログに残すだけにする。保存側が索引を古いと記録しているので、次の作り直しの
// 時点（次の起動を含む）で直る。
func (s *Scans) rebuildFolderIndex(ctx context.Context) {
	if s.folders == nil {
		return
	}
	if err := s.folders.RebuildFolderIndex(ctx); err != nil {
		s.logger.Warn("could not rebuild the folder index", slog.Any("error", err))
	}
}

// activityChanged は今の処理が変わったことを発行する。
func (s *Scans) activityChanged() {
	if s.publisher == nil {
		return
	}
	s.publisher.Publish(domain.ScanActivityChanged{})
}

// scanChanged は走査の状態が変わったことを発行する。
func (s *Scans) scanChanged() {
	if s.publisher == nil {
		return
	}
	s.publisher.Publish(domain.ScanChanged{})
}

func progressOf(result domain.ScanResult) domain.ScanProgress {
	return domain.ScanProgress{
		Total:     result.Total,
		Completed: result.Completed(),
		Failed:    result.Failed,
	}
}
