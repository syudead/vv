package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// golang.org/x/sys/windows に無い user32 と kernel32 の関数と型。

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procGetWindowLongPtrW    = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW    = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos         = user32.NewProc("SetWindowPos")
	procGetWindowPlacement   = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement   = user32.NewProc("SetWindowPlacement")
	procMonitorFromWindow    = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW      = user32.NewProc("GetMonitorInfoW")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procFindWindowW          = user32.NewProc("FindWindowW")
	procIsIconic             = user32.NewProc("IsIconic")

	procShutdownBlockReasonCreate  = user32.NewProc("ShutdownBlockReasonCreate")
	procShutdownBlockReasonDestroy = user32.NewProc("ShutdownBlockReasonDestroy")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsPopup            = 0x80000000
	wsVisible          = 0x10000000

	// cwUseDefault は CW_USEDEFAULT（int の 0x80000000）。
	cwUseDefault int32 = -0x80000000

	swHide       = 0
	swShowNormal = 1
	swRestore    = 9

	wmDestroy  = 0x0002
	wmMove     = 0x0003
	wmSize     = 0x0005
	wmActivate = 0x0006
	wmClose    = 0x0010

	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmApp             = 0x8000

	waInactive = 0

	idcArrow = 32512

	spiGetWorkArea = 0x0030

	monitorDefaultToNearest = 0x00000002

	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpFrameChanged  = 0x0020
	swpNoOwnerZOrder = 0x0200

	// gwlStyle は GWL_STYLE（-16）。
	gwlStyle = ^uintptr(15)

	// defaultDPI は拡大していない画面の DPI である。
	defaultDPI = 96
)

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type windowPlacement struct {
	Length         uint32
	Flags          uint32
	ShowCmd        uint32
	PtMinPosition  point
	PtMaxPosition  point
	RcNormalPosion rect
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

func moduleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}

// systemDPI は画面の DPI を返す。Windows 10 1607 より前は既定の 96 とする。
func systemDPI() int32 {
	if procGetDpiForSystem.Find() != nil {
		return defaultDPI
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return defaultDPI
	}
	return int32(dpi)
}

// workArea はタスクバーを除いた主画面の領域を返す。
func workArea() (rect, bool) {
	var r rect
	ok, _, _ := procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0)
	return r, ok != 0
}

func postMessage(hwnd uintptr, message uint32, wparam, lparam uintptr) {
	_, _, _ = procPostMessageW.Call(hwnd, uintptr(message), wparam, lparam)
}
