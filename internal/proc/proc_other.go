//go:build !windows

package proc

import "os/exec"

func hide(c *exec.Cmd) *exec.Cmd { return c }
