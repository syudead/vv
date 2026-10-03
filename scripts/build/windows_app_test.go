package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// zip の <版> はタグ（v を除く）か、無ければ commit の sha-<12 桁>（contracts §1）。
func TestReleaseVersion(t *testing.T) {
	commit := "0123456789abcdef0123456789abcdef01234567\n"
	cases := []struct {
		env, want string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3-rc.1", "1.2.3-rc.1"},
		{"", "sha-0123456789ab"},
		{"  ", "sha-0123456789ab"},
	}
	for _, tc := range cases {
		got, err := releaseVersion(tc.env, commit)
		if err != nil {
			t.Errorf("releaseVersion(%q): %v", tc.env, err)
			continue
		}
		if got != tc.want {
			t.Errorf("releaseVersion(%q) = %q, want %q", tc.env, got, tc.want)
		}
	}
}

// ファイル名に入る版なので、パスの区切りなどは受け付けない。
func TestReleaseVersionRejectsUnsafeNames(t *testing.T) {
	for _, env := range []string{"v1/2", "..", "v", "1 2"} {
		if _, err := releaseVersion(env, "0123456789abcdef"); err == nil {
			t.Errorf("releaseVersion(%q) が通った", env)
		}
	}
	if _, err := releaseVersion("", "abc"); err == nil {
		t.Error("短い commit から版を作った")
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func serve(t *testing.T, body []byte) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

// 固定した SHA-256 と合えば取り、2 回目は取ってあるものを使う。
func TestFetchFFmpegVerifiesAndCaches(t *testing.T) {
	body := []byte("essentials")
	server, calls := serve(t, body)
	cache := filepath.Join(t.TempDir(), "cache")

	for range 2 {
		got, err := fetchFFmpeg(server.Client(), server.URL, cache, "ffmpeg.zip", digest(body))
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(cache, "ffmpeg.zip") {
			t.Errorf("path = %s", got)
		}
	}
	if *calls != 1 {
		t.Errorf("取得の回数 = %d, want 1", *calls)
	}
}

// SHA-256 が合わない FFmpeg ではビルドが失敗し、合わないファイルを残さない（R-12）。
func TestFetchFFmpegFailsOnMismatch(t *testing.T) {
	server, _ := serve(t, []byte("tampered"))
	cache := t.TempDir()

	_, err := fetchFFmpeg(server.Client(), server.URL, cache, "ffmpeg.zip", digest([]byte("essentials")))
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("err = %v, want SHA-256 mismatch", err)
	}
	if _, err := os.Stat(filepath.Join(cache, "ffmpeg.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("合わないファイルが残った: %v", err)
	}
}

// 取ってあるファイルも毎回確かめる。
func TestFetchFFmpegRechecksTheCache(t *testing.T) {
	server, calls := serve(t, []byte("essentials"))
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(cache, "ffmpeg.zip"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := fetchFFmpeg(server.Client(), server.URL, cache, "ffmpeg.zip", digest([]byte("essentials"))); err == nil {
		t.Fatal("合わない取り置きで通った")
	}
	if *calls != 0 {
		t.Errorf("取得の回数 = %d, want 0", *calls)
	}
}

func TestFetchFFmpegFailsOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	cache := t.TempDir()

	if _, err := fetchFFmpeg(server.Client(), server.URL, cache, "ffmpeg.zip", digest(nil)); err == nil {
		t.Fatal("404 で通った")
	}
	entries, _ := os.ReadDir(cache)
	if len(entries) != 0 {
		t.Errorf("取得に失敗したのにファイルが残った: %v", entries)
	}
}

func writeZip(t *testing.T, name string, files map[string]string) {
	t.Helper()
	out, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(out)
	for entry, body := range files {
		w, err := writer.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// essentials の zip から ffmpeg.exe・ffprobe.exe・LICENSE だけを取り出す（contracts §1）。
func TestExtractFFmpegTakesOnlyTheBundledFiles(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "essentials.zip")
	writeZip(t, archive, map[string]string{
		"ffmpeg-9.0-essentials_build/bin/ffmpeg.exe":  "ffmpeg",
		"ffmpeg-9.0-essentials_build/bin/ffprobe.exe": "ffprobe",
		"ffmpeg-9.0-essentials_build/bin/ffplay.exe":  "ffplay",
		"ffmpeg-9.0-essentials_build/LICENSE":         "GPL",
		"ffmpeg-9.0-essentials_build/README.txt":      "readme",
		"ffmpeg-9.0-essentials_build/doc/ffmpeg.html": "doc",
	})
	dest := filepath.Join(dir, "out")

	if err := extractFFmpeg(archive, dest); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{"ffmpeg.exe": "ffmpeg", "ffprobe.exe": "ffprobe", "LICENSE.txt": "GPL"}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		t.Errorf("取り出したもの: %v", entries)
	}
	for name, body := range want {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || string(got) != body {
			t.Errorf("%s = %q, %v", name, got, err)
		}
	}
}

func TestExtractFFmpegFailsWhenAFileIsMissing(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "essentials.zip")
	writeZip(t, archive, map[string]string{
		"ffmpeg-9.0-essentials_build/bin/ffmpeg.exe": "ffmpeg",
		"ffmpeg-9.0-essentials_build/LICENSE":        "GPL",
	})

	err := extractFFmpeg(archive, filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "bin/ffprobe.exe") {
		t.Fatalf("err = %v, want missing ffprobe", err)
	}
}

// zip は VVMDM-<版>-windows-amd64/ を根に持ち、契約 §1 の並びになる。
func TestZipDirKeepsTheFolderAsTheRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, windowsAppName("1.2.3"))
	files := []string{"VVMDM.exe", "README.txt", "ffmpeg/ffmpeg.exe", "ffmpeg/ffprobe.exe", "ffmpeg/LICENSE.txt", "ffmpeg/README.txt"}
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, windowsAppName("1.2.3")+".zip")

	if err := zipDir(root, out); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	var got []string
	for _, entry := range reader.File {
		if !strings.HasSuffix(entry.Name, "/") {
			got = append(got, entry.Name)
		}
	}
	want := make([]string, 0, len(files))
	for _, name := range files {
		want = append(want, "VVMDM-1.2.3-windows-amd64/"+name)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("zip の中身 = %v, want %v", got, want)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.part"))
	if len(leftovers) != 0 {
		t.Errorf("一時ファイルが残った: %v", leftovers)
	}
}

// README は Windows のメモ帳で読むので CRLF にし、版と FFmpeg のソースの入手先を書く。
func TestReadmesUseCRLFAndNameTheVersions(t *testing.T) {
	app := appReadme("1.2.3")
	ff := ffmpegReadme("9.0.2")
	for name, text := range map[string]string{"README.txt": app, "ffmpeg/README.txt": ff} {
		if strings.Count(text, "\n") != strings.Count(text, "\r\n") {
			t.Errorf("%s に CRLF でない改行がある", name)
		}
	}
	for _, want := range []string{"1.2.3", "Extract All", "Run anyway", `%LOCALAPPDATA%\VVMDM`, "--port"} {
		if !strings.Contains(app, want) {
			t.Errorf("README.txt に %q が無い", want)
		}
	}
	for _, want := range []string{"9.0.2", "https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz", ffmpegURL("9.0.2")} {
		if !strings.Contains(ff, want) {
			t.Errorf("ffmpeg/README.txt に %q が無い", want)
		}
	}
}
