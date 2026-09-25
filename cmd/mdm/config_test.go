package main

import (
	"net/netip"
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

func TestLoadConfigReadsTrustedProxies(t *testing.T) {
	cfg, err := LoadConfig(envFrom(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("既定の TrustedProxies = %v, want 空", cfg.TrustedProxies)
	}

	cfg, err = LoadConfig(envFrom(map[string]string{
		"MDM_TRUSTED_PROXIES": " 10.0.0.1/8, 127.0.0.1\n::1/128 ::ffff:192.168.1.0/120 ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(cfg.TrustedProxies))
	for _, prefix := range cfg.TrustedProxies {
		got = append(got, prefix.String())
	}
	want := []string{"10.0.0.0/8", "127.0.0.1/32", "::1/128", "192.168.1.0/24"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("TrustedProxies = %v, want %v", got, want)
	}
	if !cfg.TrustedProxies[3].Contains(netip.MustParseAddr("192.168.1.20")) {
		t.Error("IPv4 射影で書いたプレフィックスが IPv4 の送信元を含まない")
	}
}

func TestLoadConfigReportsInvalidTrustedProxiesWithOtherProblems(t *testing.T) {
	_, err := LoadConfig(envFrom(map[string]string{
		"MDM_LOG_LEVEL":       "verbose",
		"MDM_TRUSTED_PROXIES": "10.0.0.0/8,10.0.0.0/33,proxy.example,fe80::1%eth0",
	}))
	if err == nil {
		t.Fatal("invalid configuration succeeded")
	}
	for _, want := range []string{"MDM_LOG_LEVEL", `"10.0.0.0/33"`, `"proxy.example"`, `"fe80::1%eth0"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), `"10.0.0.0/8"`) {
		t.Errorf("正しい項まで誤りとした: %v", err)
	}
}
