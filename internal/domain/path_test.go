package domain

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestPathWithinRootUsesWindowsCaseRules(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path comparison")
	}
	if !PathWithinRoot(`C:\Media`, `c:\media`) {
		t.Fatal("same Windows path with different casing was not recognized")
	}
	if !PathWithinRoot(`C:\Media`, `c:\MEDIA\Movies\movie.mp4`) {
		t.Fatal("Windows descendant with different casing was not recognized")
	}
}

func TestPathInsideRoot(t *testing.T) {
	root := filepath.FromSlash("/media")
	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{"direct child", "/media/a.mp4", true},
		{"nested", "/media/sub/a.mp4", true},
		{"root itself", "/media", false},
		{"prefix only", "/media-other/a.mp4", false},
		{"parent", "/", false},
		{"outside", "/etc/passwd", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathInsideRoot(root, filepath.FromSlash(tc.path)); got != tc.want {
				t.Fatalf("PathInsideRoot(%q, %q) = %v, want %v", root, tc.path, got, tc.want)
			}
		})
	}
}

func TestMediaFileInsideRoot(t *testing.T) {
	root := filepath.FromSlash("/media")
	for _, tc := range []struct {
		name           string
		path, resolved string
		want           bool
	}{
		{"plain file", "/media/a.mp4", "/media/a.mp4", true},
		{"link inside", "/media/link.mp4", "/media/sub/a.mp4", true},
		{"link escapes", "/media/link.mp4", "/etc/passwd", false},
		{"link to prefix sibling", "/media/link.mp4", "/media-other/a.mp4", false},
		{"link to root", "/media/link", "/media", false},
		{"path outside", "/media-other/a.mp4", "/media/a.mp4", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MediaFileInsideRoot(root, filepath.FromSlash(tc.path), filepath.FromSlash(tc.resolved))
			if got != tc.want {
				t.Fatalf("MediaFileInsideRoot(%q, %q, %q) = %v, want %v", root, tc.path, tc.resolved, got, tc.want)
			}
		})
	}
}

func TestSamePath(t *testing.T) {
	if !SamePath(filepath.FromSlash("/media/"), filepath.FromSlash("/media")) {
		t.Fatal("cleaned forms of the same path differ")
	}
	if SamePath(filepath.FromSlash("/media"), filepath.FromSlash("/media/sub")) {
		t.Fatal("a descendant was treated as the same path")
	}
}
