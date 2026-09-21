//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x00002000
	processTerminate                  = 0x0001
	processSetQuota                   = 0x0100
	synchronize                       = 0x00100000
	infinite                          = 0xffffffff
	waitObject0                       = 0
)

const (
	launcherPayloadEnv = "VV_DEV_LAUNCHER_PAYLOAD"
	launcherEventEnv   = "VV_DEV_LAUNCHER_EVENT"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	createJobObject          = kernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	openProcess              = kernel32.NewProc("OpenProcess")
	createEvent              = kernel32.NewProc("CreateEventW")
	openEvent                = kernel32.NewProc("OpenEventW")
	setEvent                 = kernel32.NewProc("SetEvent")
	waitForSingleObject      = kernel32.NewProc("WaitForSingleObject")
	closeHandle              = kernel32.NewProc("CloseHandle")
)

type launcherPayload struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
}

type ioCounters struct {
	readOperationCount  uint64
	writeOperationCount uint64
	otherOperationCount uint64
	readTransferCount   uint64
	writeTransferCount  uint64
	otherTransferCount  uint64
}

type basicLimitInformation struct {
	perProcessUserTimeLimit int64
	perJobUserTimeLimit     int64
	limitFlags              uint32
	minimumWorkingSetSize   uintptr
	maximumWorkingSetSize   uintptr
	activeProcessLimit      uint32
	affinity                uintptr
	priorityClass           uint32
	schedulingClass         uint32
}

type extendedLimitInformation struct {
	basicLimitInformation basicLimitInformation
	ioInfo                ioCounters
	processMemoryLimit    uintptr
	jobMemoryLimit        uintptr
	peakProcessMemoryUsed uintptr
	peakJobMemoryUsed     uintptr
}

type processGroup struct {
	handle      uintptr
	readyHandle uintptr
	once        sync.Once
}

func init() {
	if os.Getenv(launcherPayloadEnv) != "" {
		os.Exit(runWindowsLauncher())
	}
}

func newProcessGroup(cmd *exec.Cmd) (*processGroup, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("ランチャーの実行ファイルを取得できません: %w", err)
	}
	payload, err := json.Marshal(launcherPayload{Path: cmd.Path, Args: cmd.Args[1:]})
	if err != nil {
		return nil, fmt.Errorf("起動情報を作成できません: %w", err)
	}
	eventName := fmt.Sprintf("Local\\vv-dev-%d-%d", os.Getpid(), time.Now().UnixNano())
	eventNamePtr, err := syscall.UTF16PtrFromString(eventName)
	if err != nil {
		return nil, fmt.Errorf("起動イベント名を作成できません: %w", err)
	}
	readyHandle, _, callErr := createEvent.Call(0, 1, 0, uintptr(unsafe.Pointer(eventNamePtr)))
	if readyHandle == 0 {
		return nil, windowsCallError("CreateEventW", callErr)
	}

	handle, _, callErr := createJobObject.Call(0, 0)
	if handle == 0 {
		_, _, _ = closeHandle.Call(readyHandle)
		return nil, windowsCallError("CreateJobObjectW", callErr)
	}
	info := extendedLimitInformation{}
	info.basicLimitInformation.limitFlags = jobObjectLimitKillOnJobClose
	result, _, callErr := setInformationJobObject.Call(
		handle,
		jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if result == 0 {
		_, _, _ = closeHandle.Call(handle)
		_, _, _ = closeHandle.Call(readyHandle)
		return nil, windowsCallError("SetInformationJobObject", callErr)
	}

	cmd.Path = executable
	cmd.Args = []string{executable}
	cmd.Env = append(withoutLauncherEnv(cmd.Env),
		launcherPayloadEnv+"="+base64.RawURLEncoding.EncodeToString(payload),
		launcherEventEnv+"="+eventName,
	)
	return &processGroup{handle: handle, readyHandle: readyHandle}, nil
}

func (g *processGroup) attach(cmd *exec.Cmd) error {
	process, _, callErr := openProcess.Call(processTerminate|processSetQuota, 0, uintptr(cmd.Process.Pid))
	if process == 0 {
		return windowsCallError("OpenProcess", callErr)
	}
	defer func() { _, _, _ = closeHandle.Call(process) }()
	result, _, callErr := assignProcessToJobObject.Call(g.handle, process)
	if result == 0 {
		return windowsCallError("AssignProcessToJobObject", callErr)
	}
	result, _, callErr = setEvent.Call(g.readyHandle)
	if result == 0 {
		return windowsCallError("SetEvent", callErr)
	}
	return nil
}

// terminate は Job Object を閉じる。起動元が先に終了していても、同じ Job に
// 属する MDM・Vite などの子孫は KILL_ON_JOB_CLOSE によりすべて終了する。
func (g *processGroup) terminate(*exec.Cmd) {
	g.once.Do(func() {
		_, _, _ = closeHandle.Call(g.handle)
		_, _, _ = closeHandle.Call(g.readyHandle)
	})
}

func runWindowsLauncher() int {
	payloadBytes, err := base64.RawURLEncoding.DecodeString(os.Getenv(launcherPayloadEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "開発サーバーの起動情報を読めません:", err)
		return 125
	}
	var payload launcherPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		fmt.Fprintln(os.Stderr, "開発サーバーの起動情報を読めません:", err)
		return 125
	}
	eventName, err := syscall.UTF16PtrFromString(os.Getenv(launcherEventEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "開発サーバーの起動イベント名を読めません:", err)
		return 125
	}
	event, _, callErr := openEvent.Call(synchronize, 0, uintptr(unsafe.Pointer(eventName)))
	if event == 0 {
		fmt.Fprintln(os.Stderr, windowsCallError("OpenEventW", callErr))
		return 125
	}
	defer func() { _, _, _ = closeHandle.Call(event) }()
	result, _, callErr := waitForSingleObject.Call(event, infinite)
	if result != waitObject0 {
		fmt.Fprintln(os.Stderr, windowsCallError("WaitForSingleObject", callErr))
		return 125
	}

	cmd := exec.Command(payload.Path, payload.Args...)
	cmd.Env = withoutLauncherEnv(os.Environ())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "開発サーバーを起動できません:", err)
		return 125
	}
	return 0
}

func withoutLauncherEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, launcherPayloadEnv) || strings.EqualFold(name, launcherEventEnv) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func windowsCallError(name string, err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == 0 {
		err = syscall.EINVAL
	}
	return fmt.Errorf("%s: %w", name, err)
}
