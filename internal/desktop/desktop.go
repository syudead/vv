// Package desktop は Windows デスクトップ版（VVMDM.exe）の OS 側を扱う adapter である。
// Win32 のウィンドウ、WebView2 の埋め込み、ダイアログ、ジョブオブジェクトと、
// データの置き場の解決を持つ（specs/037-windows-app/research.md R-1〜R-5、R-10、R-11）。
//
// Windows の実装は _windows.go にあり、cmd/mdm の desktop タグのファイルだけが読む。
// このファイルの引数の読み取りとデータの置き場の解決は OS に依らない純粋な手順で、
// どの OS でも試験できる。他の internal/* は読まない。
package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AppName はウィンドウの題名・ダイアログの題名・データの置き場のフォルダ名である
// （specs/037-windows-app/contracts/windows-app.md）。
const AppName = "VVMDM"

// DefaultPort は引数が無いときに待ち受けるポートである（research.md R-4）。
const DefaultPort = 47880

// UsageText は使えない引数のダイアログに添える使い方である。
const UsageText = "Usage: VVMDM.exe [--port <1-65535>]"

// ParseArgs は起動の引数（プログラム名を除く）を読み、待ち受けるポートを返す。
// 引数なしは DefaultPort、`--port <1-65535>`（`--port=<番号>` も可）はその番号で、
// それ以外は誤りを返す（contracts/windows-app.md §2）。
func ParseArgs(args []string) (int, error) {
	switch {
	case len(args) == 0:
		return DefaultPort, nil
	case len(args) == 1 && strings.HasPrefix(args[0], "--port="):
		return parsePort(strings.TrimPrefix(args[0], "--port="))
	case len(args) == 2 && args[0] == "--port":
		return parsePort(args[1])
	default:
		return 0, fmt.Errorf("unsupported arguments: %s", strings.Join(args, " "))
	}
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("--port must be a number from 1 to 65535, not %q", value)
	}
	return port, nil
}

// Paths はデスクトップ版のデータの置き場である（contracts/windows-app.md §3）。
type Paths struct {
	// Root は %LOCALAPPDATA%\VVMDM。
	Root string
	// Data は MDM_DATA_DIR に当たる置き場（mdm.db と thumbnails\）。
	Data string
	// WebView2 は WebView2 の利用者データ（Cookie・localStorage）。
	WebView2 string
	// Logs はログの置き場。
	Logs string
	// LogFile は今回のログ、PreviousLogFile は前回のログである。
	LogFile         string
	PreviousLogFile string
}

// ResolvePaths は %LOCALAPPDATA% のパスからデータの置き場を決める。
func ResolvePaths(localAppData string) (Paths, error) {
	if localAppData == "" || !filepath.IsAbs(localAppData) {
		return Paths{}, fmt.Errorf("the local application data folder %q is not an absolute path", localAppData)
	}
	root := filepath.Join(localAppData, AppName)
	logs := filepath.Join(root, "logs")
	return Paths{
		Root:            root,
		Data:            filepath.Join(root, "data"),
		WebView2:        filepath.Join(root, "webview2"),
		Logs:            logs,
		LogFile:         filepath.Join(logs, "vvmdm.log"),
		PreviousLogFile: filepath.Join(logs, "vvmdm.1.log"),
	}, nil
}

// UnwritableDirError はデータの置き場の下に書けなかったことを表す。ダイアログは
// Dir を示す（contracts/windows-app.md §2 の 2 番目）。
type UnwritableDirError struct {
	Dir string
	Err error
}

func (e *UnwritableDirError) Error() string {
	return fmt.Sprintf("cannot write to %s: %v", e.Dir, e.Err)
}

func (e *UnwritableDirError) Unwrap() error { return e.Err }

// dirPerm はデータの置き場を作るときの許可属性である。
const dirPerm os.FileMode = 0o755

// PrepareDirs はデータの置き場のフォルダを作り、それぞれに書けることを確かめる。
// 書けなかったら、そのフォルダを持つ *UnwritableDirError を返す。
func PrepareDirs(paths Paths) error {
	for _, dir := range []string{paths.Root, paths.Data, paths.WebView2, paths.Logs} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return &UnwritableDirError{Dir: dir, Err: err}
		}
		probe, err := os.CreateTemp(dir, ".write-check-*")
		if err != nil {
			return &UnwritableDirError{Dir: dir, Err: err}
		}
		name := probe.Name()
		closeErr := probe.Close()
		removeErr := os.Remove(name)
		if err := errors.Join(closeErr, removeErr); err != nil {
			return &UnwritableDirError{Dir: dir, Err: err}
		}
	}
	return nil
}

// OpenLog は前回のログを PreviousLogFile へ移し、今回のログを新しく開く（research.md R-10）。
// 前々回のログは消える。
//
// 前々回のログは先に消さず、rename で置き換える（Go の os.Rename は Windows でも
// 移し先を置き換える）。今回のログを移せなかったとき（別のプロセスが開いたままの
// ときなど）は、前回のログがそのまま残る。
func OpenLog(paths Paths) (*os.File, error) {
	if _, err := os.Stat(paths.LogFile); err == nil {
		if err := os.Rename(paths.LogFile, paths.PreviousLogFile); err != nil {
			return nil, fmt.Errorf("cannot keep the previous log: %w", err)
		}
	}
	file, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cannot open the log: %w", err)
	}
	return file, nil
}

// FFmpegDir は exe の隣の同梱の ffmpeg の置き場である（research.md R-11）。
func FFmpegDir(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), "ffmpeg")
}

// ErrNotExtracted は exe が zip から直接（一時フォルダの下で）実行されたか、
// 隣に同梱の ffmpeg が無いことを表す（contracts/windows-app.md §2 の 1 番目）。
var ErrNotExtracted = errors.New("VVMDM.exe is not running from an extracted folder")

// CheckBundle は exe が一時フォルダの下になく、隣に ffmpeg\ffmpeg.exe と
// ffmpeg\ffprobe.exe があることを確かめる。tempDir が空なら一時フォルダの判定は省く。
func CheckBundle(exePath, tempDir string, exists func(path string) bool) error {
	if tempDir != "" && isUnder(exePath, tempDir) {
		return fmt.Errorf("%w: %s is under the temporary folder %s", ErrNotExtracted, exePath, tempDir)
	}
	dir := FFmpegDir(exePath)
	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		if path := filepath.Join(dir, name); !exists(path) {
			return fmt.Errorf("%w: %s is missing", ErrNotExtracted, path)
		}
	}
	return nil
}

// isUnder は path が dir の下にあるかを、大文字と小文字を区別せずに判定する
// （Windows のパスは大文字と小文字を区別しない）。
func isUnder(path, dir string) bool {
	path, dir = filepath.Clean(path), filepath.Clean(dir)
	rel, err := filepath.Rel(strings.ToLower(dir), strings.ToLower(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// PrependPath は PATH の値の先頭に dir を足した値を返す。
func PrependPath(dir, current string) string {
	if current == "" {
		return dir
	}
	return dir + string(os.PathListSeparator) + current
}
