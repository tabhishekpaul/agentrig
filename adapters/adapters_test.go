package adapters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeSkills(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "good"), 0o755)
	os.WriteFile(filepath.Join(dir, "good", "SKILL.md"),
		[]byte("---\nname: good-skill\ndescription: \"Does: things\"\n---\nbody\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "noskill"), 0o755)
	os.WriteFile(filepath.Join(dir, ".DS_Store"), nil, 0o644)

	got := claudeSkills(dir)
	if len(got) != 1 || got[0].Name != "good-skill" || got[0].Description != "Does: things" {
		t.Fatalf("got %+v", got)
	}
}
