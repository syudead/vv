package desktop

import (
	"encoding/binary"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 閉じる前の確認のダイアログ（research.md R-7、contracts/windows-app.md §4）。
// Windows 標準の TaskDialog で「閉じる」「続ける」に当たる 2 つのボタンを出す。
// TaskDialog は Common Controls v6 にしか無く、v6 は exe の manifest で選ぶ。
// manifest の無いビルドでは、はい・いいえの MessageBox で同じことを問う。

var (
	comctl32                = windows.NewLazySystemDLL("comctl32.dll")
	procTaskDialogIndirect  = comctl32.NewProc("TaskDialogIndirect")
	closeConfirmCloseID     = int32(100)
	closeConfirmContinueID  = int32(101)
	taskDialogWarningIcon   = uintptr(0xFFFF) // TD_WARNING_ICON（MAKEINTRESOURCEW(-1)）
	taskDialogAllowCancel   = uint32(0x0008)  // TDF_ALLOW_DIALOG_CANCELLATION
	taskDialogRelativeToWnd = uint32(0x1000)  // TDF_POSITION_RELATIVE_TO_WINDOW
)

// TASKDIALOGCONFIG と TASKDIALOG_BUTTON は commctrl.h で 1 バイト境界に詰めて
// 並ぶ（pshpack1.h）。Go の構造体では表せないので、64 ビットの並びのとおりに
// バイト列へ書く。
const (
	taskDialogConfigSize = 160
	taskDialogButtonSize = 12

	tdcCbSize          = 0
	tdcHwndParent      = 4
	tdcHInstance       = 12
	tdcFlags           = 20
	tdcWindowTitle     = 28
	tdcMainIcon        = 36
	tdcMainInstruction = 44
	tdcContent         = 52
	tdcButtonCount     = 60
	tdcButtons         = 64
	tdcDefaultButton   = 72
	tdbButtonID        = 0
	tdbButtonText      = 4
	messageBoxYes      = 6 // IDYES
)

// confirmClose は閉じる前の確認を owner の上に出し、「閉じる」を選んだかを返す。
// Esc やダイアログの閉じるボタンは「続ける」として扱う。UI のスレッドで呼ぶ。
func confirmClose(owner uintptr) bool {
	if closed, ok := confirmCloseWithTaskDialog(owner); ok {
		return closed
	}
	title, _ := windows.UTF16PtrFromString(AppName)
	body, err := windows.UTF16PtrFromString(CloseConfirmInstruction + ".\n\n" +
		CloseConfirmContent + "\n\n" + CloseConfirmFallbackQuestion)
	if err != nil {
		return true
	}
	answer, _ := windows.MessageBox(windows.HWND(owner), body, title,
		windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_DEFBUTTON2)
	return answer == messageBoxYes
}

// confirmCloseWithTaskDialog は TaskDialog で問う。TaskDialog を使えなかったら
// ok は偽である。
func confirmCloseWithTaskDialog(owner uintptr) (closed, ok bool) {
	if procTaskDialogIndirect.Find() != nil {
		return false, false
	}
	title, err1 := windows.UTF16PtrFromString(AppName)
	instruction, err2 := windows.UTF16PtrFromString(CloseConfirmInstruction)
	content, err3 := windows.UTF16PtrFromString(CloseConfirmContent)
	closeText, err4 := windows.UTF16PtrFromString(CloseConfirmClose)
	continueText, err5 := windows.UTF16PtrFromString(CloseConfirmContinue)
	for _, err := range []error{err1, err2, err3, err4, err5} {
		if err != nil {
			return false, false
		}
	}

	buttons := make([]byte, 2*taskDialogButtonSize)
	putButton(buttons[0:], closeConfirmCloseID, closeText)
	putButton(buttons[taskDialogButtonSize:], closeConfirmContinueID, continueText)

	config := make([]byte, taskDialogConfigSize)
	le := binary.LittleEndian
	le.PutUint32(config[tdcCbSize:], taskDialogConfigSize)
	le.PutUint64(config[tdcHwndParent:], uint64(owner))
	le.PutUint64(config[tdcHInstance:], uint64(moduleHandle()))
	le.PutUint32(config[tdcFlags:], taskDialogAllowCancel|taskDialogRelativeToWnd)
	le.PutUint64(config[tdcWindowTitle:], uint64(uintptr(unsafe.Pointer(title))))
	le.PutUint64(config[tdcMainIcon:], uint64(taskDialogWarningIcon))
	le.PutUint64(config[tdcMainInstruction:], uint64(uintptr(unsafe.Pointer(instruction))))
	le.PutUint64(config[tdcContent:], uint64(uintptr(unsafe.Pointer(content))))
	le.PutUint32(config[tdcButtonCount:], 2)
	le.PutUint64(config[tdcButtons:], uint64(uintptr(unsafe.Pointer(&buttons[0]))))
	le.PutUint32(config[tdcDefaultButton:], uint32(closeConfirmContinueID))

	var pressed int32
	hr, _, _ := procTaskDialogIndirect.Call(
		uintptr(unsafe.Pointer(&config[0])),
		uintptr(unsafe.Pointer(&pressed)),
		0, 0,
	)
	// バイト列に書いたポインタは GC から見えないので、呼び出しが戻るまで保つ。
	runtime.KeepAlive(title)
	runtime.KeepAlive(instruction)
	runtime.KeepAlive(content)
	runtime.KeepAlive(closeText)
	runtime.KeepAlive(continueText)
	runtime.KeepAlive(buttons)
	if int32(hr) < 0 {
		return false, false
	}
	return pressed == closeConfirmCloseID, true
}

func putButton(dst []byte, id int32, text *uint16) {
	binary.LittleEndian.PutUint32(dst[tdbButtonID:], uint32(id))
	binary.LittleEndian.PutUint64(dst[tdbButtonText:], uint64(uintptr(unsafe.Pointer(text))))
}
