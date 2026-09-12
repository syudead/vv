package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envFrom は環境変数の読み取りを差し替えるための getenv を返す。
func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfigUsesDefaultsWhenUnset(t *testing.T) {
	cfg, err := LoadConfig(envFrom(nil))
	if err != nil {
		t.Fatalf("既定値だけで組み立てられるはずが失敗した: %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.MediaDir != "/media" {
		t.Errorf("MediaDir = %q, want %q", cfg.MediaDir, "/media")
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/data")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoadConfigReadsEnvironment(t *testing.T) {
	cfg, err := LoadConfig(envFrom(map[string]string{
		"MDM_ADDR":      "127.0.0.1:9000",
		"MDM_MEDIA_DIR": "/srv/videos",
		"MDM_DATA_DIR":  "/srv/data",
		"MDM_LOG_LEVEL": "debug",
	}))
	if err != nil {
		t.Fatalf("正しい値で失敗した: %v", err)
	}

	if cfg.Addr != "127.0.0.1:9000" || cfg.MediaDir != "/srv/videos" ||
		cfg.DataDir != "/srv/data" || cfg.LogLevel != "debug" {
		t.Errorf("環境変数が反映されていない: %+v", cfg)
	}
}

// 契約（contracts/configuration.md）は「不正な項目を一度に列挙して終了する」ことを
// 求めている。1つ見つけて即終了すると、設定を1回直すごとに再起動する往復が生じる。
func TestLoadConfigReportsAllInvalidValuesAtOnce(t *testing.T) {
	_, err := LoadConfig(envFrom(map[string]string{
		"MDM_ADDR":      "ポートが無い",
		"MDM_MEDIA_DIR": "relative/media",
		"MDM_DATA_DIR":  "relative/data",
		"MDM_LOG_LEVEL": "verbose",
	}))
	if err == nil {
		t.Fatal("4 項目とも不正なのに成功した")
	}

	message := err.Error()
	for _, name := range []string{"MDM_ADDR", "MDM_MEDIA_DIR", "MDM_DATA_DIR", "MDM_LOG_LEVEL"} {
		if !strings.Contains(message, name) {
			t.Errorf("出力に %s が含まれていない:\n%s", name, message)
		}
	}
}

func TestLoadConfigRejectsUnknownLogLevel(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		if _, err := LoadConfig(envFrom(map[string]string{"MDM_LOG_LEVEL": level})); err != nil {
			t.Errorf("LogLevel %q は受け付けられるべき: %v", level, err)
		}
	}
	if _, err := LoadConfig(envFrom(map[string]string{"MDM_LOG_LEVEL": "trace"})); err == nil {
		t.Error("未知の LogLevel を受け付けてしまった")
	}
}

func TestVerifyCreatesDataDirAndChecksMediaDir(t *testing.T) {
	root := t.TempDir()
	mediaDir := filepath.Join(root, "media")
	if err := os.Mkdir(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "data", "nested")

	cfg := Config{Addr: ":8080", MediaDir: mediaDir, DataDir: dataDir, LogLevel: "info"}
	if err := cfg.Verify(); err != nil {
		t.Fatalf("検証に失敗した: %v", err)
	}

	info, err := os.Stat(dataDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("MDM_DATA_DIR が作成されていない: %v", err)
	}
}

func TestVerifyReportsUnreadableMediaDir(t *testing.T) {
	root := t.TempDir()
	cfg := Config{
		Addr:     ":8080",
		MediaDir: filepath.Join(root, "missing"),
		DataDir:  filepath.Join(root, "data"),
		LogLevel: "info",
	}

	err := cfg.Verify()
	if err == nil {
		t.Fatal("存在しない MDM_MEDIA_DIR を受け付けてしまった")
	}
	if !strings.Contains(err.Error(), "MDM_MEDIA_DIR") {
		t.Errorf("出力に MDM_MEDIA_DIR が含まれていない:\n%s", err)
	}
}
