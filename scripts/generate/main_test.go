package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFile(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangedFindsStaleGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	paths := []string{"a.gen", "b.gen"}
	writeFile(t, root, "a.gen", "one")
	writeFile(t, root, "b.gen", "two")

	before, err := snapshot(root, paths)
	if err != nil {
		t.Fatal(err)
	}

	// Uncommitted schema edits are valid when generated content already matches.
	writeFile(t, root, "a.gen", "one")
	after, err := snapshot(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if stale := changed(before, after, paths); len(stale) != 0 {
		t.Errorf("内容が同じなのに差分として報告した: %v", stale)
	}

	writeFile(t, root, "b.gen", "two-updated")
	after, err = snapshot(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if stale := changed(before, after, paths); !slices.Equal(stale, []string{"b.gen"}) {
		t.Errorf("変わったファイルを特定できない: %v", stale)
	}
}

func TestSnapshotTreatsMissingFileAsChange(t *testing.T) {
	root := t.TempDir()
	paths := []string{"new.gen"}

	before, err := snapshot(root, paths)
	if err != nil {
		t.Fatalf("存在しない生成物で失敗した: %v", err)
	}

	writeFile(t, root, "new.gen", "generated")
	after, err := snapshot(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if stale := changed(before, after, paths); !slices.Equal(stale, paths) {
		t.Errorf("生成物が新しく増えたことを差分にしていない: %v", stale)
	}
}

// Stop at the first failure to avoid partially updated bindings and ambiguous stale-output errors.
func TestGenerateStopsAtTheFirstFailure(t *testing.T) {
	calls := []string{}
	failing := func(dir string, name string, args ...string) error {
		calls = append(calls, name)
		return errors.New("生成器が落ちた")
	}

	if err := generate(t.TempDir(), "oapi-codegen", "openapi-typescript", "shadcn", failing); err == nil {
		t.Fatal("生成器の失敗を伝えていない")
	}
	if !slices.Equal(calls, []string{"oapi-codegen"}) {
		t.Errorf("失敗したあとも生成を続けた: %v", calls)
	}

	calls = nil
	succeeding := func(dir string, name string, args ...string) error {
		calls = append(calls, name)
		return nil
	}
	if err := generate(t.TempDir(), "oapi-codegen", "openapi-typescript", "shadcn", succeeding); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"oapi-codegen", "oapi-codegen", "openapi-typescript", "shadcn"}) {
		t.Errorf("生成器の呼び出しが揃っていない: %v", calls)
	}
}
