//go:build linux

package main

import (
	"os"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// GTK must run on the main OS thread: keep main() there.
func init() { runtime.LockOSThread() }

// GTK 3 / WebKitGTK entry points, loaded at runtime with dlopen so arig needs
// no cgo and still starts where they aren't installed. GObject pointers are
// plain uintptrs; gboolean maps to bool.
var (
	gtkInitCheck             func(argc, argv uintptr) bool
	gtkWindowNew             func(kind int32) uintptr
	gtkWindowSetTitle        func(win uintptr, title string)
	gtkWindowSetDefaultSize  func(win uintptr, w, h int32)
	gtkWindowSetDecorated    func(win uintptr, on bool)
	gtkContainerAdd          func(box, child uintptr)
	gtkWidgetShowAll         func(w uintptr)
	gtkMain                  func()
	gtkMainQuit              func()
	gtkWindowIconify         func(win uintptr)
	gtkWindowMaximize        func(win uintptr)
	gtkWindowUnmaximize      func(win uintptr)
	gtkWindowIsMaximized     func(win uintptr) bool
	gtkWindowClose           func(win uintptr)
	gtkWindowBeginMoveDrag   func(win uintptr, button, rootX, rootY int32, time uint32)
	gSignalConnectData       func(obj uintptr, signal string, cb, data, destroy uintptr, flags int32) uint64
	gFree                    func(p *byte)
	webkitUCMNew             func() uintptr
	webkitUCMRegisterHandler func(mgr uintptr, name string) bool
	webkitUCMAddScript       func(mgr, script uintptr)
	webkitUserScriptNew      func(src string, frames, at int32, allow, block uintptr) uintptr
	webkitWebViewNewWithUCM  func(mgr uintptr) uintptr
	webkitWebViewLoadURI     func(view uintptr, uri string)
	webkitJSResultGetJSValue func(res uintptr) uintptr
	jscValueToString         func(val uintptr) *byte
)

// bridgeJS gives the page the window functions the custom chrome calls; each
// posts {fn, args} to the "arig" script message handler.
const bridgeJS = `(() => {
	const post = (fn, ...args) => window.webkit.messageHandlers.arig.postMessage(JSON.stringify({ fn, args }));
	window.arigMinimize = () => post('minimize');
	window.arigMaximize = () => post('maximize');
	window.arigClose = () => post('close');
	window.arigDragStart = (x, y) => post('dragStart', x, y);
	window.arigOpen = (url) => post('open', url);
})();
`

// window shows the dashboard in a frameless WebKitGTK window; without a
// display, GTK 3 or WebKitGTK it falls back to a Chromium app window.
func window(url string) error {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return chromeApp(url)
	}
	if loadGTK() != nil || !gtkInitCheck(0, 0) {
		return chromeApp(url)
	}
	win := gtkWindowNew(0) // GTK_WINDOW_TOPLEVEL
	gtkWindowSetTitle(win, "Agentrig")
	gtkWindowSetDefaultSize(win, 1320, 860)
	gtkWindowSetDecorated(win, false) // the page draws its own buttons

	// window() runs once per process, so each callback slot is taken once.
	onDestroy := purego.NewCallback(func(w, data uintptr) { gtkMainQuit() })
	onMessage := purego.NewCallback(func(mgr, res, data uintptr) {
		p := jscValueToString(webkitJSResultGetJSValue(res))
		s := cString(p)
		gFree(p)
		c, ok := parseAppCall(s)
		if !ok {
			return
		}
		switch c.fn {
		case "minimize":
			gtkWindowIconify(win)
		case "maximize":
			if gtkWindowIsMaximized(win) {
				gtkWindowUnmaximize(win)
			} else {
				gtkWindowMaximize(win)
			}
		case "close":
			gtkWindowClose(win)
		case "dragStart":
			gtkWindowBeginMoveDrag(win, 1, int32(c.x), int32(c.y), 0) // GDK_CURRENT_TIME
		case "open":
			_ = openExternal(c.url)
		}
	})
	gSignalConnectData(win, "destroy", onDestroy, 0, 0, 0)

	mgr := webkitUCMNew()
	gSignalConnectData(mgr, "script-message-received::arig", onMessage, 0, 0, 0)
	webkitUCMRegisterHandler(mgr, "arig")
	// INJECT_TOP_FRAME (1), INJECT_AT_DOCUMENT_START (0)
	webkitUCMAddScript(mgr, webkitUserScriptNew(bridgeJS+externalLinks+customChrome, 1, 0, 0, 0))

	view := webkitWebViewNewWithUCM(mgr)
	gtkContainerAdd(win, view)
	webkitWebViewLoadURI(view, url)
	gtkWidgetShowAll(win)
	gtkMain()
	return nil
}

