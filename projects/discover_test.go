package projects

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscover(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mk := func(p string) string { d := filepath.Join(home, p); os.MkdirAll(d, 0o755); return d }
	app := mk("development/app")
	mk("development/app/.git")
	web := mk("code/team/web")
	mk("code/team/web/.git")
	mk("development/app/node_modules/dep/.git") // never descended into
	gone := filepath.Join(home, "development/deleted")

	sess := mk(".claude/projects/-x")
	os.WriteFile(filepath.Join(sess, "a.jsonl"), []byte(`{"type":"summary"}`+"\n"+`{"cwd":"`+app+`"}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(sess, "b.jsonl"), []byte(`{"cwd":"`+app+`"}`+"\n"), 0o644)
	tmp := mk(".claude/projects/-tmp")
	os.WriteFile(filepath.Join(tmp, "a.jsonl"), []byte(`{"cwd":"/private/tmp/wt"}`+"\n"), 0o644)
	mk(".codex")
	os.WriteFile(filepath.Join(home, ".codex/config.toml"), []byte(`[projects."`+web+`"]`+"\ntrust_level = \"trusted\"\n"+`[projects."`+gone+`"]`+"\n"), 0o644)

	got := map[string][]string{}
	for _, c := range Discover(home) {
		got[c.Path] = c.Sources
	}
	if s := got[app]; !slices.Contains(s, "git") || !slices.Contains(s, "Claude Code · 2 sessions") {
		t.Errorf("app: %v", s)
	}
	if s := got[web]; !slices.Contains(s, "git") || !slices.Contains(s, "Codex") {
		t.Errorf("web: %v", s)
	}
	if len(got) != 2 {
		t.Errorf("only the two real projects expected, got %v", got)
	}
}

func TestLooksLikeProject(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "x"), 0o755)
	os.MkdirAll(filepath.Join(home, ".cache/x"), 0o755)
	os.MkdirAll(filepath.Join(home, "Library/x"), 0o755)
	for p, want := range map[string]bool{
		filepath.Join(home, "x"): true, home: false, filepath.Join(home, ".cache/x"): false,
		filepath.Join(home, "Library/x"): false, "/etc": false, filepath.Join(home, "missing"): false,
	} {
		if looksLikeProject(home, p) != want {
			t.Errorf("%s: want %v", p, want)
		}
	}
}
