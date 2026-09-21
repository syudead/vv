//go:build windows

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x00002000
	processTerminate                  = 0x0001
	processSetQuota                   = 0x0100
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	createJobObject          = kernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	openProcess              = kernel32.NewProc("OpenProcess")
	closeHandle              = kernel32.NewProc("CloseHandle")
)

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
	handle uintptr
	once   sync.Once
}

func newProcessGroup(*exec.Cmd) (*processGroup, error) {
	handle, _, callErr := createJobObject.Call(0, 0)
	if handle == 0 {
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
		return nil, windowsCallError("SetInformationJobObject", callErr)
	}
	return &processGroup{handle: handle}, nil
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
	return nil
}

// terminate は Job Object を閉じる。起動元が先に終了していても、同じ Job に
// 属する MDM・Vite などの子孫は KILL_ON_JOB_CLOSE によりすべて終了する。
func (g *processGroup) terminate(*exec.Cmd) {
	g.once.Do(func() { _, _, _ = closeHandle.Call(g.handle) })
}

func windowsCallError(name string, err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == 0 {
		err = syscall.EINVAL
	}
	return fmt.Errorf("%s: %w", name, err)
}