// loadGTK opens GTK 3 and WebKitGTK (4.1, else 4.0) and binds the functions
// above. GLib, GObject and JavaScriptCore symbols resolve through their
// dependents.
func loadGTK() error {
	gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	wk, err := purego.Dlopen("libwebkit2gtk-4.1.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		if wk, err = purego.Dlopen("libwebkit2gtk-4.0.so.37", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return err
		}
	}
	return bind([]sym{
		{gtk, &gtkInitCheck, "gtk_init_check"},
		{gtk, &gtkWindowNew, "gtk_window_new"},
		{gtk, &gtkWindowSetTitle, "gtk_window_set_title"},
		{gtk, &gtkWindowSetDefaultSize, "gtk_window_set_default_size"},
		{gtk, &gtkWindowSetDecorated, "gtk_window_set_decorated"},
		{gtk, &gtkContainerAdd, "gtk_container_add"},
		{gtk, &gtkWidgetShowAll, "gtk_widget_show_all"},
		{gtk, &gtkMain, "gtk_main"},
		{gtk, &gtkMainQuit, "gtk_main_quit"},
		{gtk, &gtkWindowIconify, "gtk_window_iconify"},
		{gtk, &gtkWindowMaximize, "gtk_window_maximize"},
		{gtk, &gtkWindowUnmaximize, "gtk_window_unmaximize"},
		{gtk, &gtkWindowIsMaximized, "gtk_window_is_maximized"},
		{gtk, &gtkWindowClose, "gtk_window_close"},
		{gtk, &gtkWindowBeginMoveDrag, "gtk_window_begin_move_drag"},
		{gtk, &gSignalConnectData, "g_signal_connect_data"},
		{gtk, &gFree, "g_free"},
		{wk, &webkitUCMNew, "webkit_user_content_manager_new"},
		{wk, &webkitUCMRegisterHandler, "webkit_user_content_manager_register_script_message_handler"},
		{wk, &webkitUCMAddScript, "webkit_user_content_manager_add_script"},
		{wk, &webkitUserScriptNew, "webkit_user_script_new"},
		{wk, &webkitWebViewNewWithUCM, "webkit_web_view_new_with_user_content_manager"},
		{wk, &webkitWebViewLoadURI, "webkit_web_view_load_uri"},
		{wk, &webkitJSResultGetJSValue, "webkit_javascript_result_get_js_value"},
		{wk, &jscValueToString, "jsc_value_to_string"},
	})
}

type sym struct {
	lib  uintptr
	fn   any // pointer to a func variable
	name string
}

// bind resolves every symbol first (RegisterLibFunc would panic on a missing
// one, e.g. an old WebKitGTK), then registers them.
func bind(syms []sym) error {
	for _, s := range syms {
		p, err := purego.Dlsym(s.lib, s.name)
		if err != nil {
			return err
		}
		purego.RegisterFunc(s.fn, p)
	}
	return nil
}

// cString copies a NUL-terminated C string, at most maxAppMsg+1 bytes (longer
// messages are refused by parseAppCall anyway).
func cString(p *byte) string {
	if p == nil {
		return ""
	}
	var b []byte
	for i := 0; i <= maxAppMsg; i++ {
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}
