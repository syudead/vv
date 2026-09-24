package domain

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeMediaFolderPath(t *testing.T) {
	root := filepath.FromSlash("/media")
	if runtime.GOOS == "windows" {
		root = `C:\media`
	}
	got, err := NormalizeMediaFolderPath(filepath.Join(root, "movies", "..", "shows") + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "shows"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	for _, path := range []string{"", filepath.Join("relative", "media")} {
		if _, err := NormalizeMediaFolderPath(path); !errors.Is(err, ErrInvalidMediaFolder) {
			t.Errorf("NormalizeMediaFolderPath(%q) error = %v, want ErrInvalidMediaFolder", path, err)
		}
	}
}

func TestCheckMediaFolderMutationRejectsRunningScan(t *testing.T) {
	if err := CheckMediaFolderMutation(true); !errors.Is(err, ErrScanRunning) {
		t.Fatalf("error = %v, want ErrScanRunning", err)
	}
	if err := CheckMediaFolderMutation(false); err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
}

func TestCheckMediaFolderPlacementRejectsContainment(t *testing.T) {
	root := filepath.FromSlash("/media")
	if runtime.GOOS == "windows" {
		root = `C:\media`
	}
	movies := filepath.Join(root, "movies")
	folders := []MediaFolder{{ID: 1, Path: movies}, {ID: 2, Path: filepath.Join(root, "shows")}}
	tests := []struct {
		name string
		id   int64
		path string
		want error
	}{
		{"same path", 0, movies, ErrFolderConflict},
		{"inside", 0, filepath.Join(movies, "2024"), ErrFolderConflict},
		{"outside", 0, root, ErrFolderConflict},
		{"sibling", 0, filepath.Join(root, "music"), nil},
		{"sibling sharing a prefix", 0, movies + "-old", nil},
		{"replacing itself", 1, filepath.Join(movies, "2024"), nil},
		{"replacing into another", 1, filepath.Join(root, "shows", "x"), ErrFolderConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckMediaFolderPlacement(false, folders, tt.id, tt.path)
			if !errors.Is(err, tt.want) || (tt.want == nil && err != nil) {
				t.Fatalf("CheckMediaFolderPlacement(%d, %q) = %v, want %v", tt.id, tt.path, err, tt.want)
			}
		})
	}
	// 走査中は包含の有無によらず断る。
	if err := CheckMediaFolderPlacement(true, nil, 0, filepath.Join(root, "music")); !errors.Is(err, ErrScanRunning) {
		t.Fatalf("running scan error = %v, want ErrScanRunning", err)
	}
}
