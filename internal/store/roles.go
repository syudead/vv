package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// IngestStore handles the job queue and writes produced metadata back to the
// rebuildable library index.
type IngestStore struct{ db *DB }

// LibraryStore serves read-side library queries and reference checks.
type LibraryStore struct{ db *DB }

// ScanStore stores scan lifecycle state.
type ScanStore struct{ db *DB }

// ScanIndexStore applies filesystem scan results to the rebuildable index.
type ScanIndexStore struct{ db *DB }

// SettingsStore stores user configuration such as media folders.
type SettingsStore struct{ db *DB }

// PlaybackStore stores user playback progress. It depends only on the shared
// SQLite handle, not on library-index stores or their notifications.
type PlaybackStore struct{ sql *sql.DB }

// Ingest returns the ingest-facing store.
func (db *DB) Ingest() *IngestStore { return &IngestStore{db: db} }

// Library returns the library-index read store.
func (db *DB) Library() *LibraryStore { return &LibraryStore{db: db} }

// Scans returns the scan-state store.
func (db *DB) Scans() *ScanStore { return &ScanStore{db: db} }

// ScanIndex returns the scan-facing library-index store.
func (db *DB) ScanIndex() *ScanIndexStore { return &ScanIndexStore{db: db} }

// Settings returns the configuration store.
func (db *DB) Settings() *SettingsStore { return &SettingsStore{db: db} }

// Playback returns the user-data store.
func (db *DB) Playback() *PlaybackStore { return &PlaybackStore{sql: db.sql} }

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

func (s *LibraryStore) RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error) {
	return s.db.RequeueMissingPreview(ctx, id, contentKey)
}

func (s *LibraryStore) ThumbnailJobActive(ctx context.Context, videoID int64) (bool, error) {
	return s.db.ThumbnailJobActive(ctx, videoID)
}

func (s *LibraryStore) RetryProbe(ctx context.Context, id int64, seekThumbnailMissing bool) error {
	return s.db.RetryProbe(ctx, id, seekThumbnailMissing)
}

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

func (s *ScanStore) RequeueRunningJobs(ctx context.Context) (int64, error) {
	return s.db.RequeueRunningJobs(ctx)
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
