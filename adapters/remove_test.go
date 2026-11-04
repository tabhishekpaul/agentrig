package adapters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSkillMovesToTrash(t *testing.T) {
	home := t.TempDir()
	trash := filepath.Join(home, "cfg", "backup")
	dir := filepath.Join(home, ".claude", "skills")
	os.MkdirAll(filepath.Join(dir, "pdf"), 0o755)
	os.WriteFile(filepath.Join(dir, "pdf", "SKILL.md"), []byte("x"), 0o644)
	os.MkdirAll(dir+"-disabled", 0o755)
	os.Symlink("../../.agents/skills/go", filepath.Join(dir+"-disabled", "go"))

	id, err := removeSkill([]string{dir}, home, "pdf", trash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pdf")); !os.IsNotExist(err) {
		t.Fatal("skill still there")
	}
	if b, _ := os.ReadFile(filepath.Join(trash, id, ".claude", "skills", "pdf", "SKILL.md")); string(b) != "x" {
		t.Fatal("not in trash")
	}

	// a disabled, symlinked skill: the link moves, its target is untouched
	id, err = removeSkill([]string{dir}, home, "go", trash)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := os.Readlink(filepath.Join(trash, id, ".claude", "skills-disabled", "go")); err != nil || l != "../../.agents/skills/go" {
		t.Fatal("link not moved as a link", l, err)
	}

	for _, bad := range []string{"", "..", "../x", ".hidden", "a/b"} {
		if _, err := removeSkill([]string{dir}, home, bad, trash); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := removeSkill([]string{dir}, home, "missing", trash); err == nil {
		t.Error("missing skill should error")
	}
}

func TestUninstallPluginRejectsFlags(t *testing.T) {
	if err := uninstallClaudePlugin("claude", "--all"); err == nil {
		t.Fatal("flag-like id accepted")
	}
}
