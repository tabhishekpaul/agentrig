package projects

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrNoPicker means this computer has no folder dialog arig can open
// (Linux without zenity or kdialog): paste the path instead.
var ErrNoPicker = errors.New("no folder picker available here; paste the folder's path instead")

// PickFolder shows the system's choose-folder dialog and returns the chosen
// path, or "" if it was cancelled.
func PickFolder() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// The dialog floats in front of every app on its own. Don't add
		// "tell me to activate": it costs ~2s before the dialog appears.
		cmd = exec.CommandContext(ctx, "osascript",
			"-e", `POSIX path of (choose folder with prompt "Choose a project folder" default location (path to home folder))`)
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command",
			`Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; `+
				`$d.Description = 'Choose a project folder'; if ($d.ShowDialog() -eq 'OK') { $d.SelectedPath }`)
	default:
		if p, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.CommandContext(ctx, p, "--file-selection", "--directory", "--title=Choose a project folder")
		} else if p, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.CommandContext(ctx, p, "--getexistingdirectory", "~", "--title", "Choose a project folder")
		} else {
			return "", ErrNoPicker
		}
	}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", nil // cancelled (every picker exits non-zero on cancel)
		}
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if len(p) > 1 {
		p = strings.TrimRight(p, `/\`) // macOS ends folders with "/"
	}
	return p, nil
}
