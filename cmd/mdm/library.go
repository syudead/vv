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
	db            *store.DB
	scanner       *scanner.Scanner
	logger        *slog.Logger
	thumbnailsDir string

	// mu は走査の起動が重ならないようにする。実行中かどうかの判断は
	// scans 表（部分ユニーク索引）が持つので、ここは goroutine を
	// 二重に起こさないためだけの錠である。
	mu      sync.Mutex
	running bool
	baseCtx context.Context
}

// newLibrary は走査・ジョブの組み立てを行う。
func newLibrary(cfg Config, db *store.DB, logger *slog.Logger) *library {
	lib := &library{db: db, logger: logger, thumbnailsDir: cfg.ThumbnailsDir()}
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
			domain.JobPreview:   previewHandler(cfg, db),
		},
		Logger: logger,
	})
}

// probeHandler は ffprobe の結果を索引へ反映する。
//
// 再生可否の判定は internal/domain の純粋関数が行い、ここはその結果を
// 保存層へ渡すだけである。判定が外部プロセスに依存しないので、
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
			// 一覧からは消さない。
			if job.Attempts >= domain.MaxJobAttempts && job.LastLocation {
				if _, markErr := db.MarkProbeFailedForJob(ctx, job, err.Error()); markErr != nil {
					return markErr
				}
			}
			return err
		}

		playability := domain.EvaluatePlayability(domain.ContainerFromPath(job.LocationPath), probe)
		applied, err := db.ApplyProbeForJob(ctx, job, probe, playability)
		if err != nil || !applied {
			return err
		}
		return db.EnqueueJob(ctx, domain.JobPreview, job.VideoID)
	}
}

func previewHandler(cfg Config, db *store.DB) jobs.Handler {
	return func(ctx context.Context, job domain.Job) error {
		video, err := db.GetVideo(ctx, job.VideoID)
		if err != nil {
			return err
		}
		if video.ProbeState != domain.ProbeStateDone {
			return nil
		}
		if video.DurationMs == nil || *video.DurationMs <= 0 {
			return fmt.Errorf("プレビュー生成に必要な動画の長さがありません")
		}
		current, err := db.ContentKeyCurrent(ctx, job.VideoID, job.ContentKey)
		if err != nil || !current {
			return err
		}
		if err := checkReadableRegularFile(job.LocationPath); err != nil {
			return err
		}
		validateContent := func(validateCtx context.Context) (bool, error) {
			return db.PreviewSourceCurrent(validateCtx, job)
		}
		if err := media.GeneratePreview(ctx, job.LocationPath, cfg.ThumbnailsDir(), job.ContentKey, *video.DurationMs, validateContent); err != nil {
			return err
		}
		applied, err := db.CompletePreviewForContent(context.WithoutCancel(ctx), job)
		if err != nil {
			return err
		}
		if !applied {
			return media.ErrPreviewStale
		}
		return nil
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
		hadThumbnail := video.ThumbnailState == domain.ThumbnailStateDone
		if !hadThumbnail {
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
			applied, err := db.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone)
			if err != nil || !applied {
				return err
			}
		}
		if err := media.GenerateSeekThumbnails(
			ctx, job.LocationPath, cfg.ThumbnailsDir(), job.ContentKey,
		); err != nil {
			return err
		}

		// A scan can delete the video while ffmpeg is still generating files.
		// Recheck after the atomic rename so a cache committed after scan cleanup
		// cannot remain orphaned indefinitely.
		keys, err := db.ContentKeys(context.WithoutCancel(ctx))
		if err != nil {
			return err
		}
		if _, referenced := keys[job.ContentKey]; !referenced {
			return media.RemoveSeekThumbnails(cfg.ThumbnailsDir(), job.ContentKey)
		}
		return nil
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

// StartScan は取り込みを開始する。実行中なら新しく始めず、実行中のものを返す。
// 応答は即座に返り、走査は背後で進む。
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

	// contextcheck はここで ctx を渡していないことを指摘するが、渡してはならない。
	// 上のコメントのとおり、要求の ctx を使うと応答を返した時点で走査が
	// 打ち切られる。走査は起動時に渡した寿命の長い context で動く。
	//nolint:contextcheck // 要求の ctx を走査へ持ち込まないのは意図した設計である。
	go l.runScan(scan.ID)

	return scan, nil
}

