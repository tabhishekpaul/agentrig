//go:build !windows

package main

import (
	"github.com/tabhishekpaul/agentrig/internal/proc"
)

// On macOS and Linux the server is started by launchd/systemd; this is only
// the fallback for other systems.
func startServer(exe string) error { return proc.Command(exe, "serve").Start() }

func serverLog() string { return "run `arig serve` to see errors" }

func serverDetach() {}
