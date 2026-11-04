package projects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/adapters"
)

// setup fakes a machine with claude + cursor installed and global skills a, b.
func setup(t *testing.T) (proj string, skills string) {
	t.Helper()
	tmp := t.TempDir()
	skills = filepath.Join(tmp, "home", ".claude", "skills")
	for _, s := range []string{"a", "b"} {
		os.MkdirAll(filepath.Join(skills, s), 0o755)
		os.WriteFile(filepath.Join(skills, s, "SKILL.md"), []byte("---\nname: "+s+"\n---\n"), 0o644)
	}
	detect = func() []adapters.Tool {
		var items []adapters.Item
		for _, s := range []string{"a", "b"} {
			items = append(items, adapters.Item{Kind: "skill", ID: s, Name: s, Path: filepath.Join(skills, s), Enabled: true})
		}
		return []adapters.Tool{
			{ID: "claude", Installed: true, Items: items},
			{ID: "cursor", Installed: true},
			{ID: "gemini"},
		}
	}
	configDir = func() string { return filepath.Join(tmp, "cfg") }
	proj = filepath.Join(tmp, "proj")
	os.MkdirAll(filepath.Join(proj, ".git"), 0o755)
	return proj, skills
}

func states(ls []Link) map[string]string {
	out := map[string]string{}
	for _, l := range ls {
		out[l.Path] = l.State
	}
	return out
}

func TestApplyLinksAndRemoves(t *testing.T) {
	proj, skills := setup(t)
	if _, err := Edit(proj, true, "a", "b", "nope"); err != nil {
		t.Fatal(err)
	}
	ls, err := Apply(proj)
	if err != nil {
		t.Fatal(err)
	}
	got := states(ls)
	want := map[string]string{
		".claude/skills/a": "linked", ".claude/skills/b": "linked", ".claude/skills/nope": "not-installed",
		".cursor/skills/a": "linked", ".cursor/skills/b": "linked", ".cursor/skills/nope": "not-installed",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}
	// relative link that resolves to the global skill
	l, _ := os.Readlink(filepath.Join(proj, ".claude/skills/a"))
	if filepath.IsAbs(l) {
		t.Errorf("link %q should be relative", l)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude/skills/a/SKILL.md")); err != nil {
		t.Error(err)
	}
	gi, _ := os.ReadFile(filepath.Join(proj, ".gitignore"))
	if !strings.Contains(string(gi), "/.claude/skills/a\n") || !strings.Contains(string(gi), "/.gemini/skills/b\n") {
		t.Errorf(".gitignore:\n%s", gi)
	}

	// re-apply is a no-op
	if ls2, err := Apply(proj); err != nil || len(ls2) != len(ls) {
		t.Fatal(ls2, err)
	}

	// drop b: its links go, a stays
	Edit(proj, false, "b", "nope")
	if _, err := Apply(proj); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(proj, ".claude/skills/b")) || !exists(filepath.Join(proj, ".claude/skills/a")) {
		t.Error("b should be unlinked, a kept")
	}
	// the global skill is untouched
	if _, err := os.Stat(filepath.Join(skills, "b", "SKILL.md")); err != nil {
		t.Fatal("global skill deleted:", err)
	}

	// empty manifest: everything arig made is gone, block removed from .gitignore
	Edit(proj, false, "a")
	Apply(proj)
	if exists(filepath.Join(proj, ".claude/skills/a")) || exists(filepath.Join(proj, lockFile)) {
		t.Error("links or lock left behind")
	}
	if gi, _ := os.ReadFile(filepath.Join(proj, ".gitignore")); strings.Contains(string(gi), "arig") {
		t.Errorf(".gitignore still has the block:\n%s", gi)
	}
}

func TestNeverDeletesUserFiles(t *testing.T) {
	proj, _ := setup(t)
	// the user has their own skill "a" for claude
	mine := filepath.Join(proj, ".claude/skills/a")
	os.MkdirAll(mine, 0o755)
	os.WriteFile(filepath.Join(mine, "SKILL.md"), []byte("mine"), 0o644)
	os.WriteFile(filepath.Join(proj, ".gitignore"), []byte("node_modules\n"), 0o644)

	Edit(proj, true, "a")
	ls, err := Apply(proj)
	if err != nil {
		t.Fatal(err)
	}
	if s := states(ls); s[".claude/skills/a"] != "conflict" || s[".cursor/skills/a"] != "linked" {
		t.Fatal(s)
	}

	// replace arig's cursor link with a user folder, then drop the skill
	cur := filepath.Join(proj, ".cursor/skills/a")
	os.Remove(cur)
	os.MkdirAll(cur, 0o755)
	os.WriteFile(filepath.Join(cur, "SKILL.md"), []byte("mine too"), 0o644)
	// and a tampered lock pointing outside the tool dirs
	os.WriteFile(filepath.Join(proj, lockFile), []byte(`[{"path":".claude/skills/a"},{"path":".cursor/skills/a","copy":true},{"path":"../x"}]`), 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(proj), "x"), []byte("outside"), 0o644)

	Edit(proj, false, "a")
	if _, err := Apply(proj); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(mine, "SKILL.md"), filepath.Join(cur, "SKILL.md"), filepath.Join(filepath.Dir(proj), "x")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("user file deleted: %s", p)
		}
	}
	if gi, _ := os.ReadFile(filepath.Join(proj, ".gitignore")); string(gi) != "node_modules\n" {
		t.Errorf(".gitignore changed:\n%s", gi)
	}
}