// CurrentScan は直近の走査を返す。
func (l *library) CurrentScan(ctx context.Context) (domain.Scan, error) {
	return l.db.CurrentScan(ctx)
}

// ReportScanProgress は走査の進捗を記録する。走査中も一覧・再生は通常どおり
// 応答するので、ここでは行を1つ書き換えるだけにする。
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
	// 積み上がる（取り込み直後は最大 2万行になる）。
	l.reconcilePreviews(closeCtx)
	l.cleanFinishedJobs(closeCtx)
	l.cleanSeekThumbnails(closeCtx)
	l.cleanPreviews(closeCtx)
	l.cleanPreviewTemps(media.PreviewTempCutoff(time.Now()))
}

func (l *library) cleanSeekThumbnails(ctx context.Context) {
	keys, err := l.db.ContentKeys(ctx)
	if err != nil {
		l.logger.Warn("シークサムネイルの参照を読み出せませんでした", slog.Any("error", err))
		return
	}
	removed, err := media.RemoveOrphanSeekThumbnails(l.thumbnailsDir, keys)
	if err != nil {
		l.logger.Warn("孤児シークサムネイルを掃除できませんでした", slog.Any("error", err))
		return
	}
	if removed > 0 {
		l.logger.Info("孤児シークサムネイルを掃除しました", slog.Int("count", removed))
	}
}

func (l *library) cleanPreviews(ctx context.Context) {
	keys, err := l.db.ContentKeys(ctx)
	if err != nil {
		l.logger.Warn("プレビューの参照を読み出せませんでした", slog.Any("error", err))
		return
	}
	removed, err := media.RemoveOrphanPreviews(l.thumbnailsDir, keys)
	if err != nil {
		l.logger.Warn("孤児プレビューを掃除できませんでした", slog.Any("error", err))
		return
	}
	if removed > 0 {
		l.logger.Info("孤児プレビューを掃除しました", slog.Int("count", removed))
	}
}

func (l *library) cleanPreviewTemps(cutoff time.Time) {
	removed, err := media.RemoveAbandonedPreviewTemps(l.thumbnailsDir, cutoff)
	if err != nil {
		l.logger.Warn("中断したプレビュー生成物を掃除できませんでした", slog.Any("error", err))
		return
	}
	if removed > 0 {
		l.logger.Info("中断したプレビュー生成物を掃除しました", slog.Int("count", removed))
	}
}

func (l *library) reconcilePreviews(ctx context.Context) {
	marked, requeued, err := l.db.ReconcilePreviewFailures(ctx)
	if err != nil {
		l.logger.Warn("プレビュー失敗状態を整合できませんでした", slog.Any("error", err))
		return
	}
	if marked > 0 || requeued > 0 {
		l.logger.Info("プレビュー失敗状態を整合しました", slog.Int64("failed", marked), slog.Int64("requeued", requeued))
	}
	assets, err := l.db.PreviewAssets(ctx)
	if err != nil {
		l.logger.Warn("プレビューの状態を読み出せませんでした", slog.Any("error", err))
		return
	}
	for _, asset := range assets {
		if asset.State != domain.PreviewStateDone {
			continue
		}
		path := media.PreviewPath(l.thumbnailsDir, asset.ContentKey)
		manifest := media.PreviewManifestPath(l.thumbnailsDir, asset.ContentKey)
		if _, err := media.VerifyPreview(path, manifest); err == nil {
			continue
		}
		if err := media.RemovePreview(l.thumbnailsDir, asset.ContentKey); err != nil {
			l.logger.Warn("壊れたプレビューを削除できませんでした", slog.Any("error", err))
		}
		if err := l.db.RequeuePreviewRepair(ctx, asset.ID); err != nil {
			l.logger.Warn("プレビューの修復ジョブを積めませんでした", slog.Any("error", err))
		}
	}
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
	l.cleanPreviewTemps(time.Now())
	return nil
}
