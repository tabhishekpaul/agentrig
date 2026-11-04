//go:build darwin

package main

// The macOS app window in pure Go: AppKit + WebKit through the Objective-C
// runtime via purego, no cgo. The page fills the window; the native window
// buttons sit in the sidebar header, in line with the logo.

import (
	_ "embed"
	"encoding/json"
	"runtime"
	"structs"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

//go:embed assets/icon512.png
var dockIcon []byte

// Cocoa windows must live on the main thread.
func init() { runtime.LockOSThread() }

type nsPoint struct {
	_    structs.HostLayout
	X, Y float64
}
type nsSize struct {
	_    structs.HostLayout
	W, H float64
}
type nsRect struct {
	_      structs.HostLayout
	Origin nsPoint
	Size   nsSize
}

var sel = objc.RegisterName

func cls(name string) objc.ID { return objc.ID(objc.GetClass(name)) }
func nsString(s string) objc.ID {
	return cls("NSString").Send(sel("stringWithUTF8String:"), s)
}
func goString(ns objc.ID) string {
	p := objc.Send[*byte](ns, sel("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

const (
	styleTitled, styleClosable, styleMiniaturizable, styleResizable = 1, 2, 4, 8
	styleFullSizeContent                                            = 1 << 15
	backingBuffered                                                 = 2
	mouseDownMask                                                   = 1 << 1 // NSEventMaskLeftMouseDown
	lightsX, lightsY                                                = 18, 18 // in line with the logo and sidebar icons
)

var overDragArea bool // set by the page as the pointer moves

// window shows the dashboard in a native WebKit window until it's closed.
func window(url string) error {
	for _, f := range []string{"/System/Library/Frameworks/Cocoa.framework/Cocoa", "/System/Library/Frameworks/WebKit.framework/WebKit"} {
		if _, err := purego.Dlopen(f, purego.RTLD_GLOBAL|purego.RTLD_LAZY); err != nil {
			return chromeApp(url)
		}
	}
	app := cls("NSApplication").Send(sel("sharedApplication"))
	app.Send(sel("setActivationPolicy:"), 0) // regular app: Dock icon, menu bar
	app.Send(sel("setDelegate:"), newAppDelegate())
	app.Send(sel("setMainMenu:"), mainMenu())
	icon := cls("NSImage").Send(sel("alloc")).Send(sel("initWithData:"),
		cls("NSData").Send(sel("dataWithBytes:length:"), unsafe.Pointer(&dockIcon[0]), uint(len(dockIcon))))
	app.Send(sel("setApplicationIconImage:"), icon)

	win := cls("NSWindow").Send(sel("alloc")).Send(sel("initWithContentRect:styleMask:backing:defer:"),
		nsRect{Size: nsSize{W: 1320, H: 860}},
		uint(styleTitled|styleClosable|styleMiniaturizable|styleResizable|styleFullSizeContent), uint(backingBuffered), false)
	win.Send(sel("setTitle:"), nsString("Agentrig"))
	win.Send(sel("setTitlebarAppearsTransparent:"), true)
	win.Send(sel("setTitleVisibility:"), 1) // hidden
	win.Send(sel("setMinSize:"), nsSize{W: 960, H: 600})
	win.Send(sel("setBackgroundColor:"), cls("NSColor").Send(sel("colorWithSRGBRed:green:blue:alpha:"), 0x15/255.0, 0x14/255.0, 0x14/255.0, 1.0))
	win.Send(sel("center"))
	win.Send(sel("setFrameAutosaveName:"), nsString("Agentrig")) // reopen where it was

	win.Send(sel("setContentView:"), webView(url))
	keepLightsPlaced(win)
	watchDrags(win)

	win.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	app.Send(sel("activateIgnoringOtherApps:"), true)
	app.Send(sel("run")) // returns never: closing the window quits (see the delegate)
	return nil
}

// webView makes the WKWebView with the bridge script and message handler.
func webView(url string) objc.ID {
	config := cls("WKWebViewConfiguration").Send(sel("new"))
	ucc := config.Send(sel("userContentController"))
	script := cls("WKUserScript").Send(sel("alloc")).Send(sel("initWithSource:injectionTime:forMainFrameOnly:"),
		nsString(bridgeJS+externalLinks+windowChrome), 0, true) // at document start, main frame
	ucc.Send(sel("addUserScript:"), script)
	ucc.Send(sel("addScriptMessageHandler:name:"), newBridge(), nsString("arig"))

	wv := cls("WKWebView").Send(sel("alloc")).Send(sel("initWithFrame:configuration:"), nsRect{Size: nsSize{W: 1320, H: 860}}, config)
	wv.Send(sel("setValue:forKey:"), cls("NSNumber").Send(sel("numberWithBool:"), false), nsString("drawsBackground"))
	req := cls("NSURLRequest").Send(sel("requestWithURL:"), cls("NSURL").Send(sel("URLWithString:"), nsString(url)))
	wv.Send(sel("loadRequest:"), req)
	return wv
}

// bridgeJS gives the page the functions the shared scripts call; each posts
// {fn, args} to the native side.
const bridgeJS = `
(() => {
	const post = (fn) => (...args) => window.webkit.messageHandlers.arig.postMessage(JSON.stringify({ fn, args }));
	window.arigOpen = post('open');
	window.arigDragArea = post('dragArea');
})();
`

func newBridge() objc.ID {
	c, err := objc.RegisterClass("AgentrigBridge", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{
		Cmd: sel("userContentController:didReceiveScriptMessage:"),
		Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, msg objc.ID) {
			var m struct {
				Fn   string            `json:"fn"`
				Args []json.RawMessage `json:"args"`
			}
			body := msg.Send(sel("body"))
			if !objc.Send[bool](body, sel("isKindOfClass:"), objc.GetClass("NSString")) ||
				json.Unmarshal([]byte(goString(body)), &m) != nil || len(m.Args) != 1 {
				return
			}
			switch m.Fn {
			case "open":
				var u string
				if json.Unmarshal(m.Args[0], &u) == nil {
					_ = openExternal(u) // http(s) only
				}
			case "dragArea":
				_ = json.Unmarshal(m.Args[0], &overDragArea)
			}
		},
	}})
	if err != nil {
		panic(err)
	}
	return objc.ID(c).Send(sel("new"))
}

// Closing the only window quits the app, like a single-window Mac app.
func newAppDelegate() objc.ID {
	c, err := objc.RegisterClass("AgentrigAppDelegate", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{
		Cmd: sel("applicationShouldTerminateAfterLastWindowClosed:"),
		Fn:  func(objc.ID, objc.SEL, objc.ID) bool { return true },
	}})
	if err != nil {
		panic(err)
	}
	return objc.ID(c).Send(sel("new"))
}

// mainMenu: the app, Edit (so ⌘C/⌘V/⌘A work in text fields), View and Window menus.
func mainMenu() objc.ID {
	item := func(title, action, key string) objc.ID {
		var a objc.SEL
		if action != "" {
			a = sel(action)
		}
		return cls("NSMenuItem").Send(sel("alloc")).Send(sel("initWithTitle:action:keyEquivalent:"), nsString(title), a, nsString(key))
	}
	menu := func(title string, items ...objc.ID) objc.ID {
		m := cls("NSMenu").Send(sel("alloc")).Send(sel("initWithTitle:"), nsString(title))
		for _, it := range items {
			m.Send(sel("addItem:"), it)
		}
		top := item(title, "", "")
		top.Send(sel("setSubmenu:"), m)
		return top
	}
	sep := func() objc.ID { return cls("NSMenuItem").Send(sel("separatorItem")) }
	bar := cls("NSMenu").Send(sel("new"))
	for _, m := range []objc.ID{
		menu("Agentrig", item("Hide Agentrig", "hide:", "h"), sep(), item("Quit Agentrig", "terminate:", "q")),
		menu("Edit", item("Undo", "undo:", "z"), item("Redo", "redo:", "Z"), sep(),
			item("Cut", "cut:", "x"), item("Copy", "copy:", "c"), item("Paste", "paste:", "v"), item("Select All", "selectAll:", "a")),
		menu("View", item("Reload", "reload:", "r")),
		menu("Window", item("Minimize", "performMiniaturize:", "m"), item("Zoom", "performZoom:", ""), sep(), item("Close", "performClose:", "w")),
	} {
		bar.Send(sel("addItem:"), m)
	}
	return bar
}

// keepLightsPlaced puts the close/minimize/zoom buttons (lightsX, lightsY)
// in from the top-left, centred in a title band of y+button+y. AppKit lays the
// title bar out again on its own schedule (not always announced), so this
// re-runs after every window update and frame change; it does nothing when
// the buttons are already in place.
func keepLightsPlaced(win objc.ID) {
	var gap float64 // native spacing, measured once before anything moves
	placing := false
	place := func() {
		if placing {
			return
		}
		b := [3]objc.ID{}
		for i := range b {
			if b[i] = win.Send(sel("standardWindowButton:"), uint(i)); b[i] == 0 { // close, minimize, zoom
				return
			}
		}
		placing = true
		defer func() { placing = false }()
		bar := b[0].Send(sel("superview"))      // NSTitlebarView
		container := bar.Send(sel("superview")) // NSTitlebarContainerView
		h := objc.Send[nsRect](b[0], sel("frame")).Size.H
		band := lightsY + h + lightsY
		r := objc.Send[nsRect](container, sel("frame"))
		parent := objc.Send[nsRect](container.Send(sel("superview")), sel("frame"))
		if want := (nsRect{Origin: nsPoint{X: r.Origin.X, Y: parent.Size.H - band}, Size: nsSize{W: r.Size.W, H: band}}); r != want {
			container.Send(sel("setFrame:"), want)
		}
		if bounds := objc.Send[nsRect](container, sel("bounds")); objc.Send[nsRect](bar, sel("frame")) != bounds {
			bar.Send(sel("setFrame:"), bounds)
		}
		if gap <= 0 {
			gap = objc.Send[nsRect](b[1], sel("frame")).Origin.X - objc.Send[nsRect](b[0], sel("frame")).Origin.X
			if gap <= 0 {
				gap = 20
			}
		}
		y := band - lightsY - h
		if objc.Send[bool](bar, sel("isFlipped")) {
			y = lightsY
		}
		for i, btn := range b {
			if want := (nsPoint{X: lightsX + float64(i)*gap, Y: y}); objc.Send[nsRect](btn, sel("frame")).Origin != want {
				btn.Send(sel("setFrameOrigin:"), want)
			}
		}
	}
	place()
	again := objc.NewBlock(func(_ objc.Block, _ objc.ID) { place() })
	nc := cls("NSNotificationCenter").Send(sel("defaultCenter"))
	for _, n := range []string{"NSWindowDidUpdateNotification", "NSWindowDidResizeNotification", "NSWindowDidEndLiveResizeNotification",
		"NSWindowDidExitFullScreenNotification", "NSWindowDidBecomeKeyNotification", "NSWindowDidResignKeyNotification"} {
		nc.Send(sel("addObserverForName:object:queue:usingBlock:"), nsString(n), win, objc.ID(0), again)
	}
	for i := 0; i < 3; i++ {
		btn := win.Send(sel("standardWindowButton:"), uint(i))
		btn.Send(sel("setPostsFrameChangedNotifications:"), true)
		nc.Send(sel("addObserverForName:object:queue:usingBlock:"), nsString("NSViewFrameDidChangeNotification"), btn, objc.ID(0), again)
	}
}

// watchDrags: a press on a drag area (the page reports when the pointer is
// over one) moves the window; a double-click zooms it, like a native title
// bar. It must happen here, on the real event: by the time a message from the
// page arrives, the press is over.
func watchDrags(win objc.ID) {
	monitor := objc.NewBlock(func(_ objc.Block, e objc.ID) objc.ID {
		if !overDragArea || e.Send(sel("window")) != win {
			return e
		}
		if objc.Send[int](e, sel("clickCount")) == 2 {
			win.Send(sel("performZoom:"), objc.ID(0))
		} else {
			win.Send(sel("performWindowDragWithEvent:"), e)
		}
		return 0
	})
	cls("NSEvent").Send(sel("addLocalMonitorForEventsMatchingMask:handler:"), uint64(mouseDownMask), monitor)
}
