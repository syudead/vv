package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envFrom は環境変数の読み取りを差し替えるための getenv を返す。
func envFrom(values map[string]string) func(string) string {
	return func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		// Windows needs a volume even for root-relative paths.
		if key == envMediaDir || key == envDataDir {
			return filepath.Join(os.TempDir(), "vv-config-test")
		}
		return ""
	}
}

func TestLoadConfigUsesDefaultsWhenUnset(t *testing.T) {
	cfg, err := LoadConfig(func(string) string { return "" })
	if filepath.IsAbs(defaultMediaDir) && err != nil {
		t.Fatalf("既定値だけで組み立てられるはずが失敗した: %v", err)
	}
	if !filepath.IsAbs(defaultMediaDir) && (err == nil ||
		!strings.Contains(err.Error(), envMediaDir) || !strings.Contains(err.Error(), envDataDir)) {
		t.Fatalf("platform must reject defaults without an absolute volume: %v", err)
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
	mediaDir := filepath.Join(t.TempDir(), "videos")
	dataDir := filepath.Join(t.TempDir(), "data")
	cfg, err := LoadConfig(envFrom(map[string]string{
		"MDM_ADDR":      "127.0.0.1:9000",
		"MDM_MEDIA_DIR": mediaDir,
		"MDM_DATA_DIR":  dataDir,
		"MDM_LOG_LEVEL": "debug",
	}))
	if err != nil {
		t.Fatalf("正しい値で失敗した: %v", err)
	}

	if cfg.Addr != "127.0.0.1:9000" || cfg.MediaDir != mediaDir ||
		cfg.DataDir != dataDir || cfg.LogLevel != "debug" {
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

// MDM_SCAN_ON_START は未設定なら true。「置くだけで並ぶ」（US1）を成り立たせる
// には自動実行が既定でなければならない（R-108）。
func TestLoadConfigDefaultsScanOnStartToTrue(t *testing.T) {
	cfg, err := LoadConfig(envFrom(nil))
	if err != nil {
		t.Fatalf("既定値だけで組み立てられるはずが失敗した: %v", err)
	}
	if !cfg.ScanOnStart {
		t.Error("ScanOnStart = false, want true（未設定の既定は自動実行）")
	}
}

// true / false は受理し、それ以外は起動中止にする。曖昧な値を黙って既定へ
// 倒すと、自動取り込みが動いていない理由が分からなくなる。
func TestLoadConfigParsesScanOnStart(t *testing.T) {
	accepted := map[string]bool{"true": true, "false": false}
	for value, want := range accepted {
		cfg, err := LoadConfig(envFrom(map[string]string{"MDM_SCAN_ON_START": value}))
		if err != nil {
			t.Errorf("MDM_SCAN_ON_START=%q は受け付けられるべき: %v", value, err)
			continue
		}
		if cfg.ScanOnStart != want {
			t.Errorf("MDM_SCAN_ON_START=%q: ScanOnStart = %v, want %v", value, cfg.ScanOnStart, want)
		}
	}

	for _, value := range []string{"yes", "1", "TRUE", "on", "いいえ"} {
		if _, err := LoadConfig(envFrom(map[string]string{"MDM_SCAN_ON_START": value})); err == nil {
			t.Errorf("MDM_SCAN_ON_START=%q を受け付けてしまった", value)
		}
	}
}

// 不正な値は他の設定の誤りとまとめて列挙する。設定を1回直すごとに再起動する
// 往復を避けるため（contracts/configuration.md）。
func TestLoadConfigReportsScanOnStartWithOtherProblems(t *testing.T) {
	_, err := LoadConfig(envFrom(map[string]string{
		"MDM_ADDR":          "ポートが無い",
		"MDM_LOG_LEVEL":     "verbose",
		"MDM_SCAN_ON_START": "ときどき",
	}))
	if err == nil {
		t.Fatal("3 項目とも不正なのに成功した")
	}

	message := err.Error()
	for _, name := range []string{"MDM_ADDR", "MDM_LOG_LEVEL", "MDM_SCAN_ON_START"} {
		if !strings.Contains(message, name) {
			t.Errorf("出力に %s が含まれていない:\n%s", name, message)
		}
	}
}

// 有効な設定値は記録に出す（FR-007）。後から「自動取り込みが有効だったか」を
// 追えるようにする。
func TestLogAttrsIncludesScanOnStart(t *testing.T) {
	cfg, err := LoadConfig(envFrom(map[string]string{"MDM_SCAN_ON_START": "false"}))
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, attr := range cfg.LogAttrs() {
		if attr.Key == "MDM_SCAN_ON_START" {
			found = true
			if attr.Value.Bool() {
				t.Errorf("記録された値 = true, want false")
			}
		}
	}
	if !found {
		t.Error("LogAttrs に MDM_SCAN_ON_START が含まれていない")
	}
}

// サムネイルの置き場所は MDM_DATA_DIR から導出する。設定項目にはしない
// （contracts/configuration.md「導出される場所」）。
func TestThumbnailsDirIsDerivedFromDataDir(t *testing.T) {
	cfg := Config{DataDir: "/data"}
	want := filepath.Join("/data", "thumbnails")
	if got := cfg.ThumbnailsDir(); got != want {
		t.Errorf("ThumbnailsDir() = %q, want %q", got, want)
	}
}

// Verify は MDM_DATA_DIR/thumbnails を作れることまで確認する。起動後に
// 初めて失敗すると、取り込みが静かに進まなくなる。
func TestVerifyCreatesThumbnailsDir(t *testing.T) {
	root := t.TempDir()
	mediaDir := filepath.Join(root, "media")
	if err := os.Mkdir(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "data")

	cfg := Config{Addr: ":8080", MediaDir: mediaDir, DataDir: dataDir, LogLevel: "info"}
	if err := cfg.Verify(); err != nil {
		t.Fatalf("検証に失敗した: %v", err)
	}

	info, err := os.Stat(cfg.ThumbnailsDir())
	if err != nil || !info.IsDir() {
		t.Fatalf("thumbnails が作成されていない: %v", err)
	}
}

// 作成できない場合は、他の確認結果とまとめて列挙する。
func TestVerifyReportsUncreatableThumbnailsDir(t *testing.T) {
	root := t.TempDir()
	mediaDir := filepath.Join(root, "media")
	if err := os.Mkdir(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "data")
	if err := os.Mkdir(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// thumbnails の位置を通常ファイルで塞ぐ。MkdirAll が失敗する状況を作る。
	if err := os.WriteFile(filepath.Join(dataDir, "thumbnails"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Addr: ":8080", MediaDir: mediaDir, DataDir: dataDir, LogLevel: "info"}
	err := cfg.Verify()
	if err == nil {
		t.Fatal("thumbnails を作れないのに成功した")
	}
	if !strings.Contains(err.Error(), "thumbnails") {
		t.Errorf("出力に thumbnails が含まれていない:\n%s", err)
	}
}
