//go:build !darwin && !windows && !linux

package main

// window: no native webview wired up on this OS; use a Chromium app window.
func window(url string) error { return chromeApp(url) }
