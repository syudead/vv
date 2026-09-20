package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		if key == envDataDir {
			return filepath.Join(os.TempDir(), "vv-config-test")
		}
		return ""
	}
}

func TestLoadConfigReadsSupportedEnvironment(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	cfg, err := LoadConfig(envFrom(map[string]string{
		"MDM_ADDR": "127.0.0.1:9000", "MDM_DATA_DIR": dataDir, "MDM_LOG_LEVEL": "debug",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:9000" || cfg.DataDir != dataDir || cfg.LogLevel != "debug" {
		t.Fatalf("environment was not applied: %+v", cfg)
	}
}

func TestLoadConfigReportsAllInvalidValuesAtOnce(t *testing.T) {
	_, err := LoadConfig(envFrom(map[string]string{
		"MDM_ADDR": "missing-port", "MDM_DATA_DIR": "relative/data", "MDM_LOG_LEVEL": "verbose",
	}))
	if err == nil {
		t.Fatal("invalid configuration succeeded")
	}
	for _, name := range []string{"MDM_ADDR", "MDM_DATA_DIR", "MDM_LOG_LEVEL"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not mention %s: %v", name, err)
		}
	}
}

func TestVerifyCreatesDataAndThumbnailDirectories(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data", "nested")
	cfg := Config{Addr: ":8080", DataDir: dataDir, LogLevel: "info"}
	if err := cfg.Verify(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dataDir, cfg.ThumbnailsDir()} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("directory not created: %s: %v", path, err)
		}
	}
}

func TestVerifyReportsUncreatableThumbnailsDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "thumbnails"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := (Config{Addr: ":8080", DataDir: dataDir, LogLevel: "info"}).Verify()
	if err == nil || !strings.Contains(err.Error(), "thumbnails") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestThumbnailsDirIsDerivedFromDataDir(t *testing.T) {
	cfg := Config{DataDir: "/data"}
	if got, want := cfg.ThumbnailsDir(), filepath.Join("/data", "thumbnails"); got != want {
		t.Fatalf("ThumbnailsDir() = %q, want %q", got, want)
	}
}
