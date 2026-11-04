package library

import (
	"os"
	"path/filepath"
	"strings"
)

// Relink turns copies made when links weren't possible (Windows before
// junctions) back into links, in every tool's skills folders. A copy is
// replaced only when it still matches the folder it was copied from, so a
// copy edited by hand is left alone. Generated folders under ~/.arig/index
// are always relinked (their copies only go stale). Returns how many.
func Relink(home string) (n int) {
	gen := filepath.Join(home, ".arig", "index") + string(filepath.Separator)
	for _, d := range skillDirs(home) {
		ents, _ := os.ReadDir(d)
		for _, e := range ents {
			p := filepath.Join(d, e.Name())
			if relinkOne(p, gen) {
				n++
			}
		}
	}
	return n
}

func relinkOne(p, gen string) bool {
	marker := filepath.Join(p, copyMarker)
	b, err := os.ReadFile(marker)
	if err != nil {
		return false // not a copy arig made
	}
	src := strings.TrimSpace(string(b))
	if !isDir(src) {
		return false
	}
	if !strings.HasPrefix(src, gen) {
		want, err := Hash(src)
		if err != nil || os.Remove(marker) != nil {
			return false
		}
		got, err := Hash(p)
		if err != nil || got != want {
			os.WriteFile(marker, b, 0o644) // edited by hand: keep the copy as it was
			return false
		}
	}
	tmp := filepath.Join(filepath.Dir(p), ".arig-relink-"+filepath.Base(p))
	os.RemoveAll(tmp)
	if os.Rename(p, tmp) != nil {
		return false
	}
	if !LinkDir(p, src) {
		os.Rename(tmp, p) // still no links here: put the copy back
		if !exists(marker) {
			os.WriteFile(marker, b, 0o644)
		}
		return false
	}
	os.RemoveAll(tmp)
	return true
}
