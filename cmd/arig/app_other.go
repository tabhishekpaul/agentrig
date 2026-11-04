//go:build !((darwin || windows) && cgo)

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// window opens the dashboard as an app window (no tabs or address bar) in an
// installed Chromium-based browser. Edge ships with Windows, so it's always
// there; it's the same engine WebView2 apps use.
func window(url string) error {
	for _, b := range appBrowsers() {
		if _, err := os.Stat(b); err != nil {
			if b, err = exec.LookPath(b); err != nil {
				continue
			}
		}
		return exec.Command(b, "--app="+url, "--new-window").Start()
	}
	return errNoWindow
}

func appBrowsers() []string {
	switch runtime.GOOS {
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			if d := os.Getenv(env); d != "" {
				out = append(out, filepath.Join(d, `Microsoft\Edge\Application\msedge.exe`), filepath.Join(d, `Google\Chrome\Application\chrome.exe`))
			}
		}
		return out
	case "darwin":
		return []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"}
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"}
}
