// Package desktop adapts Windows UI, WebView2, dialogs, process jobs, and data paths without other
// internal dependencies. Windows calls live in _windows.go files; pure argument/path logic remains
// testable on all platforms.
package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const AppName = "VVMDM"

const DefaultPort = 47880

const UsageText = "Usage: VVMDM.exe [--port <1-65535>]"

// ParseArgs excludes the program name, defaults to DefaultPort, and accepts only --port values from
// 1 through 65535.
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

type Paths struct {
	// Root is %LOCALAPPDATA%\\VVMDM.
	Root string
	// Data corresponds to MDM_DATA_DIR, containing mdm.db and thumbnails.
	Data string
	// WebView2 stores cookies and localStorage.
	WebView2 string
	Logs     string
	// LogFile is the current run; PreviousLogFile is the prior run.
	LogFile         string
	PreviousLogFile string
}

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

// UnwritableDirError identifies the directory shown in the failure dialog.
type UnwritableDirError struct {
	Dir string
	Err error
}

func (e *UnwritableDirError) Error() string {
	return fmt.Sprintf("cannot write to %s: %v", e.Dir, e.Err)
}

func (e *UnwritableDirError) Unwrap() error { return e.Err }

const dirPerm os.FileMode = 0o755

// PrepareDirs returns *UnwritableDirError identifying any unwritable directory.
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

// OpenLog rotates via rename without first deleting the previous log, preserving it if the current
// log cannot be moved, including on Windows.
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

func FFmpegDir(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), "ffmpeg")
}

// ErrNotExtracted indicates temporary-directory execution or a missing bundled ffmpeg.
var ErrNotExtracted = errors.New("VVMDM.exe is not running from an extracted folder")

// CheckBundle requires adjacent ffmpeg.exe and ffprobe.exe; an empty tempDir skips the
// temporary-directory check.
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

// Compare case-insensitively to match Windows paths.
func isUnder(path, dir string) bool {
	path, dir = filepath.Clean(path), filepath.Clean(dir)
	rel, err := filepath.Rel(strings.ToLower(dir), strings.ToLower(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func PrependPath(dir, current string) string {
	if current == "" {
		return dir
	}
	return dir + string(os.PathListSeparator) + current
}

// InstanceNames hashes user SID and case-insensitive data path; global detects duplicates across
// sessions, while local identifies same-session owners without blocking other users.
func InstanceNames(sid, root string) (global, local string) {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	suffix := AppName + "-" + sid + "-" + hex.EncodeToString(sum[:8])
	return `Global\` + suffix, `Local\` + suffix
}
