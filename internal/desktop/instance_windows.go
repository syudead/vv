package desktop

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Keep mutex handles until process exit; the OS releases them.
var instanceHandles []uintptr

// AcquireInstance must run on the UI thread before opening the database; a user-only DACL protects
// the cross-session mutex. Activate a visible same-session window or wait up to wait for its owner;
// other sessions fail immediately.
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
	// The previous process exits without releasing, so WAIT_ABANDONED also means ownership was
	// acquired.
	return event == windows.WAIT_OBJECT_0 || event == windows.WAIT_ABANDONED, nil
}

func (winInstance) ActivateWindow() bool { return activateExistingWindow() }

// An existing mutex never grants requested initial ownership.
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

// ERROR_ACCESS_DENIED still proves the mutex name exists.
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

// Restore and activate visible same-session windows; ignore hidden shutdown/startup windows.
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

func currentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("cannot read the current user: %w", err)
	}
	return user.User.Sid.String(), nil
}
