package desktop

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// Also used to detect existing instances.
const ClassName = "VVMDMWindow"

// Initial dimensions at 96 DPI, capped to the screen.
const (
	initialWidth  = 1280
	initialHeight = 800
)

const wmAppCall = wmApp + 1

type WindowOptions struct {
	// Loopback URL, including the listener port.
	URL      string
	DataPath string
	// Called on the UI thread; nil skips confirmation.
	ShouldConfirmClose func() bool
	// Called on the UI thread after hiding the window; the caller invokes Destroy after shutdown
	// finishes.
	OnClose func()
	// Called on the UI thread for WM_ENDSESSION; must not return until shutdown finishes.
	OnEndSession func()
	// Called on the UI thread for fatal WebView2 errors; the process exits after return.
	OnFatal func(error)
	Logger  *slog.Logger
}

// Window embeds WebView2; operations other than NewWindow, Run, and Post require the UI thread.
type Window struct {
	opts     WindowOptions
	hwnd     uintptr
	chromium *edge.Chromium
	ready    bool
	closing  bool
	// Prevent overlapping close confirmations.
	confirming    bool
	endingSession bool

	fullscreen      bool
	savedStyle      uintptr
	savedPlacement  windowPlacement
	pendingMu       sync.Mutex
	pending         []func()
	destroyed       bool
	destroyedSignal chan struct{}
}

// The window procedure uses the process's single window.
var (
	current     *Window
	wndProcAddr = windows.NewCallback(wndProc)
)

// Call NewWindow and Run on the same runtime.LockOSThread-pinned UI thread.
func NewWindow(opts WindowOptions) (*Window, error) {
	if current != nil {
		return nil, errors.New("the window already exists")
	}
	w := &Window{opts: opts, destroyedSignal: make(chan struct{})}
	current = w

	instance := moduleHandle()
	className, err := windows.UTF16PtrFromString(ClassName)
	if err != nil {
		return nil, err
	}
	title, err := windows.UTF16PtrFromString(AppName)
	if err != nil {
		return nil, err
	}
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	class := wndClassEx{
		WndProc:   wndProcAddr,
		Instance:  instance,
		Cursor:    cursor,
		ClassName: className,
	}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return nil, fmt.Errorf("cannot register the window class: %w", callErr)
	}

	x, y, width, height := initialBounds()
	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("cannot create the window: %w", callErr)
	}
	w.hwnd = hwnd

	chromium := edge.NewChromium()
	chromium.DataPath = opts.DataPath
	chromium.SetErrorCallback(w.fatal)
	chromium.ContainsFullScreenElementChangedCallback = w.fullScreenChanged
	chromium.ProcessFailedCallback = w.processFailed
	w.chromium = chromium
	// Embed pumps messages until WebView2 is ready.
	if !chromium.Embed(hwnd) {
		return nil, errors.New("cannot embed WebView2")
	}
	w.ready = true

	_, _, _ = procShowWindow.Call(hwnd, swShowNormal)
	_, _, _ = procUpdateWindow.Call(hwnd)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	// WebView2 was embedded while hidden; explicitly show its drawing child to avoid a blank window.
	if err := chromium.Show(); err != nil {
		opts.Logger.Warn("could not show the WebView2 controller", slog.Any("error", err))
	}
	chromium.Resize()
	chromium.Navigate(opts.URL)
	w.focusWebView()
	return w, nil
}

// Scale by DPI and center within the primary work area; fall back to Windows positioning if
// unavailable.
func initialBounds() (x, y, width, height int32) {
	dpi := systemDPI()
	width = initialWidth * dpi / defaultDPI
	height = initialHeight * dpi / defaultDPI
	area, ok := workArea()
	if !ok {
		return cwUseDefault, cwUseDefault, width, height
	}
	areaWidth, areaHeight := area.Right-area.Left, area.Bottom-area.Top
	width, height = min(width, areaWidth), min(height, areaHeight)
	return area.Left + (areaWidth-width)/2, area.Top + (areaHeight-height)/2, width, height
}

// Run must use the same thread as NewWindow.
func (w *Window) Run() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		// Both WM_QUIT (0) and errors (-1) end the loop.
		if r == 0 || int32(r) == -1 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Post may be called from any thread and ignores calls after destruction.
func (w *Window) Post(fn func()) {
	w.pendingMu.Lock()
	if w.destroyed {
		w.pendingMu.Unlock()
		return
	}
	w.pending = append(w.pending, fn)
	w.pendingMu.Unlock()
	postMessage(w.hwnd, wmAppCall, 0, 0)
}

// Destroy may be called from any thread and ends Run.
func (w *Window) Destroy() {
	w.Post(func() {
		if w.chromium != nil {
			w.chromium.ShuttingDown()
		}
		_, _, _ = procDestroyWindow.Call(w.hwnd)
	})
}

func (w *Window) runPending() {
	w.pendingMu.Lock()
	pending := w.pending
	w.pending = nil
	w.pendingMu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

func wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	w := current
	if w == nil || (w.hwnd != 0 && hwnd != w.hwnd) {
		r, _, _ := procDefWindowProcW.Call(hwnd, message, wparam, lparam)
		return r
	}
	switch uint32(message) {
	case wmSize:
		if w.ready {
			w.chromium.Resize()
		}
		return 0
	case wmMove:
		if w.ready {
			_ = w.chromium.NotifyParentWindowPositionChanged()
		}
		return 0
	case wmActivate:
		if w.ready && wparam&0xFFFF != waInactive {
			w.focusWebView()
		}
	case wmClose:
		w.requestClose()
		return 0
	case wmQueryEndSession:
		// Allow session termination while showing why cleanup must finish first.
		w.setShutdownBlockReason(true)
		return 1
	case wmEndSession:
		if wparam != 0 && !w.endingSession {
			// Skip confirmation and finish shutdown before returning.
			w.endingSession = true
			w.closing = true
			if w.opts.OnEndSession != nil {
				w.opts.OnEndSession()
			}
		}
		w.setShutdownBlockReason(false)
		return 0
	case wmAppCall:
		w.runPending()
		return 0
	case wmDestroy:
		w.pendingMu.Lock()
		w.destroyed = true
		w.pendingMu.Unlock()
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wparam, lparam)
	return r
}

