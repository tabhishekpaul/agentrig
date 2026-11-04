package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/agentrig.icns
var appIcns []byte

// bundleDir is the macOS app that opens arig's window from Spotlight,
// Launchpad or the Dock: ~/Applications/Agentrig.app.
func bundleDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Applications", "Agentrig.app")
}

// renameOldBundle gives an agentrig.app from an older arig its capitalised
// name (the disk is usually case-insensitive, so it's the same folder).
func renameOldBundle() {
	dir := filepath.Dir(bundleDir())
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == "agentrig.app" {
			_ = os.Rename(filepath.Join(dir, e.Name()), bundleDir())
		}
	}
}

// installBundle writes (or refreshes) Agentrig.app. It only launches
// `arig app`, so updating arig never needs the bundle rebuilt beyond the path.
func installBundle(exe string) error {
	renameOldBundle()
	c := filepath.Join(bundleDir(), "Contents")
	files := []struct {
		path, data string
		mode       os.FileMode
	}{
		{filepath.Join(c, "Info.plist"), infoPlist, 0o644},
		{filepath.Join(c, "MacOS", "agentrig"), "#!/bin/sh\nexec " + shellQuote(exe) + " app\n", 0o755},
		{filepath.Join(c, "Resources", "agentrig.icns"), string(appIcns), 0o644},
	}
	for _, f := range files {
		if _, err := writeIfChanged(f.path, f.data); err != nil {
			return err
		}
		if err := os.Chmod(f.path, f.mode); err != nil {
			return err
		}
	}
	return nil
}

func removeBundle() error { return os.RemoveAll(bundleDir()) }

// shellQuote wraps s in single quotes for /bin/sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Agentrig</string>
	<key>CFBundleDisplayName</key><string>Agentrig</string>
	<key>CFBundleIdentifier</key><string>dev.agentrig.app</string>
	<key>CFBundleExecutable</key><string>agentrig</string>
	<key>CFBundleIconFile</key><string>agentrig</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`
