package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
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
	// events は画面へ送る変化の知らせ。nil なら知らせない。
	events *httpapi.Events

	// mu は走査の起動が重ならないようにする。実行中かどうかの判断は
	// scans 表（部分ユニーク索引）が持つので、ここは goroutine を
	// 二重に起こさないためだけの錠である。
	mu      sync.Mutex
	running bool
	baseCtx context.Context
}

// newLibrary は走査の組み立てを行う。
func newLibrary(db *store.DB, logger *slog.Logger, events *httpapi.Events) *library {
	lib := &library{db: db, logger: logger, events: events}
	lib.scanner = scanner.New(scanner.Options{
		Index:    db,
		Queue:    db,
		Reporter: lib,
		Logger:   logger,
	})
	return lib
}

// newWorkers は取り込みの段階ごとにワーカーを1本ずつ組み立てる。
//
// ハンドラをここで組み立てるのは、internal/jobs から internal/media・
// internal/store を参照させないためである。jobs が持つのは「取り出して
// 成否を記録する」進め方だけで、何をするかは cmd/mdm が決める。
//
// 1件の成否を記録するたびに、その動画と段階ごとの残りが変わったことを画面へ
// 知らせる。
func newWorkers(cfg Config, db *store.DB, logger *slog.Logger, events *httpapi.Events) []*jobs.Worker {
	handlers := map[domain.JobKind]jobs.Handler{
		domain.JobProbe:     probeHandler(db),
		domain.JobThumbnail: thumbnailHandler(cfg, db),
		domain.JobPreview:   previewHandler(cfg, db),
	}
	finished := func(job domain.Job) {
		if events == nil {
			return
		}
		events.VideoChanged(job.VideoID)
		events.ProcessingChanged()
	}
	workers := make([]*jobs.Worker, 0, len(domain.JobKinds))
	for _, kind := range domain.JobKinds {
		workers = append(workers, jobs.New(jobs.Options{
			Kind:     kind,
			Queue:    db,
			Handler:  handlers[kind],
			Finished: finished,
			Logger:   logger,
		}))
	}
	return workers
}

// wakeWorkers は仕事が積まれた段階のワーカーを起こし、残りが変わったことを
// 画面へ知らせる。保存層の OnJobsQueued に渡す。
func wakeWorkers(workers []*jobs.Worker, events *httpapi.Events) func(kinds []domain.JobKind) {
	byKind := make(map[domain.JobKind]*jobs.Worker, len(workers))
	for _, worker := range workers {
		byKind[worker.Kind()] = worker
	}
	return func(kinds []domain.JobKind) {
		for _, kind := range kinds {
			if worker, ok := byKind[kind]; ok {
				worker.Wake()
			}
		}
		if events != nil {
			events.ProcessingChanged()
		}
	}
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

		// 上限まで試して駄目なときの失敗は、ジョブを failed にするのと同じ取引で
		// FailClaimedJob が動画側へ記録する。行は残したままで、一覧からは消さない。
		probe, err := media.Probe(ctx, job.LocationPath)
		if err != nil {
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
		// 上限まで試して駄目なときの失敗は、ジョブを failed にするのと同じ取引で
		// FailClaimedJob が動画側へ記録する。
		hadThumbnail := video.ThumbnailState == domain.ThumbnailStateDone
		if !hadThumbnail {
			if _, err := media.Thumbnail(
				ctx, job.LocationPath, durationMs, cfg.ThumbnailsDir(), job.ContentKey,
			); err != nil {
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

		// 生成中にスキャンが動画を消すことがある。書き終えたあとで確かめ直し、
		// 参照の無くなった生成物を残さない。
		referenced, err := db.ContentKeyReferenced(context.WithoutCancel(ctx), job.ContentKey)
		if err != nil {
			return err
		}
		if !referenced {
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

	l.scanChanged()
	return scan, nil
}

// scanChanged はスキャンの状態が変わったことを画面へ知らせる。走査は仕事を
// 積みながら進むので、段階ごとの残りも合わせて知らせる。
func (l *library) scanChanged() {
	if l.events == nil {
		return
	}
	l.events.ScanChanged()
	l.events.ProcessingChanged()
}

// CurrentScan は直近の走査を返す。
func (l *library) CurrentScan(ctx context.Context) (domain.Scan, error) {
	return l.db.CurrentScan(ctx)
}

// ReportScanProgress は走査の進捗を記録する。走査中も一覧・再生は通常どおり
// 応答するので、ここでは行を1つ書き換えるだけにする。
func (l *library) ReportScanProgress(ctx context.Context, result domain.ScanResult) error {
	if err := l.db.UpdateScanProgress(ctx, l.currentScanID(ctx), domain.ScanProgress{
		Total:     result.Total,
		Completed: result.Completed(),
		Failed:    result.Failed,
	}); err != nil {
		return err
	}
	l.scanChanged()
	return nil
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

	// 起動時やスキャンの後にライブラリ全体を見る後始末はしない。全体を読むのは
	// 利用者が取り込みを始めたときの走査だけである。
	l.scanChanged()
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

// recoverInterrupted は前回の停止で中途半端に残った状態を戻す。
//
// running のまま残った走査を閉じないと、「実行中は1件だけ」の制約が働いた
// まま二度と取り込みを始められなくなる。running のまま残った仕事は queued へ
// 戻し、各段階のワーカーが続きから処理する。どちらも前回の停止で残った行だけを
// 対象にし、ライブラリ全体は読まない。
func (l *library) recoverInterrupted(ctx context.Context) error {
	closed, err := l.db.FailInterruptedScans(ctx)
	if err != nil {
		return err
	}
	if closed > 0 {
		l.logger.Info("中断していた取り込みを閉じました", slog.Int64("count", closed))
	}

	restored, err := l.db.RequeueRunningJobs(ctx)
	if err != nil {
		return err
	}
	if restored > 0 {
		l.logger.Info("中断したジョブを待ち行列へ戻しました", slog.Int64("count", restored))
	}
	return nil
}
