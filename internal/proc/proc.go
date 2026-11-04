// Package proc starts external commands (git, gh, claude, powershell, …)
// without a console window. The always-on server runs with no console on
// Windows, so every console program it starts would otherwise flash a window
// of its own.
package proc

import (
	"context"
	"os/exec"
)

// Command is exec.Command with no console window on Windows.
func Command(name string, args ...string) *exec.Cmd { return hide(exec.Command(name, args...)) }

// CommandContext is exec.CommandContext with no console window on Windows.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return hide(exec.CommandContext(ctx, name, args...))
}
