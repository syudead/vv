package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/jobs"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/scanner"
	"github.com/syudead/vv/internal/store"
)

// library は保存層・走査・ジョブを1つに束ねる。
//
// 誰が誰を呼ぶかの組み立ては cmd/mdm が行う（ARCHITECTURE.md）。
// internal/scanner と internal/jobs は保存の手段を知らず、interface 越しに
// ここで渡された *store.DB を使う。
type library struct {
	db      *store.DB
	scanner *scanner.Scanner
	logger  *slog.Logger

	// mu は走査の起動が重ならないようにする。実行中かどうかの判断は
	// scans 表（部分ユニーク索引）が持つので、ここは goroutine を
	// 二重に起こさないためだけの錠である。
	mu      sync.Mutex
	running bool
	baseCtx context.Context
}

// newLibrary は走査・ジョブの組み立てを行う。
func newLibrary(cfg Config, db *store.DB, logger *slog.Logger) *library {
	lib := &library{db: db, logger: logger}
	lib.scanner = scanner.New(scanner.Options{
		Index:    db,
		Queue:    db,
		Reporter: lib,
		Logger:   logger,
	})
	return lib
}

// newWorker は解析とサムネイル生成のハンドラを組み立ててワーカーを返す。
//
// ハンドラをここで組み立てるのは、internal/jobs から internal/media・
// internal/store を参照させないためである。jobs が持つのは「直列に取り出して
// 成否を記録する」進め方だけで、何をするかは cmd/mdm が決める。
func newWorker(cfg Config, db *store.DB, logger *slog.Logger) *jobs.Worker {
	return jobs.New(jobs.Options{
		Queue: db,
		Handlers: map[domain.JobKind]jobs.Handler{
			domain.JobProbe:     probeHandler(db),
			domain.JobThumbnail: thumbnailHandler(cfg, db),
		},
		Logger: logger,
	})
}

// probeHandler は ffprobe の結果を索引へ反映する。
//
// 再生可否の判定は internal/domain の純粋関数が行い、ここはその結果を
// 保存層へ渡すだけである（R-103）。判定が外部プロセスに依存しないので、
// 許可リストの規則は単体テストだけで検証できる。
func probeHandler(db *store.DB) jobs.Handler {
	return func(ctx context.Context, job domain.Job) error {
		current, err := db.JobIdentityCurrent(ctx, job)
		if err != nil {
			return err
		}
		if !current {
			return nil
		}
		if err := checkReadableRegularFile(job.LocationPath); err != nil {
			return err
		}

		probe, err := media.Probe(ctx, job.LocationPath)
		if err != nil {
			// 上限まで試して駄目なら、行は残したまま失敗として記録する。
			// 一覧からは消さない（FR-008）。
			if job.Attempts >= domain.MaxJobAttempts && job.LastLocation {
				if _, markErr := db.MarkProbeFailedForJob(ctx, job, err.Error()); markErr != nil {
					return markErr
				}
			}
			return err
		}

		playability := domain.EvaluatePlayability(domain.ContainerFromPath(job.LocationPath), probe)
		_, err = db.ApplyProbeForJob(ctx, job, probe, playability)
		return err
	}
}

// thumbnailHandler は静止画を1枚生成し、状態を記録する。
func thumbnailHandler(cfg Config, db *store.DB) jobs.Handler {
	return func(ctx context.Context, job domain.Job) error {
		video, err := db.GetVideo(ctx, job.VideoID)
		if err != nil {
			return err
		}

		var durationMs int64
		if video.DurationMs != nil {
			durationMs = *video.DurationMs
		}

		current, err := db.JobIdentityCurrent(ctx, job)
		if err != nil {
			return err
		}
		if !current {
			return nil
		}
		if err := checkReadableRegularFile(job.LocationPath); err != nil {
			return err
		}
		if _, err := media.Thumbnail(
			ctx, job.LocationPath, durationMs, cfg.ThumbnailsDir(), job.ContentKey,
		); err != nil {
			if job.Attempts >= domain.MaxJobAttempts && job.LastLocation {
				if _, markErr := db.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateFailed); markErr != nil {
					return markErr
				}
			}
			return err
		}

		_, err = db.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone)
		return err
	}
}

func checkReadableRegularFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("通常ファイルではありません: %s", path)
	}
	return nil
}

