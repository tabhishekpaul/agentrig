package library

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/tabhishekpaul/agentrig/internal/proc"
)

// LinkDir points at to the folder real without copying it: a relative
// symlink, else on Windows a directory junction (needs no admin rights or
// Developer Mode, the way pnpm and other skill managers link there). False
// means neither worked and the caller should copy.
//
// go.mod sets godebug winsymlink=0 so Go reports junctions as symlinks, and
// every "is this our link" check treats both alike.
func LinkDir(at, real string) bool {
	if parent, err := filepath.EvalSymlinks(filepath.Dir(at)); err == nil {
		if rel, err := filepath.Rel(parent, real); err == nil && os.Symlink(rel, at) == nil {
			return true
		}
	}
	return runtime.GOOS == "windows" && proc.Command("cmd", "/c", "mklink", "/J", at, real).Run() == nil
}
