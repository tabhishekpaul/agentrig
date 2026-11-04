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

// placeLights puts the close/minimize/zoom buttons inset (x, y) from the
// window's top-left: the title bar band grows to y+button+y so they sit
// centred in it. AppKit lays the title bar out again on its own schedule, so
// this re-runs whenever a button's frame changes.
static BOOL placing = NO;
static void placeLights(NSWindow *win, double x, double y) {
	if (placing) return;
	NSButton *close = [win standardWindowButton:NSWindowCloseButton];
	NSButton *mini = [win standardWindowButton:NSWindowMiniaturizeButton];
	NSButton *zoom = [win standardWindowButton:NSWindowZoomButton];
	if (!close || !mini || !zoom) return;
	placing = YES;
	NSView *bar = close.superview;        // NSTitlebarView
	NSView *container = bar.superview;    // NSTitlebarContainerView
	double h = close.frame.size.height;
	double band = y + h + y;
	NSRect r = container.frame;
	r.size.height = band;
	r.origin.y = container.superview.frame.size.height - band;
	if (!NSEqualRects(container.frame, r)) container.frame = r;
	if (!NSEqualRects(bar.frame, container.bounds)) bar.frame = container.bounds;
	static double gap = 0; // native spacing, measured once before anything moves
	if (gap <= 0) gap = mini.frame.origin.x - close.frame.origin.x;
	if (gap <= 0) gap = 20;
	NSArray *buttons = @[close, mini, zoom];
	for (NSUInteger i = 0; i < buttons.count; i++) {
		NSButton *b = buttons[i];
		NSPoint want = NSMakePoint(x + i * gap, bar.isFlipped ? y : band - y - h);
		if (!NSEqualPoints(b.frame.origin, want)) [b setFrameOrigin:want];
	}
	placing = NO;
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
	void (^again)(NSNotification *) = ^(NSNotification *note) { placeLights(win, x, y); };
	NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
	// DidUpdate runs after every event the window handles: it catches the
	// relayouts AppKit doesn't announce, and costs nothing when nothing moved.
	for (NSNotificationName n in @[NSWindowDidUpdateNotification, NSWindowDidResizeNotification, NSWindowDidEndLiveResizeNotification,
	                               NSWindowDidExitFullScreenNotification, NSWindowDidBecomeKeyNotification,
	                               NSWindowDidResignKeyNotification]) {
		[nc addObserverForName:n object:win queue:nil usingBlock:again];
	}
	for (NSWindowButton k = NSWindowCloseButton; k <= NSWindowZoomButton; k++) {
		NSButton *b = [win standardWindowButton:k];
		b.postsFrameChangedNotifications = YES;
		[nc addObserverForName:NSViewFrameDidChangeNotification object:b queue:nil usingBlock:again];
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
	C.overlayTitlebar(win, 18, 18) // in line with the logo and the sidebar icons (18 px), as far from the top
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
