package desktop

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// instanceHandles はプロセスの終わりまで持つミューテックスである。閉じないので、
// プロセスが終わると OS が放す。
var instanceHandles []uintptr

// AcquireInstance は、DB を開く前に、同じ利用者が同じデータの置き場で VVMDM を
// 起動していないかを確かめる（手順は acquireInstance）。
//
// セッションをまたぐ名前付きミューテックスを、作った利用者だけが開ける DACL で
// 取る。取れなければ、同じセッションの持ち主なら、表に出ているウィンドウを元の
// 大きさに戻して前面に出すか、ミューテックスが放されるのを wait まで待つ（前の
// プロセスが停止の途中でウィンドウを隠しているとき、まだウィンドウを出す前の
// とき）。別のセッションの持ち主なら待たない。呼び出し元は UI のスレッドで呼ぶ
// （ミューテックスは取ったスレッドが持つ）。
func AcquireInstance(root string, wait time.Duration) (InstanceState, error) {
	sid, err := currentUserSID()
	if err != nil {
		return 0, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + sid + ")")
	if err != nil {
		return 0, fmt.Errorf("cannot build the security descriptor: %w", err)
	}
	attrs := &windows.SecurityAttributes{SecurityDescriptor: sd}
	attrs.Length = uint32(unsafe.Sizeof(*attrs))
	globalName, localName := InstanceNames(sid, root)

	state, handles, err := acquireInstance(winInstance{attrs: attrs}, globalName, localName, wait)
	instanceHandles = append(instanceHandles, handles...)
	return state, err
}

// winInstance は instanceOS の Win32 の実装である。
type winInstance struct {
	attrs *windows.SecurityAttributes
}

func (w winInstance) CreateMutex(name string, owner bool) (uintptr, bool, error) {
	handle, existed, err := createMutex(w.attrs, name, owner)
	return uintptr(handle), existed, err
}

func (winInstance) MutexExists(name string) bool { return mutexExists(name) }

func (winInstance) Close(handle uintptr) { _ = windows.CloseHandle(windows.Handle(handle)) }

func (winInstance) Wait(handle uintptr, d time.Duration) (bool, error) {
	event, err := windows.WaitForSingleObject(windows.Handle(handle), uint32(d/time.Millisecond))
	if err != nil {
		return false, err
	}
	// 前のプロセスは放さずに終わるので、WAIT_ABANDONED も取れたことを表す。
	return event == windows.WAIT_OBJECT_0 || event == windows.WAIT_ABANDONED, nil
}

func (winInstance) ActivateWindow() bool { return activateExistingWindow() }

// createMutex は名前付きミューテックスを作るか開く。既にあったら existed は真で、
// owner を求めても持ち主にはならない。
func createMutex(attrs *windows.SecurityAttributes, name string, owner bool) (handle windows.Handle, existed bool, err error) {
	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, false, err
	}
	handle, err = windows.CreateMutex(attrs, owner, name16)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return handle, true, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("cannot create the mutex %s: %w", name, err)
	}
	return handle, false, nil
}

// mutexExists は名前付きミューテックスが今あるかを返す。開けなくても、名前が
// 使われていれば（ERROR_ACCESS_DENIED）あるとみなす。
func mutexExists(name string) bool {
	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name16)
	if err == nil {
		_ = windows.CloseHandle(handle)
		return true
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// activateExistingWindow は同じセッションの VVMDM のウィンドウが表に出ていれば、
// 最小化を元に戻して前面に出し、真を返す。停止の途中で隠れているウィンドウや、
// まだ出ていないウィンドウには何もしない。
func activateExistingWindow() bool {
	className, err := windows.UTF16PtrFromString(ClassName)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 || !windows.IsWindowVisible(windows.HWND(hwnd)) {
		return false
	}
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swRestore)
	}
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	return true
}

// currentUserSID はこのプロセスの利用者の SID を文字列で返す。
func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("cannot read the current user: %w", err)
	}
	return user.User.Sid.String(), nil
}
