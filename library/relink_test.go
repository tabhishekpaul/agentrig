package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelinkReplacesMatchingCopiesOnly(t *testing.T) {
	home := t.TempDir()
	entry := filepath.Join(home, "lib", "a")
	os.MkdirAll(entry, 0o755)
	os.WriteFile(filepath.Join(entry, "SKILL.md"), []byte("a"), 0o644)
	dir := filepath.Join(home, ".claude", "skills")
	copyLinks = true
	Link(filepath.Join(dir, "same"), entry)
	Link(filepath.Join(dir, "edited"), entry)
	copyLinks = false
	os.WriteFile(filepath.Join(dir, "edited", "SKILL.md"), []byte("mine"), 0o644)

	for _, name := range []string{"same", "edited"} {
		if relinkOne(filepath.Join(dir, name), filepath.Join(home, ".arig", "index")+"/") != (name == "same") {
			t.Fatalf("%s: wrong relink decision", name)
		}
	}
	if fi, _ := os.Lstat(filepath.Join(dir, "same")); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("matching copy not turned into a link")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "edited", "SKILL.md")); string(b) != "mine" {
		t.Fatal("edited copy touched")
	}
	if !exists(filepath.Join(dir, "edited", copyMarker)) {
		t.Fatal("edited copy lost its marker")
	}
}
