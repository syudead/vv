package app

import (
	"context"
	"log/slog"

	"github.com/syudead/vv/internal/domain"
)

// Waker asks the loop that runs a queue worker's Step for another step; cmd/mdm runs every Step on a jobs.Loop and
// passes its Wake. A nil Waker drops wakeups.
type Waker func()

func (w Waker) Wake() {
	if w != nil {
		w()
	}
}

// retryStep logs a failed step unless ctx has ended, when the failure is the shutdown itself, and asks for a retry.
func retryStep(ctx context.Context, logger *slog.Logger, message string, err error) domain.WorkerWait {
	if ctx.Err() == nil {
		logger.Error(message, slog.Any("error", err))
	}
	return domain.WorkerRetry
}
