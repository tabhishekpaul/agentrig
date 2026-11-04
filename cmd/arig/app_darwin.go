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
*/
import "C"

import (
	_ "embed"
	"runtime"
	"unsafe"

	"github.com/tabhishekpaul/agentrig"
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
	w.SetTitle("agentrig")
	w.SetSize(1320, 860, webview.HintNone)
	// The webview can't open new windows: send links that leave the dashboard
	// (skills.sh, GitHub, docs) to the default browser instead.
	_ = w.Bind("arigOpen", func(u string) { agentrig.OpenBrowser(u) })
	w.Init(externalLinks)
	w.Navigate(url)
	w.Run()
	return nil
}
