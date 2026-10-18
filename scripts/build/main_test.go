package main

import (
	"os"
	"path/filepath"
	"testing"
)

func prepareRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, distPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, distPath, keepFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func distEntries(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, distPath))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// Keep .gitkeep so go build works before the SPA has been built.
func TestCleanDistRemovesArtefactsButKeepsThePlaceholder(t *testing.T) {
	root := prepareRoot(t)
	dist := filepath.Join(root, distPath)
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("前回の出力"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanDist(root); err != nil {
		t.Fatal(err)
	}

	names := distEntries(t, root)
	if len(names) != 1 || names[0] != keepFile {
		t.Errorf("前回の生成物だけを消していない: %v", names)
	}
}

// Create a missing embed directory before invoking Vite.
func TestCleanDistCreatesTheDirectoryWhenItIsMissing(t *testing.T) {
	root := t.TempDir()

	if err := cleanDist(root); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(root, distPath))
	if err != nil {
		t.Fatalf("置き場所を作っていない: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("%s がディレクトリではない", distPath)
	}
}

func TestCleanDistLeavesACleanDirectoryAlone(t *testing.T) {
	root := prepareRoot(t)

	if err := cleanDist(root); err != nil {
		t.Fatal(err)
	}

	names := distEntries(t, root)
	if len(names) != 1 || names[0] != keepFile {
		t.Errorf("掃除済みの置き場所を変えた: %v", names)
	}
}
