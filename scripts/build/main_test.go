package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// prepareRoot は web/dist の置き場所だけを持つ版管理の根を模す。
func prepareRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, distPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, distPath, ".gitkeep"), nil, 0o644); err != nil {
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

func TestPreserveDistRestoresPlaceholderAfterBuild(t *testing.T) {
	root := prepareRoot(t)

	err := preserveDist(root, func() error {
		// ビルドが本番の資材を書く様子を模す。
		if err := os.MkdirAll(filepath.Join(root, distPath, "assets"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, distPath, "index.html"), []byte("built"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}

	names := distEntries(t, root)
	if len(names) != 1 || names[0] != ".gitkeep" {
		t.Errorf("ビルド後に web/dist が元へ戻っていない: %v", names)
	}
}

// ビルドが失敗しても作業ツリーを汚したままにしない。
func TestPreserveDistRestoresAfterFailure(t *testing.T) {
	root := prepareRoot(t)
	failure := errors.New("ビルド失敗")

	err := preserveDist(root, func() error {
		if err := os.MkdirAll(filepath.Join(root, distPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, distPath, "index.html"), []byte("half built"), 0o644); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Errorf("ビルドの失敗を伝えていない: %v", err)
	}

	names := distEntries(t, root)
	if len(names) != 1 || names[0] != ".gitkeep" {
		t.Errorf("失敗時に web/dist が元へ戻っていない: %v", names)
	}

	// 退避先も残さない。
	local, err := os.ReadDir(filepath.Join(root, ".local"))
	if err != nil {
		t.Fatal(err)
	}
	if len(local) != 0 {
		t.Errorf("退避先を片付けていない: %v", local)
	}
}

// web/dist が無い状態から始めた場合、ビルドが作った出力だけを片付ける。
func TestPreserveDistRemovesOutputWhenNothingWasThere(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".local"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := preserveDist(root, func() error {
		return os.MkdirAll(filepath.Join(root, distPath), 0o755)
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, distPath)); !os.IsNotExist(err) {
		t.Errorf("退避するものが無かったのに出力を残した: %v", err)
	}
	local, err := os.ReadDir(filepath.Join(root, ".local"))
	if err != nil {
		t.Fatal(err)
	}
	if len(local) != 0 {
		t.Errorf("退避先を片付けていない: %v", local)
	}
}