// Hide immediately for visible feedback; the caller destroys the window after shutdown completes.
func (w *Window) requestClose() {
	if w.closing || w.confirming {
		return
	}
	if w.opts.ShouldConfirmClose != nil {
		w.confirming = true
		ask := w.opts.ShouldConfirmClose()
		closeNow := !ask || confirmClose(w.hwnd)
		w.confirming = false
		// A session shutdown begun during confirmation owns cleanup.
		if !closeNow || w.closing {
			return
		}
	}
	w.closing = true
	_, _, _ = procShowWindow.Call(w.hwnd, swHide)
	if w.opts.OnClose != nil {
		w.opts.OnClose()
	}
}

func (w *Window) setShutdownBlockReason(on bool) {
	if !on {
		_, _, _ = procShutdownBlockReasonDestroy.Call(w.hwnd)
		return
	}
	reason, err := windows.UTF16PtrFromString(ShutdownBlockReason)
	if err != nil {
		return
	}
	if ok, _, callErr := procShutdownBlockReasonCreate.Call(w.hwnd, uintptr(unsafe.Pointer(reason))); ok == 0 {
		w.opts.Logger.Warn("could not register the shutdown block reason", slog.Any("error", callErr))
	}
}

func (w *Window) focusWebView() {
	controller := w.chromium.GetController()
	if controller == nil {
		return
	}
	if err := controller.MoveFocus(edge.COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC); err != nil {
		w.opts.Logger.Debug("could not focus the web view", slog.Any("error", err))
	}
}

func (w *Window) fullScreenChanged(sender *edge.ICoreWebView2, _ *edge.ICoreWebView2ContainsFullScreenElementChangedEventArgs) {
	on, err := sender.GetContainsFullScreenElement()
	if err != nil {
		w.opts.Logger.Warn("could not read the full screen state", slog.Any("error", err))
		return
	}
	w.setFullScreen(on)
}

func (w *Window) setFullScreen(on bool) {
	if on == w.fullscreen {
		return
	}
	if on {
		style, _, _ := procGetWindowLongPtrW.Call(w.hwnd, gwlStyle)
		placement := windowPlacement{}
		placement.Length = uint32(unsafe.Sizeof(placement))
		if ok, _, _ := procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&placement))); ok == 0 {
			return
		}
		info := monitorInfo{}
		info.Size = uint32(unsafe.Sizeof(info))
		monitor, _, _ := procMonitorFromWindow.Call(w.hwnd, monitorDefaultToNearest)
		if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
			return
		}
		w.savedStyle, w.savedPlacement = style, placement
		w.fullscreen = true
		_, _, _ = procSetWindowLongPtrW.Call(w.hwnd, gwlStyle, (style&^wsOverlappedWindow)|wsPopup|wsVisible)
		screen := info.Monitor
		_, _, _ = procSetWindowPos.Call(w.hwnd, 0,
			uintptr(screen.Left), uintptr(screen.Top),
			uintptr(screen.Right-screen.Left), uintptr(screen.Bottom-screen.Top),
			swpNoOwnerZOrder|swpFrameChanged)
		return
	}
	w.fullscreen = false
	_, _, _ = procSetWindowLongPtrW.Call(w.hwnd, gwlStyle, w.savedStyle)
	_, _, _ = procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&w.savedPlacement)))
	_, _, _ = procSetWindowPos.Call(w.hwnd, 0, 0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoZOrder|swpNoOwnerZOrder|swpFrameChanged)
}

// Reload renderer failures; browser-process failure makes WebView2 unusable and is fatal.
func (w *Window) processFailed(sender *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
	kind, err := args.GetProcessFailedKind()
	if err != nil {
		w.opts.Logger.Warn("could not read the failed WebView2 process", slog.Any("error", err))
		return
	}
	switch kind {
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_EXITED,
		edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_UNRESPONSIVE:
		source, err := sender.GetSource()
		if err != nil || source == "" {
			source = w.opts.URL
		}
		w.opts.Logger.Warn("the WebView2 render process failed; reloading",
			slog.Int("kind", int(kind)), slog.String("url", source))
		if err := sender.Navigate(source); err != nil {
			w.opts.Logger.Error("could not reload the page", slog.Any("error", err))
		}
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_BROWSER_PROCESS_EXITED:
		w.fatal(errors.New("the WebView2 browser process exited"))
	default:
		w.opts.Logger.Warn("a WebView2 process failed", slog.Int("kind", int(kind)))
	}
}

// The caller must finish shutdown before returning because go-webview2 may terminate the process
// next.
func (w *Window) fatal(err error) {
	w.opts.Logger.Error("WebView2 failed", slog.Any("error", err))
	if w.opts.OnFatal != nil {
		w.opts.OnFatal(err)
	}
}

// Pin the goroutine because Win32 and WebView2 must stay on their creation thread.
func LockUIThread() {
	runtime.LockOSThread()
}
