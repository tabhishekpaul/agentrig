package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/tabhishekpaul/agentrig"
)

var errNoWindow = errors.New("no app window available on this computer")

// externalLinks runs in the app window: any link to another site opens in the
// default browser.
const externalLinks = `document.addEventListener('click', (e) => {
	const a = e.target.closest && e.target.closest('a[href]');
	if (!a) return;
	const u = new URL(a.href, location.href);
	if (u.origin !== location.origin) { e.preventDefault(); window.arigOpen(u.href); }
}, true);`

// windowChrome marks the page as the macOS app (CSS makes room for the window
// buttons) and lets headers move the window: drag to move, double-click to
// zoom, like a native title bar. Controls inside headers still work.
const windowChrome = `
document.documentElement.classList.add('app-mac');
let arigDrag = false;
const dragArea = (t) =>
	!!(t.closest && !t.closest('button, a, input, select, textarea, label, [role=switch], [contenteditable]') && t.closest('header, [data-drag]'));
const setDrag = (on) => { if (on !== arigDrag) { arigDrag = on; window.arigDragArea(on); } };
document.addEventListener('mousemove', (e) => setDrag(dragArea(e.target)), true);
document.addEventListener('mouseleave', () => setDrag(false));`

// customChrome is the frameless window on Windows and Linux: the page shows its own
// macOS-style buttons (class app-custom) and empty header space drags the
// window, double-click maximizes.
const customChrome = `
document.documentElement.classList.add('app-custom');
document.addEventListener('mousedown', (e) => {
	const t = e.target;
	if (e.button !== 0 || !t.closest || t.closest('button, a, input, select, textarea, label, [role=switch], [contenteditable]')) return;
	if (!t.closest('header, [data-drag]')) return;
	if (e.detail === 2) window.arigMaximize(); else window.arigDragStart(e.screenX, e.screenY);
}, true);
window.addEventListener('focus', () => document.documentElement.classList.remove('app-blurred'));
window.addEventListener('blur', () => document.documentElement.classList.add('app-blurred'));`

// openExternal is the only thing the app window may ask of the OS: open an
// http(s) link in the browser. Other schemes (file:, app handlers) are refused,
// since the dashboard shows third-party text from skills.sh.
func openExternal(u string) error {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
		return errors.New("refused: only http(s) links open outside the app")
	}
	agentrig.OpenBrowser(p.String())
	return nil
}

// openApp shows the dashboard in its own window, starting the background
// server first. Without a window (no WebKit/Edge/Chrome) it opens the browser.
func openApp() {
	url := agentrig.URL(agentrig.DefaultPort)
	if err := ensureService(); err != nil {
		fmt.Fprintln(os.Stderr, "arig: serving from the app window:", err)
		go agentrig.Launch(agentrig.DefaultPort, false)
		for range 50 {
			if agentrig.Running(agentrig.DefaultPort) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if err := window(url); err != nil {
		fmt.Fprintln(os.Stderr, "arig:", err, "- opening the browser instead")
		agentrig.OpenBrowser(url)
	}
}
