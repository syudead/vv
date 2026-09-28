package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindDirectives(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("clean.go", "package p\n\n// 通常のコメント。nolint という語を含んでも本文の先頭でなければよい。\nvar s = \"//nolint:gosec\"\n")
	write("line.go", "package p\n\nvar a = 1 //nolint:gosec // 理由\n")
	write("block.go", "package p\n\n/* nolint */\nvar b = 1\n")
	write("sub/own_line_test.go", "package p\n\n//nolint:errorlint\nvar c = 1\n\n// NoLint:all\nvar d = 1\n")
	write("node_modules/dep/x.go", "package x\n\nvar e = 1 //nolint\n")

	found, err := findDirectives(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 4 {
		t.Fatalf("found = %q, want 4 directives (line, block, and two in sub/)", found)
	}
}
