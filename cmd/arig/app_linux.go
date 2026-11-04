//go:build linux

package main

// window: placeholder until the runtime-loaded WebKitGTK window lands.
func window(url string) error { return chromeApp(url) }
