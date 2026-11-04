package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelinkReplacesMatchingCopiesOnly(t *testing.T) {
	home := t.TempDir()
	root := RootAt(home)
	gen := filepath.Join(home, ".arig", "index")
	entry := filepath.Join(root, "a", "0123456789ab")
	os.MkdirAll(entry, 0o755)
	os.WriteFile(filepath.Join(entry, "SKILL.md"), []byte("a"), 0o644)
	secret := filepath.Join(home, ".ssh")
	os.MkdirAll(secret, 0o755)
	dir := filepath.Join(home, ".claude", "skills")
	copyLinks = true
	Link(filepath.Join(dir, "same"), entry)
	Link(filepath.Join(dir, "edited"), entry)
	Link(filepath.Join(dir, "planted"), entry)
	copyLinks = false
	os.WriteFile(filepath.Join(dir, "edited", "SKILL.md"), []byte("mine"), 0o644)
	// a skill shipping its own marker that escapes the index dir
	os.WriteFile(filepath.Join(dir, "planted", copyMarker), []byte(filepath.Join(gen, "..", "..", ".ssh")), 0o644)

	for name, want := range map[string]bool{"same": true, "edited": false, "planted": false} {
		if relinkOne(filepath.Join(dir, name), root, gen) != want {
			t.Fatalf("%s: relinked=%v", name, !want)
		}
	}
	if fi, _ := os.Lstat(filepath.Join(dir, "same")); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("matching copy not turned into a link")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "edited", "SKILL.md")); string(b) != "mine" || !exists(filepath.Join(dir, "edited", copyMarker)) {
		t.Fatal("edited copy touched")
	}
	if fi, _ := os.Lstat(filepath.Join(dir, "planted")); fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("followed a planted marker")
	}
}
