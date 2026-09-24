package store

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

func (s *ScanStore) StartScan(ctx context.Context) (domain.Scan, bool, error) {
	return s.db.StartScan(ctx)
}
func (s *ScanStore) UpdateScanProgress(ctx context.Context, id int64, progress domain.ScanProgress) error {
	return s.db.UpdateScanProgress(ctx, id, progress)
}
func (s *ScanStore) FinishScan(ctx context.Context, id int64, state domain.ScanState, reason string) error {
	return s.db.FinishScan(ctx, id, state, reason)
}
func (s *ScanStore) CurrentScan(ctx context.Context) (domain.Scan, error) {
	return s.db.CurrentScan(ctx)
}
func (s *ScanStore) FailInterruptedScans(ctx context.Context) (int64, error) {
	return s.db.FailInterruptedScans(ctx)
}
func (s *ScanIndexStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return s.db.ListMediaFolders(ctx)
}
func (s *ScanIndexStore) IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error) {
	return s.db.IndexedVideosByPath(ctx)
}
func (s *ScanIndexStore) UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error) {
	return s.db.UpsertVideo(ctx, file)
}
func (s *ScanIndexStore) DeleteVideoLocations(ctx context.Context, ids []int64) error {
	return s.db.DeleteVideoLocations(ctx, ids)
}
