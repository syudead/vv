package desktop

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 二重起動の判定（specs/037-windows-app/research.md R-6、contracts/windows-app.md §2）。

// InstanceState は AcquireInstance の結果である。
type InstanceState int

const (
	// InstanceAcquired はこのプロセスが唯一の VVMDM として起動してよいことを表す。
	InstanceAcquired InstanceState = iota
	// InstanceActivated は同じセッションの既存のウィンドウを前面に出したことを表す。
	// このプロセスは何も示さずに終わる。
	InstanceActivated
	// InstanceOtherSession は同じ利用者が別のセッションで起動中であることを表す。
	InstanceOtherSession
	// InstanceStillRunning は同じセッションの VVMDM が、待つ間にウィンドウを出さず
	// 終わりもしなかったことを表す。
	InstanceStillRunning
)

// instancePoll は待つ間に既存のウィンドウとミューテックスを確かめ直す間隔である。
const instancePoll = 200 * time.Millisecond

// instanceHandles はプロセスの終わりまで持つミューテックスである。閉じないので、
// プロセスが終わると OS が放す。
var instanceHandles []windows.Handle

// AcquireInstance は、DB を開く前に、同じ利用者が同じデータの置き場で VVMDM を
// 起動していないかを確かめる。
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

	global, existed, err := createMutex(attrs, globalName, true)
	if err != nil {
		return 0, err
	}
	if !existed {
		return InstanceAcquired, holdLocal(attrs, localName, global)
	}

	// 持ち主は先に global、次に local を作るので、その間に来たときのために 1 度
	// だけ確かめ直す。
	sameSession := mutexExists(localName)
	if !sameSession {
		time.Sleep(instancePoll)
		sameSession = mutexExists(localName)
	}
	if !sameSession {
		_ = windows.CloseHandle(global)
		return InstanceOtherSession, nil
	}

	deadline := time.Now().Add(wait)
	for {
		if activateExistingWindow() {
			_ = windows.CloseHandle(global)
			return InstanceActivated, nil
		}
		event, err := windows.WaitForSingleObject(global, uint32(instancePoll/time.Millisecond))
		if err != nil {
			_ = windows.CloseHandle(global)
			return 0, fmt.Errorf("cannot wait for the running VVMDM: %w", err)
		}
		// 前のプロセスは放さずに終わるので、WAIT_ABANDONED も取れたことを表す。
		if event == windows.WAIT_OBJECT_0 || event == windows.WAIT_ABANDONED {
			return InstanceAcquired, holdLocal(attrs, localName, global)
		}
		if time.Now().After(deadline) {
			_ = windows.CloseHandle(global)
			return InstanceStillRunning, nil
		}
	}
}

// holdLocal は取った global と、同じセッションの持ち主であることを示す local を
// プロセスの終わりまで持つ。
func holdLocal(attrs *windows.SecurityAttributes, localName string, global windows.Handle) error {
	instanceHandles = append(instanceHandles, global)
	local, _, err := createMutex(attrs, localName, false)
	if err != nil {
		return err
	}
	instanceHandles = append(instanceHandles, local)
	return nil
}

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
