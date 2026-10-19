package main

// Build the Windows ZIP for task build-windows-app and CI; see docs/design-docs/windows-app.md.

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/syudead/vv/scripts/devtools"
)

// Pin Gyan.dev essentials from GyanD/codexffmpeg releases. Update the version and SHA-256 together; see docs/how-to/dependency-updates.md.
const (
	ffmpegVersion = "9.0.2"
	// Published at https://www.gyan.dev/ffmpeg/builds/packages/<archive>.sha256.
	ffmpegSHA256 = "60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba"
)

const (
	windowsAppDir  = "dist"
	ffmpegCacheDir = "dist/cache"
	winresJSON     = "cmd/mdm/winres/winres.json"
	// Remove temporary .syso files after packaging: Go includes them in every cmd/mdm build for that platform.
	winresOutPrefix = "cmd/mdm/rsrc"
	downloadTimeout = 10 * time.Minute
)

func ffmpegArchiveName(version string) string {
	return "ffmpeg-" + version + "-essentials_build.zip"
}

// Versioned release URLs keep the pinned archive available for later builds.
func ffmpegURL(version string) string {
	return "https://github.com/GyanD/codexffmpeg/releases/download/" + version + "/" + ffmpegArchiveName(version)
}

func windowsAppName(version string) string {
	return "VVMDM-" + version + "-windows-amd64"
}

var (
	versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]*$`)
	// Numeric a.b.c.d version required by go-winres.
	winresVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,3}$`)
)

// releaseVersion uses VERSION without its v prefix, falling back to sha-<12 digits> for local and manual builds.
func releaseVersion(env, commit string) (string, error) {
	env = strings.TrimSpace(env)
	version := strings.TrimPrefix(env, "v")
	if env == "" {
		commit = strings.TrimSpace(commit)
		if len(commit) < 12 {
			return "", fmt.Errorf("commit %q から版を決められません", commit)
		}
		version = "sha-" + commit[:12]
	}
	if !versionPattern.MatchString(version) {
		return "", fmt.Errorf("版 %q はファイル名に使えません（英数字と . _ - だけ）", version)
	}
	return version, nil
}

func sha256File(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifySHA256(name, want string) error {
	got, err := sha256File(name)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("%s の SHA-256 が固定した値と合いません（期待 %s、実際 %s）", filepath.Base(name), want, got)
	}
	return nil
}

// fetchPinned verifies cached archives too; a mismatch removes the cache and fails so the next run can download it again.
func fetchPinned(client *http.Client, url, cacheDir, name, want string) (string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(cacheDir, name)
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		if err := download(client, url, dest); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if err := verifySHA256(dest, want); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// download removes partial output on failure.
func download(client *http.Client, url, dest string) error {
	fmt.Printf("Downloading %s\n", url)
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("%s を取得できません: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s を取得できません: %s", url, response.Status)
	}
	temp, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".*.part")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if _, err := io.Copy(temp, response.Body); err != nil {
		_ = temp.Close()
		return fmt.Errorf("%s を取得できません: %w", url, err)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), dest)
}

// Paths inside the archive, excluding its root directory, mapped to bundled names.
var ffmpegFiles = map[string]string{
	"bin/ffmpeg.exe":  "ffmpeg.exe",
	"bin/ffprobe.exe": "ffprobe.exe",
	"LICENSE":         "LICENSE.txt",
}

// extractFFmpeg selects ffmpegFiles from the archive beneath its versioned root directory.
func extractFFmpeg(archive, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	want := map[string]string{}
	for inner, target := range ffmpegFiles {
		want[inner] = filepath.Join(destDir, target)
	}
	return extractZip(archive, want)
}

// extractZip copies the wanted entries, named below the archive's root directory, to their targets.
func extractZip(archive string, want map[string]string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("%s を開けません: %w", filepath.Base(archive), err)
	}
	defer func() { _ = reader.Close() }()
	found := map[string]bool{}
	for _, entry := range reader.File {
		_, inner, ok := strings.Cut(entry.Name, "/")
		target, wanted := want[inner]
		if !ok || !wanted {
			continue
		}
		if found[inner] {
			return fmt.Errorf("%s に %s が 2 つあります", filepath.Base(archive), inner)
		}
		found[inner] = true
		if err := extractEntry(entry, target); err != nil {
			return err
		}
	}
	return requireFound(archive, want, found)
}

func extractEntry(entry *zip.File, dest string) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	return writeFile(dest, source, 0o644)
}

// Use CRLF for Windows Notepad.
func crlf(text string) string {
	return strings.ReplaceAll(text, "\n", "\r\n")
}

// Identify the bundled FFmpeg version and source location.
func ffmpegReadme(version string) string {
	return crlf(`FFmpeg ` + version + ` (Gyan.dev "essentials" build for Windows x64)

This folder holds ffmpeg.exe and ffprobe.exe from the FFmpeg ` + version + ` essentials build
by Gyan Doshi. VVMDM uses them to read video metadata, make thumbnails and
convert videos for playback. They are licensed under the GNU General Public
License version 3; see LICENSE.txt.

Build used:
  ` + ffmpegURL(version) + `

Source code:
  FFmpeg ` + version + `: https://ffmpeg.org/releases/ffmpeg-` + version + `.tar.xz
  The libraries in this build and their versions: https://www.gyan.dev/ffmpeg/builds/
  FFmpeg project: https://ffmpeg.org/
`)
}

