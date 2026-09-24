package app

import (
	"context"
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
	CurrentScan(ctx context.Context) (domain.Scan, error)
	UpdateScanProgress(ctx context.Context, id int64, progress domain.ScanProgress) error
	FinishScan(ctx context.Context, id int64, state domain.ScanState, reason string) error
	// FailInterruptedScans は running のまま残った走査を failed で閉じる。
	FailInterruptedScans(ctx context.Context) (int64, error)
}

// JobRecoveryStore は中断した取り込みジョブを待ち行列へ戻す保存先である。
type JobRecoveryStore interface {
	RequeueRunningJobs(ctx context.Context) (int64, error)
}

// Scanner は登録済みのメディアフォルダを1回走査する。internal/scanner の
// *Scanner がこれを満たす。
type Scanner interface {
	Scan(ctx context.Context) (domain.ScanResult, error)
}

// ScanReporter は走査の進捗の報告先である。*Scans がこれを満たし、
// ScansOptions.NewScanner に渡される。
type ScanReporter interface {
	ReportScanProgress(ctx context.Context, result domain.ScanResult) error
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
	// NewScanner は進捗の報告先を受け取って走査を組み立てる。走査は報告先を、
	// 報告先は走査を必要とするので、組み立てを関数で受け取る。
	NewScanner func(ScanReporter) Scanner
	// Context は走査に使う寿命の長い context。停止時に取り消され、走査は
	// failed で閉じる。nil なら context.Background を使う。
	Context context.Context
	// Publisher は nil なら発行しない。
	Publisher Publisher
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Scans は走査の開始・実行・進捗の記録と、起動時の中断からの回復を受け持つ。
type Scans struct {
	store     ScanStore
	jobs      JobRecoveryStore
	scanner   Scanner
	baseCtx   context.Context
	publisher Publisher
	logger    *slog.Logger

	// mu は走査の起動が重ならないようにする。実行中かどうかの判断は
	// scans 表（部分ユニーク索引）が持つので、ここは goroutine を
	// 二重に起こさないためだけの錠である。
	mu      sync.Mutex
	running bool
	// done は背後で走っている走査の終わりを待つ。
	done sync.WaitGroup
}

// NewScans は走査を組み立てる。
func NewScans(opts ScansOptions) *Scans {
	s := &Scans{
		store:     opts.Store,
		jobs:      opts.Jobs,
		baseCtx:   opts.Context,
		publisher: opts.Publisher,
		logger:    opts.Logger,
	}
	if s.baseCtx == nil {
		s.baseCtx = context.Background()
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if opts.NewScanner != nil {
		s.scanner = opts.NewScanner(s)
	}
	return s
}

// StartScan は取り込みを開始する。実行中なら新しく始めず、実行中のものを返す。
// 応答は即座に返り、走査は背後で進む。
//
// ctx は要求のものなので、走査自体には使わない。要求が終わった時点で走査が
// 打ち切られてしまう。走査は組み立て時に渡した寿命の長い context で動かす。
func (s *Scans) StartScan(ctx context.Context) (domain.Scan, error) {
	scan, started, err := s.store.StartScan(ctx)
	if err != nil {
		return domain.Scan{}, err
	}
	if !started {
		return scan, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return scan, nil
	}
	s.running = true
	s.done.Add(1)

	// contextcheck はここで ctx を渡していないことを指摘するが、渡してはならない。
	// 上のコメントのとおり、要求の ctx を使うと応答を返した時点で走査が
	// 打ち切られる。走査は組み立て時に渡した寿命の長い context で動く。
	//nolint:contextcheck // 要求の ctx を走査へ持ち込まないのは意図した設計である。
	go s.run(scan.ID)

	s.scanChanged()
	return scan, nil
}

// CurrentScan は直近の走査を返す。
func (s *Scans) CurrentScan(ctx context.Context) (domain.Scan, error) {
	return s.store.CurrentScan(ctx)
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
// まま二度と取り込みを始められなくなる。running のまま残った仕事は queued へ
// 戻し、各段階のワーカーが続きから処理する。どちらも前回の停止で残った行だけを
// 対象にし、ライブラリ全体は読まない。
func (s *Scans) RecoverInterrupted(ctx context.Context) error {
	closed, err := s.store.FailInterruptedScans(ctx)
	if err != nil {
		return err
	}
	if closed > 0 {
		s.logger.Info("中断していた取り込みを閉じました", slog.Int64("count", closed))
	}

	restored, err := s.jobs.RequeueRunningJobs(ctx)
	if err != nil {
		return err
	}
	if restored > 0 {
		s.logger.Info("中断したジョブを待ち行列へ戻しました", slog.Int64("count", restored))
	}
	return nil
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

// run は走査を最後まで走らせ、結果を記録する。
func (s *Scans) run(scanID int64) {
	defer s.done.Done()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	ctx := s.baseCtx

	result, err := func() (result domain.ScanResult, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("取り込み処理がpanicしました: %v", recovered)
			}
		}()
		if s.scanner == nil {
			return domain.ScanResult{}, fmt.Errorf("走査が組み立てられていません")
		}
		return s.scanner.Scan(ctx)
	}()

	// 停止指示で打ち切った場合は、失敗として閉じる。次の起動で走り直せる。
	state := domain.ScanDone
	reason := ""
	if err != nil {
		state = domain.ScanFailed
		reason = err.Error()
		s.logger.Warn("取り込みが最後まで走りませんでした", slog.Any("error", err))
	} else {
		s.logger.Info("取り込みが終わりました",
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
		s.logger.Warn("取り込みの進捗を記録できませんでした", slog.Any("error", err))
	}

	if err := s.store.FinishScan(closeCtx, scanID, state, reason); err != nil {
		s.logger.Warn("取り込みの終了を記録できませんでした", slog.Any("error", err))
	}

	// 起動時やスキャンの後にライブラリ全体を見る後始末はしない。全体を読むのは
	// 利用者が取り込みを始めたときの走査だけである。
	s.scanChanged()
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
