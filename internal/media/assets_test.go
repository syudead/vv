package media

import (
	"os"
	"path/filepath"
	"testing"
)

// 生成物の有無は、配信側（internal/httpapi）と同じ置き場の規則で確かめる。
func TestAssetsAvailability(t *testing.T) {
	dir := t.TempDir()
	assets := NewAssets(dir)
	const key = "abcdef0123456789:1024"

	if assets.PreviewAvailable(key) || assets.SeekThumbnailsAvailable(key) {
		t.Fatal("無い生成物をあると答えた")
	}

	preview := filepath.Join(dir, "preview", "ab", "abcdef0123456789_1024.mp4")
	if err := os.MkdirAll(filepath.Dir(preview), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preview, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if assets.PreviewAvailable(key) {
		t.Fatal("空のプレビューをあると答えた")
	}
	if err := os.WriteFile(preview, []byte("mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !assets.PreviewAvailable(key) {
		t.Fatal("あるプレビューを無いと答えた")
	}

	if err := os.MkdirAll(filepath.Join(dir, "seek", "ab", "abcdef0123456789_1024"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !assets.SeekThumbnailsAvailable(key) {
		t.Fatal("あるシーク用プレビューの置き場を無いと答えた")
	}

	if NewAssets("").PreviewAvailable(key) || NewAssets(dir).PreviewAvailable("") {
		t.Fatal("置き場か内容の識別子が無いのにあると答えた")
	}
}
