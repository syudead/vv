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

// ClassName はウィンドウクラス名である。二重起動の検出に使う
// （specs/037-windows-app/contracts/windows-app.md §4）。
const ClassName = "VVMDMWindow"

// 初期のウィンドウの大きさ（96 DPI での画素数）。画面より大きければ収める。
const (
	initialWidth  = 1280
	initialHeight = 800
)

// wmAppCall は Post で積んだ関数を UI のスレッドで呼ぶ合図である。
const wmAppCall = wmApp + 1

// WindowOptions は Window を作るときに渡すものである。
type WindowOptions struct {
	// URL は WebView2 が開くアドレス（http://localhost:<ポート>/）。
	URL string
	// DataPath は WebView2 の利用者データの置き場。
	DataPath string
	// ShouldConfirmClose は利用者がウィンドウを閉じようとしたときに UI のスレッドで
	// 呼び、真なら閉じる前の確認を出す（research.md R-7）。nil なら確認しない。
	ShouldConfirmClose func() bool
	// OnClose は利用者がウィンドウを閉じると決めたときに UI のスレッドで呼ぶ。
	// ウィンドウは先に隠れる。ウィンドウを壊すのは Destroy で、呼び出し元が停止を
	// 終えてから呼ぶ。
	OnClose func()
	// OnEndSession はサインアウト・シャットダウンが確定したとき（WM_ENDSESSION）に
	// UI のスレッドで呼ぶ。停止を終えるまで戻らない（research.md R-8）。
	OnEndSession func()
	// OnFatal は WebView2 が続けられない誤りを起こしたときに UI のスレッドで呼ぶ。
	// 戻るとプロセスは終わる。
	OnFatal func(error)
	Logger  *slog.Logger
}

// Window は WebView2 を埋め込んだ自前の Win32 ウィンドウである（research.md R-2）。
// NewWindow・Run・Post 以外の操作は UI のスレッドで行う。
type Window struct {
	opts     WindowOptions
	hwnd     uintptr
	chromium *edge.Chromium
	ready    bool
	closing  bool
	// confirming は閉じる前の確認を出している間は真で、重ねて出さない。
	confirming bool
	// endingSession はサインアウト・シャットダウンの停止を始めたら真である。
	endingSession bool

	fullscreen      bool
	savedStyle      uintptr
	savedPlacement  windowPlacement
	pendingMu       sync.Mutex
	pending         []func()
	destroyed       bool
	destroyedSignal chan struct{}
}

// 1 つのプロセスにウィンドウは 1 つだけで、ウィンドウ手続きはこれを使う。
var (
	current     *Window
	wndProcAddr = windows.NewCallback(wndProc)
)

// NewWindow はウィンドウを作って WebView2 を埋め込み、URL を開いて表に出す。
// 呼び出し元は runtime.LockOSThread で固定したスレッド（UI のスレッド）で呼び、
// 同じスレッドで Run を呼ぶ。
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
	// Embed は WebView2 の用意ができるまでメッセージを回す。
	if !chromium.Embed(hwnd) {
		return nil, errors.New("cannot embed WebView2")
	}
	w.ready = true

	_, _, _ = procShowWindow.Call(hwnd, swShowNormal)
	_, _, _ = procUpdateWindow.Call(hwnd)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	chromium.Resize()
	chromium.Navigate(opts.URL)
	w.focusWebView()
	return w, nil
}

// initialBounds は初期の位置と大きさを返す。DPI に合わせて広げ、主画面の作業領域に
// 収めて中央に置く。作業領域が分からなければ位置は Windows に任せる。
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

// Run はウィンドウが壊れるまでメッセージを回す。NewWindow と同じスレッドで呼ぶ。
func (w *Window) Run() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		// 0 は WM_QUIT、-1 は誤り。どちらでも回すのをやめる。
		if r == 0 || int32(r) == -1 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Post は fn を UI のスレッドで呼ぶよう積む。どのスレッドからでも呼べる。
// ウィンドウが壊れたあとは何もしない。
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

// Destroy はウィンドウを壊し、Run を終わらせる。どのスレッドからでも呼べる。
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
		// 終了は拒まず、停止を待つ理由を Windows に示す（research.md R-8）。
		w.setShutdownBlockReason(true)
		return 1
	case wmEndSession:
		if wparam != 0 && !w.endingSession {
			// 確認は出さない。今の停止手順を最後まで通してから戻る。
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

// requestClose は閉じる操作を扱う。取り込みの途中なら確認を出し、「続ける」なら
// 何もしない。閉じるなら、反応がすぐ見えるよう先にウィンドウを隠し、停止を終えた
// 呼び出し元が Destroy で壊す（research.md R-7）。
func (w *Window) requestClose() {
	if w.closing || w.confirming {
		return
	}
	if w.opts.ShouldConfirmClose != nil {
		w.confirming = true
		ask := w.opts.ShouldConfirmClose()
		closeNow := !ask || confirmClose(w.hwnd)
		w.confirming = false
		// 確認の間にサインアウトの停止が始まっていたら、そちらに任せる。
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

// setShutdownBlockReason はサインアウト・シャットダウンで停止を待つ理由を
// 登録するか消す。
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

// fullScreenChanged は動画の全画面をウィンドウの全画面（枠なし・画面いっぱい）に
// 切り替え、抜けたら元の位置と大きさに戻す（contracts/windows-app.md §4）。
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

// processFailed は描画プロセスが落ちたり応答しなくなったりしたら、今のページを
// 読み直す。ブラウザのプロセスが終わると WebView2 は使えないので、続けられない
// 誤りとして扱う。
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

// fatal は WebView2 の続けられない誤りを呼び出し元へ渡す。go-webview2 はこの
// あとプロセスを終わらせることがあるので、呼び出し元は戻る前に停止を済ませる。
func (w *Window) fatal(err error) {
	w.opts.Logger.Error("WebView2 failed", slog.Any("error", err))
	if w.opts.OnFatal != nil {
		w.opts.OnFatal(err)
	}
}

// LockUIThread は呼んだ goroutine を今の OS のスレッドに固定する。ウィンドウと
// WebView2 は作ったスレッドでしか扱えない。
func LockUIThread() {
	runtime.LockOSThread()
}
