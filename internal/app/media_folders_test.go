package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeFolderChecker struct {
	cleaned string
	err     error
	checked []string
}

func (f *fakeFolderChecker) CheckMediaFolder(path string) (string, error) {
	f.checked = append(f.checked, path)
	return f.cleaned, f.err
}

type fakeMediaFolderStore struct {
	added    []string
	replaced []string
	deleted  []int64
}

func (f *fakeMediaFolderStore) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return nil, nil
}

func (f *fakeMediaFolderStore) AddMediaFolder(_ context.Context, path string) (domain.MediaFolder, error) {
	f.added = append(f.added, path)
	return domain.MediaFolder{ID: 1, Path: path, Version: 1}, nil
}

func (f *fakeMediaFolderStore) ReplaceMediaFolder(_ context.Context, id, _ int64, path string) (domain.MediaFolder, error) {
	f.replaced = append(f.replaced, path)
	return domain.MediaFolder{ID: id, Path: path, Version: 2}, nil
}

func (f *fakeMediaFolderStore) DeleteMediaFolder(_ context.Context, id, _ int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestMediaFoldersSaveTheCheckedPath(t *testing.T) {
	store := &fakeMediaFolderStore{}
	checker := &fakeFolderChecker{cleaned: "/media/movies"}
	folders := NewMediaFolders(MediaFoldersOptions{Store: store, Checker: checker})

	if _, err := folders.AddMediaFolder(context.Background(), "/media/movies/"); err != nil {
		t.Fatal(err)
	}
	if _, err := folders.ReplaceMediaFolder(context.Background(), 1, 1, "/media/./movies"); err != nil {
		t.Fatal(err)
	}
	if len(checker.checked) != 2 || checker.checked[0] != "/media/movies/" || checker.checked[1] != "/media/./movies" {
		t.Fatalf("checked = %q", checker.checked)
	}
	if len(store.added) != 1 || store.added[0] != "/media/movies" || len(store.replaced) != 1 || store.replaced[0] != "/media/movies" {
		t.Fatalf("saved add=%q replace=%q, want the checked path", store.added, store.replaced)
	}
}

func TestMediaFoldersDoNotSaveRejectedPath(t *testing.T) {
	store := &fakeMediaFolderStore{}
	checker := &fakeFolderChecker{err: domain.ErrUnsupportedMediaFolder}
	folders := NewMediaFolders(MediaFoldersOptions{Store: store, Checker: checker})

	if _, err := folders.AddMediaFolder(context.Background(), "/link"); !errors.Is(err, domain.ErrUnsupportedMediaFolder) {
		t.Fatalf("add error = %v, want ErrUnsupportedMediaFolder", err)
	}
	if _, err := folders.ReplaceMediaFolder(context.Background(), 1, 1, "/link"); !errors.Is(err, domain.ErrUnsupportedMediaFolder) {
		t.Fatalf("replace error = %v, want ErrUnsupportedMediaFolder", err)
	}
	if len(store.added) != 0 || len(store.replaced) != 0 {
		t.Fatalf("rejected path was saved: add=%q replace=%q", store.added, store.replaced)
	}
}
