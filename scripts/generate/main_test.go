package main

import (
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

	// 生成し直しても内容が同じなら差分ではない。手元で openapi.yaml を
	// 編集中でも、生成物と整合していれば task check は通る必要がある。
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

func TestReadToolVersionsRejectsIncompleteFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "scripts/tool-versions.json", `{"oapiCodegen":"v2.8.0"}`)
	if _, err := readToolVersions(root); err == nil {
		t.Error("生成器の版が欠けているのに受理した")
	}

	writeFile(t, root, "scripts/tool-versions.json", `{"oapiCodegen":"v2.8.0","openapiTypescript":"7.13.0"}`)
	versions, err := readToolVersions(root)
	if err != nil {
		t.Fatal(err)
	}
	if versions.OapiCodegen != "v2.8.0" || versions.OpenapiTypescript != "7.13.0" {
		t.Errorf("版を読み違えた: %+v", versions)
	}
}
