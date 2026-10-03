package main

// Windows 版の zip（VVMDM-<版>-windows-amd64.zip）を組み立てる。task build-windows-app の
// 実体で、.github/workflows/windows-app.yml も同じ入口を呼ぶ。中身は
// specs/037-windows-app/contracts/windows-app.md §1、FFmpeg の固定は research.md R-12、
// 組み立ての形は R-13 による。

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
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/syudead/vv/scripts/devtools"
)

// 同梱する FFmpeg。Gyan.dev の Windows 版「essentials」を、GyanD/codexffmpeg の
// GitHub Releases から取る。版を上げるときは版と SHA-256 を一緒に変える
// （docs/how-to/dependency-updates.md）。SHA-256 が合わなければビルドは失敗する。
const (
	ffmpegVersion = "9.0.2"
	// Gyan.dev が公開する値（https://www.gyan.dev/ffmpeg/builds/packages/<zip の名前>.sha256）。
	ffmpegSHA256 = "60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba"
)

const (
	windowsAppDir  = "dist"
	ffmpegCacheDir = "dist/cache"
	winresJSON     = "cmd/mdm/winres/winres.json"
	// go-winres が書く .syso の接頭辞。cmd/mdm に置いた .syso は、その GOOS/GOARCH の
	// cmd/mdm の全ビルドに入るので、組み立ての間だけ置いて消す（.gitignore で除く）。
	winresOutPrefix = "cmd/mdm/rsrc"
	downloadTimeout = 10 * time.Minute
)

// ffmpegArchiveName は Gyan.dev の essentials の zip の名前である。
func ffmpegArchiveName(version string) string {
	return "ffmpeg-" + version + "-essentials_build.zip"
}

// ffmpegURL は版の Release に添付された essentials の zip の URL である。
// Release は版ごとに残るので、固定した版を後から取り直せる（R-12）。
func ffmpegURL(version string) string {
	return "https://github.com/GyanD/codexffmpeg/releases/download/" + version + "/" + ffmpegArchiveName(version)
}

// windowsAppName は zip と、その中の根のフォルダの名前である（contracts §1）。
func windowsAppName(version string) string {
	return "VVMDM-" + version + "-windows-amd64"
}

var (
	versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]*$`)
	// winresVersion は go-winres が版の数（a.b.c.d）として埋め込める形である。
	winresVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,3}$`)
)

// releaseVersion は zip の <版> を決める。VERSION があればそれ（タグの v を除く）、
// 無ければ今の commit の sha-<12 桁>（workflow の手動実行と手元のビルド）。
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

// sha256File はファイルの SHA-256 を 16 進で返す。
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

// verifySHA256 は固定した SHA-256 と合うかを確かめる。合わなければ実際の値を添えて誤りにする。
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

// fetchFFmpeg は essentials の zip を cacheDir に取り、SHA-256 を確かめてからパスを返す。
// 取ってある zip も毎回確かめ、合わなければ消して失敗する（次の実行で取り直す）。
func fetchFFmpeg(client *http.Client, url, cacheDir, name, want string) (string, error) {
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

// download は url を dest へ書く。途中で失敗しても dest に半端なファイルを残さない。
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

// ffmpegFiles は essentials の zip の中のパス（根のフォルダを除く）と、同梱する名前である。
var ffmpegFiles = map[string]string{
	"bin/ffmpeg.exe":  "ffmpeg.exe",
	"bin/ffprobe.exe": "ffprobe.exe",
	"LICENSE":         "LICENSE.txt",
}

// extractFFmpeg は essentials の zip から ffmpegFiles だけを destDir へ取り出す。
// zip は根に版のフォルダを 1 つ持つ（ffmpeg-<版>-essentials_build/bin/ffmpeg.exe）。
func extractFFmpeg(archive, destDir string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("%s を開けません: %w", filepath.Base(archive), err)
	}
	defer func() { _ = reader.Close() }()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	found := map[string]bool{}
	for _, entry := range reader.File {
		_, inner, ok := strings.Cut(entry.Name, "/")
		if !ok {
			continue
		}
		target, wanted := ffmpegFiles[inner]
		if !wanted {
			continue
		}
		if found[inner] {
			return fmt.Errorf("%s に %s が 2 つあります", filepath.Base(archive), inner)
		}
		found[inner] = true
		if err := extractEntry(entry, filepath.Join(destDir, target)); err != nil {
			return err
		}
	}
	for inner := range ffmpegFiles {
		if !found[inner] {
			return fmt.Errorf("%s に %s がありません", filepath.Base(archive), inner)
		}
	}
	return nil
}

func extractEntry(entry *zip.File, dest string) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, source); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// crlf は Windows のメモ帳で読む文を CRLF にする。
func crlf(text string) string {
	return strings.ReplaceAll(text, "\n", "\r\n")
}

// ffmpegReadme は ffmpeg/README.txt で、同梱した FFmpeg の版とソースの入手先を示す。
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

// appReadme は zip の根の README.txt で、展開して実行すること・SmartScreen の通し方・
// データの場所・更新の仕方を示す（contracts §1）。詳しくは running-vv.md の Windows app の節。
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

// zipDir は root のフォルダを、その名前を根に持つ zip として out に書く。
// 途中で失敗しても out に半端な zip を残さない。
func zipDir(root, out string) error {
	temp, err := os.CreateTemp(filepath.Dir(out), filepath.Base(out)+".*.part")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	writer := zip.NewWriter(temp)
	base := filepath.Base(root)
	walkErr := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		zipName := path.Join(base, filepath.ToSlash(rel))
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			header.Name = zipName + "/"
			_, err := writer.CreateHeader(header)
			return err
		}
		header.Name = zipName
		header.Method = zip.Deflate
		dest, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(name)
		if err != nil {
			return err
		}
		defer func() { _ = source.Close() }()
		_, err = io.Copy(dest, source)
		return err
	})
	if walkErr != nil {
		_ = temp.Close()
		return walkErr
	}
	if err := writer.Close(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// CreateTemp は 0600 で作るので、配布物として読める権限に戻す。
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(temp.Name(), out)
}

// currentCommit は今の commit の SHA を返す。
func currentCommit(root string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("今の commit を読めません: %w", err)
	}
	return string(output), nil
}

// buildWindowsApp は SPA を組んだあとに呼ばれ、zip のパスを返す。
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
	archive, err := fetchFFmpeg(client, ffmpegURL(ffmpegVersion), filepath.Join(root, ffmpegCacheDir),
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

// buildDesktopExe は go-winres でアイコンと manifest の .syso を作り、VVMDM.exe を組む。
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
