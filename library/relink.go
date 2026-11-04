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
	gen := filepath.Join(home, ".arig", "index")
	root := RootAt(home)
	for _, d := range skillDirs(home) {
		ents, _ := os.ReadDir(d)
		for _, e := range ents {
			p := filepath.Join(d, e.Name())
			if relinkOne(p, root, gen) {
				n++
			}
		}
	}
	return n
}

func relinkOne(p, root, gen string) bool {
	marker := filepath.Join(p, copyMarker)
	b, err := os.ReadFile(marker)
	if err != nil {
		return false // not a copy arig made
	}
	// The marker is a file anyone's skill could ship: follow it only to a
	// direct entry of the library (<root>/<name>/<hash>) or the index dir.
	src, err := filepath.EvalSymlinks(filepath.Clean(strings.TrimSpace(string(b))))
	if err != nil || !isDir(src) {
		return false
	}
	generated := within(src, gen, 1)
	if !generated && !within(src, root, 2) {
		return false
	}
	if !generated {
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

// within: p is exactly depth folders below dir (both resolved).
func within(p, dir string, depth int) bool {
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(d, p)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	return len(strings.Split(rel, string(filepath.Separator))) == depth
}
