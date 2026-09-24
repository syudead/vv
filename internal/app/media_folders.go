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

// MediaFoldersOptions はメディアフォルダの操作に必要な依存である。
type MediaFoldersOptions struct {
	Store   MediaFolderStore
	Checker FolderChecker
}

// MediaFolders は設定画面のメディアフォルダの操作である。パスをファイル
// システムで確かめてから保存する。
type MediaFolders struct {
	store   MediaFolderStore
	checker FolderChecker
}

// NewMediaFolders はメディアフォルダの操作を返す。
func NewMediaFolders(opts MediaFoldersOptions) *MediaFolders {
	return &MediaFolders{store: opts.Store, checker: opts.Checker}
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
	return m.store.AddMediaFolder(ctx, cleaned)
}

// ReplaceMediaFolder は path を確かめてから、登録済みのフォルダの場所を
// 置き換える。
func (m *MediaFolders) ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error) {
	cleaned, err := m.checker.CheckMediaFolder(path)
	if err != nil {
		return domain.MediaFolder{}, err
	}
	return m.store.ReplaceMediaFolder(ctx, id, expectedVersion, cleaned)
}

// DeleteMediaFolder はメディアフォルダの登録を外す。
func (m *MediaFolders) DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error {
	return m.store.DeleteMediaFolder(ctx, id, expectedVersion)
}
