package gitsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(path string) string { b, _ := os.ReadFile(path); return string(b) }

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func machine(t *testing.T) *Syncer {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755)
	return &Syncer{Home: home, Dir: filepath.Join(root, "agentrig")}
}

func run(t *testing.T, s *Syncer) {
	t.Helper()
	if c, err := s.Run(); err != nil {
		t.Fatalf("sync: %v (%+v)", err, c)
	}
}

func TestTwoMachines(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}

	// Machine A: a real skill, a shared skill linked from Claude, a secret.
	a := machine(t)
	write(t, filepath.Join(a.Home, ".claude/skills/s1/SKILL.md"), "---\nname: s1\n---\nv1")
	write(t, filepath.Join(a.Home, ".claude/skills/s1/.env"), "TOKEN=secret")
	write(t, filepath.Join(a.Home, ".agents/skills/shared/SKILL.md"), "---\nname: shared\n---\n")
	os.Symlink("../../.agents/skills/shared", filepath.Join(a.Home, ".claude/skills/shared"))
	if _, err := a.Connect(remote); err != nil {
		t.Fatal(err)
	}

	// Machine B joins with a skill of its own and no ~/.agents or ~/.cursor.
	b := machine(t)
	write(t, filepath.Join(b.Home, ".claude/skills/b1/SKILL.md"), "b1 v1")
	write(t, filepath.Join(a.Home, ".cursor/skills/c1/SKILL.md"), "cursor skill on A only")
	run(t, a)
	if _, err := b.Connect(remote); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(b.Home, ".claude/skills/b1")) {
		t.Fatal("joining deleted B's own skill")
	}
	if read(filepath.Join(b.Home, ".claude/skills/shared/SKILL.md")) == "" {
		t.Fatal("shared skill link doesn't resolve on B (~/.agents not synced)")
	}
	if read(filepath.Join(b.Home, ".claude/skills/s1/SKILL.md")) != "---\nname: s1\n---\nv1" {
		t.Fatal("s1 not received on B")
	}
	if exists(filepath.Join(b.Home, ".claude/skills/s1/.env")) {
		t.Fatal(".env was synced")
	}
	if exists(filepath.Join(b.Home, ".cursor")) {
		t.Fatal("created ~/.cursor on a machine without Cursor")
	}

	run(t, a)
	if !exists(filepath.Join(a.Home, ".claude/skills/b1")) {
		t.Fatal("A didn't receive b1")
	}
	if !exists(filepath.Join(a.Home, ".cursor/skills/c1")) {
		t.Fatal("B (no Cursor) deleted A's Cursor skill")
	}

	// A switches s1 off (moved to skills-disabled); B follows and keeps a backup.
	os.MkdirAll(filepath.Join(a.Home, ".claude/skills-disabled"), 0o755)
	os.Rename(filepath.Join(a.Home, ".claude/skills/s1"), filepath.Join(a.Home, ".claude/skills-disabled/s1"))
	run(t, a)
	run(t, b)
	if exists(filepath.Join(b.Home, ".claude/skills/s1")) || !exists(filepath.Join(b.Home, ".claude/skills-disabled/s1/SKILL.md")) {
		t.Fatal("disable didn't propagate to B")
	}
	backups, _ := filepath.Glob(filepath.Join(b.Dir, "backup", "*", ".claude/skills/s1/SKILL.md"))
	if len(backups) == 0 {
		t.Fatal("no backup of the replaced s1 on B")
	}

	// Conflict: B deletes b1 while A edits it. A syncs last, so A's edit wins.
	os.RemoveAll(filepath.Join(b.Home, ".claude/skills/b1"))
	run(t, b)
	write(t, filepath.Join(a.Home, ".claude/skills/b1/SKILL.md"), "b1 v2 edited on A")
	run(t, a)
	run(t, b)
	if got := read(filepath.Join(b.Home, ".claude/skills/b1/SKILL.md")); got != "b1 v2 edited on A" {
		t.Fatalf("edit-vs-delete: B has %q", got)
	}
}

func TestRedact(t *testing.T) {
	if got := Redact("https://ghp_secret@github.com/me/x.git"); strings.Contains(got, "ghp_secret") {
		t.Fatal(got)
	}
	if Redact("git@github.com:me/x.git") != "git@github.com:me/x.git" {
		t.Fatal("ssh URL changed")
	}
}

func TestConnectRejectsFlags(t *testing.T) {
	s := machine(t)
	if _, err := s.Connect("--upload-pack=touch /tmp/x"); err == nil {
		t.Fatal("accepted a git flag as repo URL")
	}
}
