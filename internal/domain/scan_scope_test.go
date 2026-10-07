package domain

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestDirtyDirectoryContains(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "media")
	flat := DirtyDirectory{Path: filepath.Join(root, "a")}
	deep := DirtyDirectory{Path: filepath.Join(root, "a"), Recursive: true}
	for _, tc := range []struct {
		name string
		dir  DirtyDirectory
		path string
		want bool
	}{
		{"直下のファイル", flat, filepath.Join(root, "a", "1.mp4"), true},
		{"下のディレクトリのファイルは入らない", flat, filepath.Join(root, "a", "b", "1.mp4"), false},
		{"隣のディレクトリ", flat, filepath.Join(root, "ab", "1.mp4"), false},
		{"再帰なら下も入る", deep, filepath.Join(root, "a", "b", "1.mp4"), true},
		{"再帰でも名前の前方一致は入らない", deep, filepath.Join(root, "ab", "1.mp4"), false},
	} {
		if got := tc.dir.Contains(tc.path); got != tc.want {
			t.Errorf("%s: Contains = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeDirtyDirectories(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "media")
	a, ab := filepath.Join(root, "a"), filepath.Join(root, "a", "b")
	got := NormalizeDirtyDirectories([]DirtyDirectory{
		{Path: ab, Recursive: true},
		{Path: a + string(filepath.Separator)},
		{Path: a, Recursive: false},
		{Path: a, Recursive: true},
		{Path: ""},
		{Path: filepath.Join(root, "c")},
	})
	want := []DirtyDirectory{{Path: a, Recursive: true}, {Path: filepath.Join(root, "c")}}
	if !slices.Equal(got, want) {
		t.Fatalf("Normalize = %v, want %v", got, want)
	}
}
