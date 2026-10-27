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
	folders  []domain.MediaFolder
	added    []string
	replaced []string
	deleted  []int64
}

func (f *fakeMediaFolderStore) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return f.folders, nil
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

type fakeVideoBytes struct{ dirs []string }

func (f *fakeVideoBytes) MediaFolderVideoBytes(_ context.Context, dir string) (int64, error) {
	f.dirs = append(f.dirs, dir)
	return 42, nil
}

type fakeDisk struct {
	usage domain.DiskUsage
	err   error
	// A non-nil block holds the call until closed, like an unresponsive network volume.
	block chan struct{}
	paths []string
}

func (f *fakeDisk) DiskUsage(path string) (domain.DiskUsage, error) {
	f.paths = append(f.paths, path)
	if f.block != nil {
		<-f.block
	}
	return f.usage, f.err
}

func usageFolders(disk *fakeDisk, sizes *fakeVideoBytes) *MediaFolders {
	store := &fakeMediaFolderStore{folders: []domain.MediaFolder{{ID: 1, Path: "/a"}, {ID: 2, Path: "/b"}}}
	return NewMediaFolders(MediaFoldersOptions{Store: store, VideoBytes: sizes, Disk: disk})
}

func TestMediaFolderUsageReadsTheFolderPath(t *testing.T) {
	disk := &fakeDisk{usage: domain.DiskUsage{FreeBytes: 5, TotalBytes: 9}}
	sizes := &fakeVideoBytes{}
	got, err := usageFolders(disk, sizes).MediaFolderUsage(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoBytes != 42 || got.Disk == nil || *got.Disk != disk.usage {
		t.Fatalf("usage = %+v, disk %+v", got, got.Disk)
	}
	if len(sizes.dirs) != 1 || sizes.dirs[0] != "/b" || len(disk.paths) != 1 || disk.paths[0] != "/b" {
		t.Fatalf("read sizes %q and disk %q, want /b", sizes.dirs, disk.paths)
	}
}

func TestMediaFolderUsageOmitsAnUnreadableDisk(t *testing.T) {
	failing := &fakeDisk{err: errors.New("no such device")}
	got, err := usageFolders(failing, &fakeVideoBytes{}).MediaFolderUsage(context.Background(), 1)
	if err != nil || got.Disk != nil || got.VideoBytes != 42 {
		t.Fatalf("failing disk: usage = %+v, err = %v; want video bytes without disk", got, err)
	}

	hung := &fakeDisk{block: make(chan struct{})}
	t.Cleanup(func() { close(hung.block) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = usageFolders(hung, &fakeVideoBytes{}).MediaFolderUsage(ctx, 1)
	if err != nil || got.Disk != nil {
		t.Fatalf("hung disk: usage = %+v, err = %v; want no disk", got, err)
	}
}

func TestMediaFolderUsageRejectsAnUnknownFolder(t *testing.T) {
	_, err := usageFolders(&fakeDisk{}, &fakeVideoBytes{}).MediaFolderUsage(context.Background(), 3)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
