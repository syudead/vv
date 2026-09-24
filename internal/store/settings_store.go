package store

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

func (s *SettingsStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return s.db.ListMediaFolders(ctx)
}
func (s *SettingsStore) AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error) {
	return s.db.AddMediaFolder(ctx, path)
}
func (s *SettingsStore) ReplaceMediaFolder(
	ctx context.Context, id, expectedVersion int64, path string,
) (domain.MediaFolder, error) {
	return s.db.ReplaceMediaFolder(ctx, id, expectedVersion, path)
}
func (s *SettingsStore) DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error {
	return s.db.DeleteMediaFolder(ctx, id, expectedVersion)
}
