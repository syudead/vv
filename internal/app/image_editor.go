package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// ImageEditor hides provider execution behind a port; provider failures return
// *domain.ImageEditError.
type ImageEditor interface {
	Start(ctx context.Context, conn domain.ComfyUISettings, input domain.ImageEditInput) (domain.ImageEditJob, error)
	// Unknown provider states preserve previous; failed states include *domain.ImageEditError.
	State(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error)
	// Refresh provider state before choosing interrupt, queue deletion, or no-op; use previous only
	// for unknown states.
	Cancel(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) error
	// Progress is optional and never determines execution state.
	Progress(job domain.ImageEditJob) *domain.ImageEditProgress
	Output(ctx context.Context, job domain.ImageEditJob) ([]byte, error)
}