func TestCopyFallbackIsOwned(t *testing.T) {
	proj, skills := setup(t)
	// what link() leaves when symlinks aren't possible
	e := lockEntry{Path: ".claude/skills/a", Copy: true}
	os.CopyFS(filepath.Join(proj, e.Path), os.DirFS(filepath.Join(skills, "a")))
	os.WriteFile(filepath.Join(proj, e.Path, copyMarker), nil, 0o644)
	os.MkdirAll(filepath.Join(proj, Folder), 0o755)
	os.WriteFile(filepath.Join(proj, lockFile), []byte(`[{"path":".claude/skills/a","copy":true}]`), 0o644)
	if _, err := Apply(proj); err != nil { // empty manifest
		t.Fatal(err)
	}
	if exists(filepath.Join(proj, e.Path)) {
		t.Error("arig's copy should be removed")
	}
}

func TestToolDirOutsideProjectRefused(t *testing.T) {
	proj, _ := setup(t)
	outside := t.TempDir()
	os.MkdirAll(proj, 0o755)
	os.Symlink(outside, filepath.Join(proj, ".claude"))
	WriteManifest(proj, Manifest{Skills: []string{"a"}, Tools: []string{"claude"}})
	if _, err := Apply(proj); err == nil {
		t.Error("want an error")
	}
	if exists(filepath.Join(outside, "skills", "a")) {
		t.Error("linked outside the project")
	}
}

func TestValidation(t *testing.T) {
	proj, _ := setup(t)
	for _, bad := range []string{"", "..", "../x", "a/b", ".hidden"} {
		if _, err := Edit(proj, true, bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if err := WriteManifest(proj, Manifest{Tools: []string{"zed"}}); err == nil {
		t.Error("accepted a tool without project skills")
	}
	if err := Add("relative"); err == nil {
		t.Error("accepted relative path")
	}
	real, _ := filepath.EvalSymlinks(proj)
	if err := Add(proj); err != nil || !Known(real) || len(List()) != 1 {
		t.Fatal(err, List())
	}
	Add(real) // same folder: no duplicate
	Remove(real)
	if len(List()) != 0 {
		t.Error("not removed")
	}
}

func TestProjectSkillsAndMigration(t *testing.T) {
	proj, _ := setup(t)
	// an older project: manifest and lock at the top level
	os.WriteFile(filepath.Join(proj, oldManifest), []byte(`{"skills":["a"]}`), 0o644)
	own := filepath.Join(proj, LocalSkills, "house-style")
	os.MkdirAll(own, 0o755)
	os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("---\nname: house-style\n---\n"), 0o644)

	links, err := Apply(proj)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Link{}
	for _, l := range links {
		if l.Tool == "claude" {
			got[l.Skill] = l
		}
	}
	if got["a"].State != "linked" || got["a"].Local {
		t.Errorf("global skill: %+v", got["a"])
	}
	if l := got["house-style"]; l.State != "linked" || !l.Local {
		t.Errorf("project skill: %+v", l)
	}
	if b, err := os.ReadFile(filepath.Join(proj, ".claude/skills/house-style/SKILL.md")); err != nil || !strings.Contains(string(b), "house-style") {
		t.Error("project skill not reachable through the link")
	}
	if t2, _ := os.Readlink(filepath.Join(proj, ".claude/skills/house-style")); t2 != "../../.arig/skills/house-style" {
		t.Errorf("project skill link should stay inside the repo: %q", t2)
	}

	// pinning moves the old manifest into .arig/
	if _, err := Edit(proj, true, "a"); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(proj, oldManifest)) || !exists(filepath.Join(proj, ManifestFile)) {
		t.Error("manifest not migrated into .arig/")
	}
	if !exists(filepath.Join(proj, lockFile)) || exists(filepath.Join(proj, oldLock)) {
		t.Error("lock should live in .arig/")
	}

	// removing the project skill folder unlinks it on the next apply
	os.RemoveAll(own)
	if _, err := Apply(proj); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(proj, ".claude/skills/house-style")) {
		t.Error("stale project-skill link left behind")
	}
}
