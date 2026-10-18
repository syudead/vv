package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

// LocalAppData uses the known-folder API rather than an environment variable.
func LocalAppData() (string, error) {
	path, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", fmt.Errorf("cannot find the local application data folder: %w", err)
	}
	return path, nil
}

// Expand 8.3 temporary paths before comparing against the executable path.
func TempDir() string {
	dir := filepath.Clean(os.TempDir())
	long, err := longPathName(dir)
	if err != nil {
		return dir
	}
	return long
}

func longPathName(path string) (string, error) {
	short, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetLongPathName(short, &buf[0], uint32(len(buf)))
		if err != nil {
			return "", err
		}
		if int(n) <= len(buf) {
			return windows.UTF16ToString(buf[:n]), nil
		}
		buf = make([]uint16, n)
	}
}

func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

var ErrNoWebView2 = errors.New("the Microsoft Edge WebView2 Runtime is not installed")

// WebView2Version returns ErrNoWebView2 when the runtime is absent.
func WebView2Version() (string, error) {
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil {
		return "", fmt.Errorf("cannot find the WebView2 Runtime: %w", err)
	}
	if version == "" {
		return "", ErrNoWebView2
	}
	return version, nil
}

// ShowError blocks until the standard Windows dialog closes.
func ShowError(text string) {
	title, _ := windows.UTF16PtrFromString(AppName)
	body, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, body, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}

// Keep the kill-on-close job handle until process exit so forced termination also kills
// ffmpeg/ffprobe children; closing it earlier would kill the application too.
func JoinKillOnCloseJob() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("cannot create a job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("cannot set the job object limits: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("cannot join the job object: %w", err)
	}
	return nil
}
