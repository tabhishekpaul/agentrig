package main

import (
	_ "embed"
	le "encoding/binary"
	"github.com/tabhishekpaul/agentrig/internal/proc"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed assets/icon512.png
var appIconPNG []byte

//go:embed assets/icon256.png
var icon256PNG []byte // .ico images top out at 256 px

// installShortcut puts Agentrig in the system's app list so it opens like any
// app (`arig app`): a Start menu shortcut on Windows, a .desktop entry on
// Linux. macOS gets Agentrig.app instead (bundle.go).
func installShortcut(exe string) error {
	switch runtime.GOOS {
	case "windows":
		return windowsShortcut(exe)
	case "linux":
		return linuxDesktopEntry(exe)
	}
	return nil
}

func removeShortcut() {
	switch runtime.GOOS {
	case "windows":
		os.Remove(startMenuLink())
		os.Remove(filepath.Join(winDataDir(), "Agentrig.ico"))
		os.Remove(filepath.Join(winDataDir(), "shortcut.txt"))
	case "linux":
		home, _ := os.UserHomeDir()
		os.Remove(desktopFile(home))
		os.Remove(linuxIcon(home))
	}
}

// --- Windows ---

func winDataDir() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "arig") }
func startMenuLink() string {
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Agentrig.lnk")
}

func windowsShortcut(exe string) error {
	ico := filepath.Join(winDataDir(), "Agentrig.ico")
	if _, err := writeIfChanged(ico, string(icoFromPNG(icon256PNG))); err != nil {
		return err
	}
	// Recreate the shortcut only when arig moved or the shortcut is gone.
	changed, err := writeIfChanged(filepath.Join(winDataDir(), "shortcut.txt"), exe)
	if err != nil {
		return err
	}
	if _, err := os.Stat(startMenuLink()); err == nil && !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(startMenuLink()), 0o755); err != nil {
		return err
	}
	// Windows' own shortcut API. Paths go in through the environment, never
	// spliced into the script. WindowStyle 7 = minimized, so the console arig
	// starts with doesn't flash up before the window.
	cmd := proc.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:ARIG_LNK); `+
			`$s.TargetPath = $env:ARIG_EXE; $s.Arguments = 'app'; $s.IconLocation = $env:ARIG_ICO; `+
			`$s.WorkingDirectory = $env:USERPROFILE; $s.WindowStyle = 7; $s.Description = 'Agentrig'; $s.Save()`)
	cmd.Env = append(os.Environ(), "ARIG_LNK="+startMenuLink(), "ARIG_EXE="+exe, "ARIG_ICO="+ico)
	return cmd.Run()
}

// icoFromPNG wraps a PNG in a one-image .ico (PNG entries are valid since
// Windows Vista); a size byte of 0 means 256 px or more.
func icoFromPNG(png []byte) []byte {
	b := make([]byte, 22, 22+len(png))
	le.LittleEndian.PutUint16(b[2:], 1)  // type: icon
	le.LittleEndian.PutUint16(b[4:], 1)  // one image
	le.LittleEndian.PutUint16(b[10:], 1) // colour planes
	le.LittleEndian.PutUint16(b[12:], 32)
	le.LittleEndian.PutUint32(b[14:], uint32(len(png)))
	le.LittleEndian.PutUint32(b[18:], 22) // image data follows the header
	return append(b, png...)
}

// --- Linux ---

func desktopFile(home string) string {
	return filepath.Join(home, ".local", "share", "applications", "agentrig.desktop")
}
func linuxIcon(home string) string {
	return filepath.Join(home, ".local", "share", "icons", "hicolor", "512x512", "apps", "agentrig.png")
}

func linuxDesktopEntry(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if _, err := writeIfChanged(linuxIcon(home), string(appIconPNG)); err != nil {
		return err
	}
	_, err = writeIfChanged(desktopFile(home), desktopEntry(exe))
	return err
}

func desktopEntry(exe string) string {
	return "[Desktop Entry]\nType=Application\nName=Agentrig\n" +
		"Comment=Skills, plugins and MCP servers across your AI coding tools\n" +
		"Exec=" + desktopQuote(exe) + " app\nIcon=agentrig\nTerminal=false\nCategories=Development;Utility;\n"
}

// desktopQuote quotes an Exec argument per the Desktop Entry spec.
func desktopQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`)
	return `"` + r.Replace(s) + `"`
}
