package app

import (
	"context"

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

type FolderChangeHook interface {
	FoldersChanging(ctx context.Context)
	// Rearm after every attempted save, including failures.
	FoldersChanged(ctx context.Context)
}

type MediaFoldersOptions struct {
	Store   MediaFolderStore
	Checker FolderChecker
	// Nil disables change hooks.
	Changes FolderChangeHook
}

// Validate filesystem paths before persistence.
type MediaFolders struct {
	store   MediaFolderStore
	checker FolderChecker
	changes FolderChangeHook
}

func NewMediaFolders(opts MediaFoldersOptions) *MediaFolders {
	return &MediaFolders{store: opts.Store, checker: opts.Checker, changes: opts.Changes}
}

func (m *MediaFolders) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return m.store.ListMediaFolders(ctx)
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
