//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
static void setDockIcon(const void *b, int n) {
	NSImage *img = [[NSImage alloc] initWithData:[NSData dataWithBytes:b length:n]];
	if (img) [NSApp setApplicationIconImage:img];
}

// placeLights moves the close/minimize/zoom buttons to x, y (top-left of the
// close button, from the window's top-left). AppKit puts them back on every
// resize, so this runs again then.
static void placeLights(NSWindow *win, double x, double y) {
	NSButton *close = [win standardWindowButton:NSWindowCloseButton];
	NSButton *mini = [win standardWindowButton:NSWindowMiniaturizeButton];
	NSButton *zoom = [win standardWindowButton:NSWindowZoomButton];
	if (!close || !mini || !zoom) return;
	NSView *bar = close.superview.superview;
	NSRect r = bar.frame;
	r.size.height = close.frame.size.height + y;
	r.origin.y = win.frame.size.height - r.size.height;
	bar.frame = r;
	double gap = mini.frame.origin.x - close.frame.origin.x;
	NSArray *buttons = @[close, mini, zoom];
	for (NSUInteger i = 0; i < buttons.count; i++) {
		NSButton *b = buttons[i];
		[b setFrameOrigin:NSMakePoint(x + i * gap, b.frame.origin.y)];
	}
}

// overlayTitlebar removes the title strip: the page fills the window and the
// native window buttons sit inside the app's own sidebar header.
static void overlayTitlebar(void *w, double x, double y) {
	NSWindow *win = (NSWindow *)w;
	win.titlebarAppearsTransparent = YES;
	win.titleVisibility = NSWindowTitleHidden;
	win.styleMask |= NSWindowStyleMaskFullSizeContentView;
	win.minSize = NSMakeSize(960, 600);
	placeLights(win, x, y);
	for (NSNotificationName n in @[NSWindowDidResizeNotification, NSWindowDidEndLiveResizeNotification,
	                               NSWindowDidExitFullScreenNotification, NSWindowDidBecomeKeyNotification]) {
		[[NSNotificationCenter defaultCenter] addObserverForName:n object:win queue:nil
		                                              usingBlock:^(NSNotification *note) { placeLights(win, x, y); }];
	}
}

// The page says whether the pointer is over a drag area (a header's empty
// space); a mouse-down there moves the window, a double-click zooms it, like a
// native title bar. It has to happen here, on the real event: by the time a
// message from the page arrives, the press is already over.
static BOOL overDragArea = NO;
static void setDragArea(BOOL on) { overDragArea = on; }
static void watchDrags(void *w) {
	NSWindow *win = (NSWindow *)w;
	[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown handler:^NSEvent *(NSEvent *e) {
		if (e.window != win || !overDragArea) return e;
		if (e.clickCount == 2) [win performZoom:nil];
		else [win performWindowDragWithEvent:e];
		return nil;
	}];
}

*/
import "C"

import (
	_ "embed"
	"runtime"
	"unsafe"

	webview "github.com/webview/webview_go"
)

//go:embed assets/icon512.png
var dockIcon []byte

// Cocoa windows must live on the main thread.
func init() { runtime.LockOSThread() }

// window shows the dashboard in a native WebKit window until it's closed.
func window(url string) error {
	w := webview.New(false)
	if w == nil {
		return errNoWindow
	}
	defer w.Destroy()
	C.setDockIcon(unsafe.Pointer(&dockIcon[0]), C.int(len(dockIcon)))
	w.SetTitle("Agentrig")
	w.SetSize(1320, 860, webview.HintNone)
	win := w.Window()
	C.overlayTitlebar(win, 18, 21) // centred in the sidebar's 56 px brand row
	C.watchDrags(win)
	_ = w.Bind("arigDragArea", func(on bool) { C.setDragArea(C.BOOL(on)) })
	// The webview can't open new windows: send links that leave the dashboard
	// (skills.sh, GitHub, docs) to the default browser instead.
	_ = w.Bind("arigOpen", openExternal)
	w.Init(externalLinks + windowChrome)
	w.Navigate(url)
	w.Run()
	return nil
}
