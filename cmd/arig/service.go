package main

import (
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig"
)

// arig keeps its dashboard server always on: the first `arig` run registers
// `arig serve` to start at login and restart if it exits, per user, with no
// admin rights (LaunchAgent on macOS, systemd user unit on Linux, HKCU Run
// key on Windows). Every later run re-checks the registration, so moving or
// upgrading the binary repairs itself.
const label = "dev.agentrig.arig"

// binary is the stable path of this arig executable, for the service to run.
func binary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	if strings.Contains(exe, "go-build") || strings.HasPrefix(exe, os.TempDir()) {
		return "", errors.New("not an installed binary (go run)")
	}
	return exe, nil
}

func home() string { h, _ := os.UserHomeDir(); return h }

func plistPath() string     { return filepath.Join(home(), "Library", "LaunchAgents", label+".plist") }
func unitPath() string      { return filepath.Join(home(), ".config", "systemd", "user", "arig.service") }
func logPath() string       { return filepath.Join(home(), "Library", "Logs", "arig.log") }
func launchdTarget() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

const winRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func plist(exe string) string {
	e := html.EscapeString
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + label + `</string>
	<key>ProgramArguments</key>
	<array><string>` + e(exe) + `</string><string>serve</string></array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>ProcessType</key><string>Background</string>
	<key>StandardOutPath</key><string>` + e(logPath()) + `</string>
	<key>StandardErrorPath</key><string>` + e(logPath()) + `</string>
</dict>
</plist>
`
}

func unit(exe string) string {
	return `[Unit]
Description=agentrig dashboard (arig)

[Service]
ExecStart="` + exe + `" serve
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// writeIfChanged writes path only if its content differs; reports whether it changed.
func writeIfChanged(path, content string) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(content), 0o644)
}

// ensureService makes sure the always-on server is registered for this
// binary and running, then waits until it answers.
func ensureService() error {
	exe, err := binary()
	if err != nil {
		return err
	}
	running := agentrig.Running(agentrig.DefaultPort)
	// A server from an older build (arig was upgraded) must be restarted too.
	stale := running && agentrig.RunningBuild(agentrig.DefaultPort) != agentrig.Build
	switch runtime.GOOS {
	case "darwin":
		changed, err := writeIfChanged(plistPath(), plist(exe))
		if err != nil {
			return err
		}
		if changed || !running || stale {
			_ = run("launchctl", "bootout", launchdTarget()+"/"+label) // drop a stale registration
			if err := run("launchctl", "bootstrap", launchdTarget(), plistPath()); err != nil {
				return err
			}
		}
	case "linux":
		changed, err := writeIfChanged(unitPath(), unit(exe))
		if err != nil {
			return err
		}
		if changed || !running || stale {
			for _, args := range [][]string{{"daemon-reload"}, {"enable", "arig.service"}, {"restart", "arig.service"}} {
				if err := run("systemctl", append([]string{"--user"}, args...)...); err != nil {
					return err
				}
			}
		}
	case "windows":
		// ponytail: Run key starts it at login but nothing restarts it after a
		// crash; `arig` restarts it on demand. Use a scheduled task if that matters.
		if err := run("reg", "add", winRunKey, "/v", "arig", "/t", "REG_SZ", "/d", `"`+exe+`" serve`, "/f"); err != nil {
			return err
		}
		if stale {
			_ = run("taskkill", "/F", "/IM", "arig.exe") // ponytail: also stops a foreground arig
		}
		if !running || stale {
			if err := exec.Command(exe, "serve").Start(); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("background mode isn't supported on %s yet", runtime.GOOS)
	}
	for i := 0; i < 40; i++ {
		if agentrig.RunningBuild(agentrig.DefaultPort) == agentrig.Build {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("background server didn't start; see %s", logHint())
}

func removeService() error {
	switch runtime.GOOS {
	case "darwin":
		_ = run("launchctl", "bootout", launchdTarget()+"/"+label)
		if err := os.Remove(plistPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
	case "linux":
		_ = run("systemctl", "--user", "disable", "--now", "arig.service")
		if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		_ = run("systemctl", "--user", "daemon-reload")
	case "windows":
		_ = run("reg", "delete", winRunKey, "/v", "arig", "/f")
		_ = run("taskkill", "/F", "/IM", "arig.exe")
	}
	return nil
}

func logHint() string {
	switch runtime.GOOS {
	case "darwin":
		return logPath()
	case "linux":
		return "journalctl --user -u arig"
	}
	return "run `arig serve` to see errors"
}
