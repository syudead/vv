package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// defaultRetryDelay は、待ち行列からの取り出し自体が失敗したときに、次に
// 試すまでの間隔である。仕事の有無を見に行く間隔ではない。仕事が無いときは
// 通知（Wake）が来るまで眠る。
const defaultRetryDelay = 5 * time.Second

// Queue は待ち行列である。jobs は保存の手段を知らない。
type Queue interface {
	// ClaimJob は kind の仕事を1件専有する。空なら domain.ErrNoJob を返す。
	ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error)
	// CompleteClaimedJob は専有した時点の所在が今も同じなら完了を記録する。
	CompleteClaimedJob(ctx context.Context, job domain.Job) error
	// FailClaimedJob は失敗を記録する。試行回数が上限に達していなければ
	// 待ち行列へ戻す。
	FailClaimedJob(ctx context.Context, job domain.Job, reason string) error
}

// Handler は1種類のジョブの処理である。
//
// 実体（ffprobe の実行と保存層への反映）は internal/app が持ち、cmd/mdm が
// 渡す。internal/jobs から internal/app・internal/media・internal/store を参照
// しないのは、依存の向きを一方向に保つためである（ARCHITECTURE.md）。ここに置くのは
// 「取り出して、成否を記録し、止まったら戻す」という進め方だけである。
type Handler func(ctx context.Context, job domain.Job) error

// Options はワーカーの組み立てに必要な依存である。
type Options struct {
	// Kind はこのワーカーが受け持つ段階である。
	Kind    domain.JobKind
	Queue   Queue
	Handler Handler
	// Finished は1件の成否を記録したあとに呼ばれる。nil でもよい。
	Finished func(job domain.Job)
	// RetryDelay は取り出しが失敗したときの再試行の間隔。0 なら既定値。
	RetryDelay time.Duration
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Worker は取り込みの1段階を受け持つプロセス内のワーカーである。
//
// 自分の種類の仕事だけを1件ずつ処理する。並列度を上げないのは、初回スキャンで
// HDD の I/O が飽和し、全体としてはかえって遅くなるためである。
//
// 待ち行列が空になったら、Wake が呼ばれるまで何もしない。仕事が積まれたことの
// 購読（cmd/mdm）が Wake を呼ぶので、一定間隔で待ち行列を問い合わせる必要が無い。
type Worker struct {
	kind     domain.JobKind
	queue    Queue
	handler  Handler
	finished func(domain.Job)
	retry    time.Duration
	logger   *slog.Logger
	wake     chan struct{}
}

// New はワーカーを組み立てる。
func New(opts Options) *Worker {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	retry := opts.RetryDelay
	if retry <= 0 {
		retry = defaultRetryDelay
	}
	return &Worker{
		kind:     opts.Kind,
		queue:    opts.Queue,
		handler:  opts.Handler,
		finished: opts.Finished,
		retry:    retry,
		logger:   logger.With(slog.String("stage", string(opts.Kind))),
		// 容量1で、起こす知らせを1つだけ貯める。眠る直前に届いた知らせも
		// 失われず、次の待機がすぐに明ける。
		wake: make(chan struct{}, 1),
	}
}

// Kind はこのワーカーが受け持つ段階を返す。
func (w *Worker) Kind() domain.JobKind {
	return w.kind
}

// Wake は仕事が積まれたことを知らせる。ブロックしない。
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run は ctx が終わるまで仕事を処理し続ける。
//
// 起動直後は、知らせを待たずに1度待ち行列を見る。前回の停止で残った仕事を
// 拾うためである。
func (w *Worker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		job, err := w.queue.ClaimJob(ctx, w.kind)
		switch {
		case errors.Is(err, domain.ErrNoJob):
			if !w.sleep(ctx, nil) {
				return
			}
			continue
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			w.logger.Warn("ジョブを取り出せませんでした", slog.Any("error", err))
			timer := time.NewTimer(w.retry)
			ok := w.sleep(ctx, timer.C)
			timer.Stop()
			if !ok {
				return
			}
			continue
		}

		w.process(ctx, job)
	}
}

// sleep は知らせ（または retry が明けるの）を待つ。取り消されたら false を返す。
func (w *Worker) sleep(ctx context.Context, retry <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-w.wake:
		return true
	case <-retry:
		return true
	}
}

// process は1件を処理し、結果を記録する。
//
// 停止指示を受けた場合は完了も失敗も記録しない。記録せずに抜ければ、行は
// running のまま残り、次の起動の巻き戻しで queued へ戻る。処理中のジョブを
// 失敗として数えると、再試行の回数を無駄に消費してしまう。
func (w *Worker) process(ctx context.Context, job domain.Job) {
	var err error
	if w.handler == nil {
		// 再試行しても結果は変わらない。待ち行列を塞ぐだけなので諦める。
		err = fmt.Errorf("扱いを知らない種類のジョブです: %s", job.Kind)
	} else {
		err = w.handler(ctx, job)
	}
	if ctx.Err() != nil {
		return
	}

	if err != nil {
		w.fail(ctx, job, err)
	} else if completeErr := w.queue.CompleteClaimedJob(ctx, job); completeErr != nil {
		w.logger.Warn("ジョブの完了を記録できませんでした",
			slog.Int64("job", job.ID), slog.Any("error", completeErr))
	}
	if w.finished != nil {
		w.finished(job)
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

	if err := w.queue.FailClaimedJob(ctx, job, cause.Error()); err != nil {
		w.logger.Warn("ジョブの失敗を記録できませんでした",
			slog.Int64("job", job.ID), slog.Any("error", err))
	}
}