// StartScan は取り込みを開始する。実行中なら新しく始めず、実行中のものを返す
// （R-108）。応答は即座に返り、走査は背後で進む。
//
// ctx は要求のものなので、走査自体には使わない。要求が終わった時点で走査が
// 打ち切られてしまう。走査は起動時に渡した寿命の長い context で動かす。
func (l *library) StartScan(ctx context.Context) (domain.Scan, error) {
	scan, started, err := l.db.StartScan(ctx)
	if err != nil {
		return domain.Scan{}, err
	}
	if !started {
		return scan, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return scan, nil
	}
	l.running = true

	go l.runScan(scan.ID)

	return scan, nil
}

// CurrentScan は直近の走査を返す。
func (l *library) CurrentScan(ctx context.Context) (domain.Scan, error) {
	return l.db.CurrentScan(ctx)
}

// ReportScanProgress は走査の進捗を記録する。走査中も一覧・再生は通常どおり
// 応答する（FR-007）ので、ここでは行を1つ書き換えるだけにする。
func (l *library) ReportScanProgress(ctx context.Context, result domain.ScanResult) error {
	return l.db.UpdateScanProgress(ctx, l.currentScanID(ctx), domain.ScanProgress{
		Total:     result.Total,
		Completed: result.Completed(),
		Failed:    result.Failed,
	})
}

// currentScanID は進捗の書き込み先を返す。取れない場合は 0 を返し、
// 書き込みは何にも当たらない（走査は続ける）。
func (l *library) currentScanID(ctx context.Context) int64 {
	scan, err := l.db.CurrentScan(ctx)
	if err != nil {
		return 0
	}
	return scan.ID
}

// runScan は走査を最後まで走らせ、結果を記録する。
func (l *library) runScan(scanID int64) {
	defer func() {
		l.mu.Lock()
		l.running = false
		l.mu.Unlock()
	}()

	ctx := l.scanContext()

	result, err := func() (result domain.ScanResult, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("取り込み処理がpanicしました: %v", recovered)
			}
		}()
		return l.scanner.Scan(ctx)
	}()

	// 停止指示で打ち切った場合は、失敗として閉じる。次の起動で走り直せる。
	state := store.ScanDone
	reason := ""
	if err != nil {
		state = store.ScanFailed
		reason = err.Error()
		l.logger.Warn("取り込みが最後まで走りませんでした", slog.Any("error", err))
	} else {
		l.logger.Info("取り込みが終わりました",
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

	if err := l.db.UpdateScanProgress(closeCtx, scanID, domain.ScanProgress{
		Total:     result.Total,
		Completed: result.Completed(),
		Failed:    result.Failed,
	}); err != nil {
		l.logger.Warn("取り込みの進捗を記録できませんでした", slog.Any("error", err))
	}

	if err := l.db.FinishScan(closeCtx, scanID, state, reason); err != nil {
		l.logger.Warn("取り込みの終了を記録できませんでした", slog.Any("error", err))
	}

	// 走査のたびに掃除する。起動時だけだと、長く動かしているうちに完了行が
	// 積み上がる（取り込み直後は最大 2万行になる — data-model.md 4 節）。
	l.cleanFinishedJobs(closeCtx)
}

// cleanFinishedJobs は保存期間を過ぎた完了・失敗のジョブを消す。
// 後始末なので、失敗しても取り込みの成否には影響させない。
func (l *library) cleanFinishedJobs(ctx context.Context) {
	removed, err := l.db.DeleteFinishedJobsBefore(ctx, time.Now().Add(-store.JobRetention))
	if err != nil {
		l.logger.Warn("完了したジョブを掃除できませんでした", slog.Any("error", err))
		return
	}
	if removed > 0 {
		l.logger.Info("古いジョブを掃除しました", slog.Int64("count", removed))
	}
}

// scanContext は走査に使う context を返す。
func (l *library) scanContext() context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.baseCtx == nil {
		return context.Background()
	}
	return l.baseCtx
}

// bindContext は走査に使う寿命の長い context を結びつける。停止時にこれが
// 取り消され、走査中のジョブは queued に残る（次の起動で再開できる）。
func (l *library) bindContext(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.baseCtx = ctx
}

// recoverInterrupted は前回の停止で中途半端に残った状態を片付ける。
//
// running のまま残った走査を閉じないと、「実行中は1件だけ」の制約が働いた
// まま二度と取り込みを始められなくなる。ジョブの巻き戻しはワーカーが行う。
func (l *library) recoverInterrupted(ctx context.Context) error {
	closed, err := l.db.FailInterruptedScans(ctx)
	if err != nil {
		return err
	}
	if closed > 0 {
		l.logger.Info("中断していた取り込みを閉じました", slog.Int64("count", closed))
	}

	l.cleanFinishedJobs(ctx)
	return nil
}
