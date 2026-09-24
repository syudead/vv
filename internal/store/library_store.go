package store

import (
	"context"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func (s *LibraryStore) ListVideos(ctx context.Context, q domain.VideoQuery) (domain.VideoPage, error) {
	return s.db.ListVideos(ctx, q)
}
func (s *LibraryStore) GetVideo(ctx context.Context, id int64) (domain.Video, error) {
	return s.db.GetVideo(ctx, id)
}
func (s *LibraryStore) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return s.db.ListMediaFolders(ctx)
}
func (s *LibraryStore) FolderLocations(ctx context.Context, dir string) ([]domain.FolderLocation, error) {
	return s.db.FolderLocations(ctx, dir)
}
func (s *LibraryStore) HasFolderLocations(ctx context.Context, dir string) (bool, error) {
	return s.db.HasFolderLocations(ctx, dir)
}
func (s *LibraryStore) ListFolderVideos(ctx context.Context, q domain.FolderVideoQuery) (domain.VideoPage, error) {
	return s.db.ListFolderVideos(ctx, q)
}
func (s *LibraryStore) DirectVideoPaths(ctx context.Context, dir string) ([]domain.RelatedSibling, error) {
	return s.db.DirectVideoPaths(ctx, dir)
}
func (s *LibraryStore) VideosAddedNear(
	ctx context.Context, id int64, addedAt time.Time, limit int,
) ([]domain.RelatedNeighbor, error) {
	return s.db.VideosAddedNear(ctx, id, addedAt, limit)
}
func (s *LibraryStore) VideosByIDs(ctx context.Context, ids []int64) ([]domain.Video, error) {
	return s.db.VideosByIDs(ctx, ids)
}
func (s *LibraryStore) ContentKeyReferenced(ctx context.Context, key string) (bool, error) {
	return s.db.ContentKeyReferenced(ctx, key)
}
func (s *LibraryStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	return s.db.RefreshSearchKeys(ctx)
}
