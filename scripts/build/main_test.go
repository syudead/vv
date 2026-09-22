package main

import (
	"os"
	"path/filepath"
	"testing"
)

// prepareRoot は .gitkeep だけを持つ版管理の根を模す。
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

// 前回の生成物は消すが、版管理された .gitkeep は残す。これが消えると
// 埋め込み先が空になり、SPA をビルドしていない状態で go build ./... が通らなくなる。
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

// 置き場所が無ければ作る。埋め込み先が無いまま Vite を呼ぶと、
// 出力先の用意で失敗する経路が増える。
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

// 掃除だけで済ませるので、既に .gitkeep しか無い状態は何も変えない。
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
