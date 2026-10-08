package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// MediaFolderStore はメディアフォルダの保存先である。追加と置き換えは、
// 受け取ったパスが FolderChecker を通った
// ものとして保存し、走査中の変更と包含の禁止は取引の中で確かめ直す。
type MediaFolderStore interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error)
	ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error)
	DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error
}

// FolderChecker は、メディアフォルダとして登録するパスがファイルシステム上で
// 走査できるディレクトリかを確かめ、整えたパスを返す。internal/mediafs の
// FS がこれを満たす。
type FolderChecker interface {
	CheckMediaFolder(path string) (string, error)
}

// FolderChangeHook はメディアフォルダを変える操作の前後に呼ばれる。*AutoImport が満たす。
type FolderChangeHook interface {
	// FoldersChanging は保存の前に、走っている監視の走査を止める。
	FoldersChanging(ctx context.Context)
	// FoldersChanged は保存を試みたあとに、成否にかかわらず、監視を張り直す。
	FoldersChanged(ctx context.Context)
}

// MediaFoldersOptions はメディアフォルダの操作に必要な依存である。
type MediaFoldersOptions struct {
	Store   MediaFolderStore
	Checker FolderChecker
	// Changes は nil なら呼ばない。
	Changes FolderChangeHook
}

// MediaFolders は設定画面のメディアフォルダの操作である。パスをファイル
// システムで確かめてから保存する。
type MediaFolders struct {
	store   MediaFolderStore
	checker FolderChecker
	changes FolderChangeHook
}

// NewMediaFolders はメディアフォルダの操作を返す。
func NewMediaFolders(opts MediaFoldersOptions) *MediaFolders {
	return &MediaFolders{store: opts.Store, checker: opts.Checker, changes: opts.Changes}
}

// ListMediaFolders は登録済みのメディアフォルダを返す。
func (m *MediaFolders) ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error) {
	return m.store.ListMediaFolders(ctx)
}

// AddMediaFolder は path を確かめてからメディアフォルダとして登録する。
func (m *MediaFolders) AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error) {
	cleaned, err := m.checker.CheckMediaFolder(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer m.around(ctx)()
	return m.store.AddMediaFolder(ctx, cleaned)
}

// ReplaceMediaFolder は path を確かめてから、登録済みのフォルダの場所を
// 置き換える。
func (m *MediaFolders) ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error) {
	cleaned, err := m.checker.CheckMediaFolder(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	defer m.around(ctx)()
	return m.store.ReplaceMediaFolder(ctx, id, expectedVersion, cleaned)
}

// DeleteMediaFolder はメディアフォルダの登録を外す。
func (m *MediaFolders) DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error {
	defer m.around(ctx)()
	return m.store.DeleteMediaFolder(ctx, id, expectedVersion)
}

// around は変更の前に走っている監視の走査を止め、返した関数で変更のあとに監視を張り直す。
// 保存側は走査中の変更を断るので、止めるのは保存の前である（specs/042-folder-watch-import/research.md R-5）。
func (m *MediaFolders) around(ctx context.Context) func() {
	if m.changes == nil {
		return func() {}
	}
	m.changes.FoldersChanging(ctx)
	return func() { m.changes.FoldersChanged(ctx) }
}
