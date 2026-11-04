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
