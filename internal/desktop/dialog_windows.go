package desktop

import (
	"encoding/binary"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Use Common Controls v6 TaskDialog selected by the executable manifest; fall back to a yes/no
// MessageBox when unavailable.

var (
	comctl32                = windows.NewLazySystemDLL("comctl32.dll")
	procTaskDialogIndirect  = comctl32.NewProc("TaskDialogIndirect")
	closeConfirmCloseID     = int32(100)
	closeConfirmContinueID  = int32(101)
	taskDialogWarningIcon   = uintptr(0xFFFF) // TD_WARNING_ICON（MAKEINTRESOURCEW(-1)）
	taskDialogAllowCancel   = uint32(0x0008)  // TDF_ALLOW_DIALOG_CANCELLATION
	taskDialogRelativeToWnd = uint32(0x1000)  // TDF_POSITION_RELATIVE_TO_WINDOW
)

// Win32 task-dialog structs are packed to one-byte alignment; encode the 64-bit layout manually
// because Go structs cannot express it.
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

// Call on the UI thread; Esc and window-close mean continue.
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

// Returns ok=false when TaskDialog is unavailable.
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
	// Keep pointer targets alive because the GC cannot see pointers encoded in byte slices.
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