// Windows distribution instructions; see the Windows app section of docs/how-to/running-vv.md.
func appReadme(version string) string {
	return crlf(`VVMDM ` + version + ` for Windows (x64)

Start
  1. Right-click the zip and choose "Extract All...". VVMDM does not start
     from inside the zip.
  2. Open the extracted folder and run VVMDM.exe. Keep the ffmpeg folder next
     to VVMDM.exe.
  3. VVMDM is not code-signed, so the first time Windows SmartScreen may show
     "Windows protected your PC". Click "More info", then "Run anyway".
  4. Create the owner account, then add your video folders in Settings and
     start a scan.

  VVMDM needs the Microsoft Edge WebView2 Runtime, which Windows 10 and 11
  normally include. If it is missing, VVMDM shows where to get it.
  Closing the window stops VVMDM. Running VVMDM.exe again while it is open
  brings the open window to the front.

Where your data is
  %LOCALAPPDATA%\VVMDM\data       library database and thumbnails
  %LOCALAPPDATA%\VVMDM\webview2   window data (the sign-in cookie)
  %LOCALAPPDATA%\VVMDM\logs       vvmdm.log (this run), vvmdm.1.log (the previous run)
  Nothing is written next to VVMDM.exe, and your video files are never changed.

Update
  Close VVMDM, then replace the contents of the extracted folder with the
  contents of the new zip (or extract the new zip to a new folder and delete
  the old one). Your data under %LOCALAPPDATA%\VVMDM stays; the new version
  updates it when it starts.

Port and local network
  VVMDM listens on port 47880. If another program uses that port, start
  VVMDM with another one, for example from a shortcut:  VVMDM.exe --port 47881
  Only this PC can open VVMDM until you turn on "Allow connections from the
  local network" in Settings > Network. When Windows Firewall asks, allow
  VVMDM on private networks.

Remove
  Delete the extracted folder. To delete your library too, delete
  %LOCALAPPDATA%\VVMDM.

The ffmpeg folder holds FFmpeg; see ffmpeg\README.txt and ffmpeg\LICENSE.txt.
More: https://github.com/syudead/vv/blob/main/docs/how-to/running-vv.md#windows-app
`)
}

func currentCommit(root string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("今の commit を読めません: %w", err)
	}
	return string(output), nil
}

// buildWindowsApp runs after the SPA build and returns the ZIP path.
func buildWindowsApp(root, versionEnv string) (string, error) {
	commit := ""
	if strings.TrimSpace(versionEnv) == "" {
		var err error
		if commit, err = currentCommit(root); err != nil {
			return "", err
		}
	}
	version, err := releaseVersion(versionEnv, commit)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: downloadTimeout}
	archive, err := fetchPinned(client, ffmpegURL(ffmpegVersion), filepath.Join(root, ffmpegCacheDir),
		ffmpegArchiveName(ffmpegVersion), ffmpegSHA256)
	if err != nil {
		return "", err
	}

	name := windowsAppName(version)
	staging := filepath.Join(root, windowsAppDir, name)
	if err := os.RemoveAll(staging); err != nil {
		return "", err
	}
	if err := extractFFmpeg(archive, filepath.Join(staging, "ffmpeg")); err != nil {
		return "", err
	}
	files := map[string]string{
		filepath.Join(staging, "ffmpeg", "README.txt"): ffmpegReadme(ffmpegVersion),
		filepath.Join(staging, "README.txt"):           appReadme(version),
	}
	for file, text := range files {
		if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
			return "", err
		}
	}

	if err := buildDesktopExe(root, version, filepath.Join(staging, "VVMDM.exe")); err != nil {
		return "", err
	}

	out := filepath.Join(root, windowsAppDir, name+".zip")
	if err := zipDir(staging, out); err != nil {
		return "", err
	}
	if err := os.RemoveAll(staging); err != nil {
		return "", err
	}
	return out, nil
}

// buildDesktopExe embeds the icon and manifest with go-winres before compiling VVMDM.exe.
func buildDesktopExe(root, version, exe string) error {
	winres, err := devtools.GoToolPath(root, "go-winres")
	if err != nil {
		return err
	}
	args := []string{"make", "--in", winresJSON, "--arch", "amd64", "--out", winresOutPrefix}
	if winresVersion.MatchString(version) {
		args = append(args, "--product-version", version, "--file-version", version)
	}
	syso := winresOutPrefix + "_windows_amd64.syso"
	defer func() { _ = os.Remove(filepath.Join(root, syso)) }()
	if err := devtools.Run(root, winres, args...); err != nil {
		return err
	}
	return devtools.RunEnv(root, []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}, "go", "build",
		"-trimpath", "-tags", "desktop",
		"-ldflags", "-s -w -H=windowsgui -X main.version="+version,
		"-o", exe, "./cmd/mdm")
}
