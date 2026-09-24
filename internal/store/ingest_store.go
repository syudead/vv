package store

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

func (s *IngestStore) EnqueueJob(ctx context.Context, kind domain.JobKind, videoID int64) error {
	return s.db.EnqueueJob(ctx, kind, videoID)
}
func (s *IngestStore) EnsureJob(ctx context.Context, kind domain.JobKind, videoID int64) error {
	return s.db.EnsureJob(ctx, kind, videoID)
}
func (s *IngestStore) ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error) {
	return s.db.ClaimJob(ctx, kind)
}
func (s *IngestStore) CompleteClaimedJob(ctx context.Context, job domain.Job) error {
	return s.db.CompleteClaimedJob(ctx, job)
}
func (s *IngestStore) FailClaimedJob(ctx context.Context, job domain.Job, reason string) error {
	return s.db.FailClaimedJob(ctx, job, reason)
}
func (s *IngestStore) JobIdentityCurrent(ctx context.Context, job domain.Job) (bool, error) {
	return s.db.JobIdentityCurrent(ctx, job)
}
func (s *IngestStore) RequeueRunningJobs(ctx context.Context) (int64, error) {
	return s.db.RequeueRunningJobs(ctx)
}
func (s *IngestStore) ThumbnailJobActive(ctx context.Context, videoID int64) (bool, error) {
	return s.db.ThumbnailJobActive(ctx, videoID)
}
func (s *IngestStore) Processing(ctx context.Context) (domain.Processing, error) {
	return s.db.Processing(ctx)
}
func (s *IngestStore) ApplyProbeForJob(
	ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability,
) (bool, error) {
	return s.db.ApplyProbeForJob(ctx, job, probe, play)
}
func (s *IngestStore) SetThumbnailStateForJob(
	ctx context.Context, job domain.Job, state domain.ThumbnailState,
) (bool, error) {
	return s.db.SetThumbnailStateForJob(ctx, job, state)
}
func (s *IngestStore) CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error) {
	return s.db.CompletePreviewForContent(ctx, job)
}
func (s *IngestStore) ContentKeyCurrent(ctx context.Context, videoID int64, key string) (bool, error) {
	return s.db.ContentKeyCurrent(ctx, videoID, key)
}
func (s *IngestStore) PreviewSourceCurrent(ctx context.Context, job domain.Job) (bool, error) {
	return s.db.PreviewSourceCurrent(ctx, job)
}
func (s *IngestStore) RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error) {
	return s.db.RequeueMissingPreview(ctx, id, contentKey)
}
func (s *IngestStore) RetryProbe(ctx context.Context, id int64, seekThumbnailMissing bool) error {
	return s.db.RetryProbe(ctx, id, seekThumbnailMissing)
}
func (s *IngestStore) GetVideo(ctx context.Context, id int64) (domain.Video, error) {
	return s.db.GetVideo(ctx, id)
}
func (s *IngestStore) ContentKeyReferenced(ctx context.Context, key string) (bool, error) {
	return s.db.ContentKeyReferenced(ctx, key)
}
