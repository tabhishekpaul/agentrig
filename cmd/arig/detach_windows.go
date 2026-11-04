//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// startServer runs `arig serve` detached from this console: closing the
// terminal must not stop the always-on server.
func startServer(exe string) error {
	cmd := exec.Command(exe, "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
		HideWindow:    true,
	}
	return cmd.Start()
}

func serverLog() string { return filepath.Join(winDataDir(), "arig.log") }

// serverDetach: started from the Run key at login, Windows gives the server a
// console window of its own, and closing it would stop the server. Let it go
// and write output to a log file instead. (Started by startServer, there's no
// console to begin with.)
func serverDetach() {
	var ids [2]uint32
	if n, _, _ := consoleProcs.Call(uintptr(unsafe.Pointer(&ids[0])), 2); n == 1 {
		freeConsole.Call()
	}
	if err := os.MkdirAll(winDataDir(), 0o755); err != nil {
		return
	}
	if f, err := os.OpenFile(serverLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}
