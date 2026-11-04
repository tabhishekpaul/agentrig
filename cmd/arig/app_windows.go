//go:build windows && cgo

package main

import (
	"syscall"
	"unsafe"

	webview "github.com/webview/webview_go"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	getWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	setWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	setWindowPos     = user32.NewProc("SetWindowPos")
	showWindow       = user32.NewProc("ShowWindow")
	isZoomed         = user32.NewProc("IsZoomed")
	releaseCapture   = user32.NewProc("ReleaseCapture")
	sendMessage      = user32.NewProc("SendMessageW")
	postMessage      = user32.NewProc("PostMessageW")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	consoleProcs     = kernel32.NewProc("GetConsoleProcessList")
	freeConsole      = kernel32.NewProc("FreeConsole")
)

const (
	gwlStyle       = ^uintptr(15) // GWL_STYLE (-16)
	wsCaption      = 0x00C00000
	swMinimize     = 6
	swMaximize     = 3
	swRestore      = 9
	wmClose        = 0x0010
	wmNCLButtonDn  = 0x00A1
	htCaption      = 2
	swpFrameChange = 0x0020 | 0x0002 | 0x0001 | 0x0004 // FRAMECHANGED|NOMOVE|NOSIZE|NOZORDER
)

// window shows the dashboard in a frameless WebView2 window. The page draws
// macOS-style window buttons at the top right, in Windows' order.
func window(url string) error {
	// Started from Explorer or the Start menu, Windows gave arig a console of
	// its own: let it go so only the app window shows. From a terminal (shared
	// console) it stays.
	var ids [2]uint32
	if n, _, _ := consoleProcs.Call(uintptr(unsafe.Pointer(&ids[0])), 2); n == 1 {
		freeConsole.Call()
	}
	w := webview.New(false)
	if w == nil {
		return errNoWindow
	}
	defer w.Destroy()
	w.SetTitle("Agentrig")
	w.SetSize(1320, 860, webview.HintNone)
	hwnd := uintptr(w.Window())
	// Drop the title bar but keep the resizable border.
	style, _, _ := getWindowLongPtr.Call(hwnd, gwlStyle)
	setWindowLongPtr.Call(hwnd, gwlStyle, style&^wsCaption)
	setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpFrameChange)

	_ = w.Bind("arigMinimize", func() { showWindow.Call(hwnd, swMinimize) })
	_ = w.Bind("arigMaximize", func() {
		if z, _, _ := isZoomed.Call(hwnd); z != 0 {
			showWindow.Call(hwnd, swRestore)
		} else {
			showWindow.Call(hwnd, swMaximize)
		}
	})
	_ = w.Bind("arigClose", func() { postMessage.Call(hwnd, wmClose, 0, 0) })
	// Hand the press to Windows as a title-bar press: it moves the window.
	_ = w.Bind("arigDragStart", func() {
		releaseCapture.Call()
		sendMessage.Call(hwnd, wmNCLButtonDn, htCaption, 0)
	})
	_ = w.Bind("arigOpen", openExternal)
	w.Init(externalLinks + customChrome)
	w.Navigate(url)
	w.Run()
	return nil
}
