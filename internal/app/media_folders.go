package app

import (
	"context"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Recheck scan activity and containment restrictions transactionally after FolderChecker validates
// filesystem paths.
type MediaFolderStore interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error)
	ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error)
	DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error
}

type FolderChecker interface {
	CheckMediaFolder(path string) (string, error)
}

type FolderVideoBytes interface {
	MediaFolderVideoBytes(ctx context.Context, dir string) (int64, error)
}

// DiskReader cannot be cancelled; a hung network volume blocks the call until the OS gives up.
type DiskReader interface {
	DiskUsage(path string) (domain.DiskUsage, error)
}

type FolderChangeHook interface {
	FoldersChanging(ctx context.Context)
	// Rearm after every attempted save, including failures.
	FoldersChanged(ctx context.Context)
}

type MediaFoldersOptions struct {
	Store   MediaFolderStore
	Checker FolderChecker
	// Nil disables change hooks.
	Changes    FolderChangeHook
	VideoBytes FolderVideoBytes
	Disk       DiskReader
}

// diskUsageTimeout bounds how long a usage request waits for an unresponsive volume.
const diskUsageTimeout = 5 * time.Second

// Validate filesystem paths before persistence.
type MediaFolders struct {
	store      MediaFolderStore
	checker    FolderChecker
	changes    FolderChangeHook
	videoBytes FolderVideoBytes
	disk       DiskReader
}

func NewMediaFolders(opts MediaFoldersOptions) *MediaFolders {
	return &MediaFolders{
		store: opts.Store, checker: opts.Checker, changes: opts.Changes,
		videoBytes: opts.VideoBytes, disk: opts.Disk,
	}
}

func (m *MediaFolders) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return m.store.ListMediaFolders(ctx)
}

// MediaFolderUsage leaves Disk nil when the volume fails or does not answer in time, so one
// unreachable folder never fails the request.
func (m *MediaFolders) MediaFolderUsage(ctx context.Context, id int64) (domain.MediaFolderUsage, error) {
	folders, err := m.store.ListMediaFolders(ctx)
	if err != nil {
		return domain.MediaFolderUsage{}, err
	}
	for _, folder := range folders {
		if folder.ID != id {
			continue
		}
		videoBytes, err := m.videoBytes.MediaFolderVideoBytes(ctx, folder.Path)
		if err != nil {
			return domain.MediaFolderUsage{}, err
		}
		return domain.MediaFolderUsage{VideoBytes: videoBytes, Disk: m.readDisk(ctx, folder.Path)}, nil
	}
	return domain.MediaFolderUsage{}, fmt.Errorf("media folder %d: %w", id, domain.ErrNotFound)
}

func (m *MediaFolders) readDisk(ctx context.Context, path string) *domain.DiskUsage {
	ctx, cancel := context.WithTimeout(ctx, diskUsageTimeout)
	defer cancel()
	// Buffered so a late answer does not block the abandoned goroutine.
	done := make(chan *domain.DiskUsage, 1)
	go func() {
		usage, err := m.disk.DiskUsage(path)
		if err != nil {
			done <- nil
			return
		}
		done <- &usage
	}()
	select {
	case usage := <-done:
		return usage
	case <-ctx.Done():
		return nil
	}
}

func (m *MediaFolders) AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error) {
	cleaned, err := m.checker.CheckMediaFolder(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer m.around(ctx)()
	return m.store.AddMediaFolder(ctx, cleaned)
}

func (m *MediaFolders) ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error) {
	cleaned, err := m.checker.CheckMediaFolder(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer m.around(ctx)()
	return m.store.ReplaceMediaFolder(ctx, id, expectedVersion, cleaned)
}

func (m *MediaFolders) DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error {
	defer m.around(ctx)()
	return m.store.DeleteMediaFolder(ctx, id, expectedVersion)
}

// Stop watcher scans before persistence, which rejects active-scan folder changes; the returned
// callback rearms afterward.
func (m *MediaFolders) around(ctx context.Context) func() {
	if m.changes == nil {
		return func() {}
	}
	m.changes.FoldersChanging(ctx)
	return func() { m.changes.FoldersChanged(ctx) }
}
