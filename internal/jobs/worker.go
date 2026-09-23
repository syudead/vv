package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// defaultIdleInterval は待ち行列が空のときに次を見るまでの間隔である。
//
// 通知の仕組みを持たず単純に眠るのは、待ち行列が空である時間の方が圧倒的に
// 長く、取り込み直後の詰まった状態では ClaimJob が続けて返るためである。
// 取り込みを促してから最初のジョブが動き出すまでの遅れはこの値が上限になる。
const defaultIdleInterval = time.Second

// Queue は待ち行列である。jobs は保存の手段を知らない。
type Queue interface {
	// ClaimJob は1件を専有する。空なら domain.ErrNoJob を返す。
	ClaimJob(ctx context.Context) (domain.Job, error)
	// CompleteJob は完了を記録する。
	CompleteJob(ctx context.Context, id int64) error
	// FailJob は失敗を記録する。試行回数が上限に達していなければ待ち行列へ戻す。
	FailJob(ctx context.Context, id int64, reason string) error
	// RequeueRunningJobs は running のまま残っている行を queued へ戻す。
	RequeueRunningJobs(ctx context.Context) (int64, error)
}

// Handler は1種類のジョブの処理である。
//
// 実体（ffprobe の実行と保存層への反映）は cmd/mdm が組み立てて渡す。
// internal/jobs から internal/media・internal/store を参照しないのは、
// 依存の向きを一方向に保つためである（ARCHITECTURE.md）。ここに置くのは
// 「直列に取り出して、成否を記録し、止まったら戻す」という進め方だけである。
type Handler func(ctx context.Context, job domain.Job) error

type identityQueue interface {
	CompleteClaimedJob(context.Context, domain.Job) error
	FailClaimedJob(context.Context, domain.Job, string) error
}

// Options はワーカーの組み立てに必要な依存である。
type Options struct {
	Queue    Queue
	Handlers map[domain.JobKind]Handler
	// IdleInterval は待ち行列が空のときに次を見るまでの間隔。0 なら既定値。
	IdleInterval time.Duration
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Worker はプロセス内のジョブワーカーである。
//
// goroutine 1本で直列に処理する。並列度を上げないのは、初回スキャンで
// HDD の I/O が飽和し、全体としてはかえって遅くなるためである。並列度は設定に
// せず、必要になった時点で見直す。
type Worker struct {
	queue    Queue
	handlers map[domain.JobKind]Handler
	idle     time.Duration
	logger   *slog.Logger
}

// New はワーカーを組み立てる。
func New(opts Options) *Worker {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	idle := opts.IdleInterval
	if idle <= 0 {
		idle = defaultIdleInterval
	}

	handlers := map[domain.JobKind]Handler{}
	for kind, handler := range opts.Handlers {
		handlers[kind] = handler
	}

	return &Worker{queue: opts.Queue, handlers: handlers, idle: idle, logger: logger}
}

// Run は ctx が終わるまでジョブを処理し続ける。
//
// 起動時にまず running のまま残っている行を queued へ戻す。前回の停止で
// 処理中だったジョブがここで拾われるので、取り込みの途中で止めても次の
// 起動で再開でき、重複も生まない。
func (w *Worker) Run(ctx context.Context) {
	if restored, err := w.queue.RequeueRunningJobs(ctx); err != nil {
		w.logger.Warn("中断したジョブを戻せませんでした", slog.Any("error", err))
	} else if restored > 0 {
		w.logger.Info("中断したジョブを待ち行列へ戻しました", slog.Int64("count", restored))
	}

	for {
		if ctx.Err() != nil {
			return
		}

		job, err := w.queue.ClaimJob(ctx)
		switch {
		case errors.Is(err, domain.ErrNoJob):
			// 空なら少し待つ。ここで止めないと CPU を回し続ける。
			if !sleepContext(ctx, w.idle) {
				return
			}
			continue
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			w.logger.Warn("ジョブを取り出せませんでした", slog.Any("error", err))
			if !sleepContext(ctx, w.idle) {
				return
			}
			continue
		}

		w.process(ctx, job)
	}
}

// process は1件を処理し、結果を記録する。
//
// 停止指示を受けた場合は完了も失敗も記録しない。記録せずに抜ければ、行は
// running のまま残り、次の起動の巻き戻しで queued へ戻る。処理中のジョブを
// 失敗として数えると、再試行の回数を無駄に消費してしまう。
func (w *Worker) process(ctx context.Context, job domain.Job) {
	handler, ok := w.handlers[job.Kind]
	if !ok {
		// 再試行しても結果は変わらない。待ち行列を塞ぐだけなので諦める。
		w.fail(ctx, job, fmt.Errorf("扱いを知らない種類のジョブです: %s", job.Kind))
		return
	}

	if err := handler(ctx, job); err != nil {
		if ctx.Err() != nil {
			return
		}
		w.fail(ctx, job, err)
		return
	}
	if ctx.Err() != nil {
		return
	}

	var err error
	if queue, ok := w.queue.(identityQueue); ok {
		err = queue.CompleteClaimedJob(ctx, job)
	} else {
		err = w.queue.CompleteJob(ctx, job.ID)
	}
	if err != nil {
		w.logger.Warn("ジョブの完了を記録できませんでした",
			slog.Int64("job", job.ID), slog.Any("error", err))
	}
}

// fail は失敗を記録する。上限に達したかどうかの判断は待ち行列側が持つ。
func (w *Worker) fail(ctx context.Context, job domain.Job, cause error) {
	w.logger.Warn("ジョブに失敗しました",
		slog.Int64("job", job.ID),
		slog.String("kind", string(job.Kind)),
		slog.Int64("video", job.VideoID),
		slog.Int("attempts", job.Attempts),
		slog.Any("error", cause),
	)

	var err error
	if queue, ok := w.queue.(identityQueue); ok {
		err = queue.FailClaimedJob(ctx, job, cause.Error())
	} else {
		err = w.queue.FailJob(ctx, job.ID, cause.Error())
	}
	if err != nil {
		w.logger.Warn("ジョブの失敗を記録できませんでした",
			slog.Int64("job", job.ID), slog.Any("error", err))
	}
}

// sleepContext は待つ。取り消されたら false を返す。
func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
