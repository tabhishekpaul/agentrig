package authoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/library"
)

// isolate points HOME (and so every tool dir and the git config) at a temp dir.
func isolate(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func TestCreateLinksAndRefusesOverwrite(t *testing.T) {
	home := isolate(t)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	dirs := adapters.SkillDirs()
	s := Skill{ID: "my-skill", Description: `Does "things": well`, Category: "coding/git", Tags: []string{"git", "x"},
		Body: "Do it.", Tools: []string{"claude", "codex"}}
	dir, err := Create(home, dirs, s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, filepath.FromSlash(".arig/library/my-skill/")) {
		t.Fatalf("library entry %s", dir)
	}
	// every chosen tool sees it through a relative link into the library
	link := filepath.Join(home, ".claude/skills/my-skill")
	for _, l := range []string{link, filepath.Join(home, ".codex/skills/my-skill")} {
		if target, err := os.Readlink(l); err != nil || filepath.IsAbs(target) || !library.IsLibraryLink(l) {
			t.Fatalf("want relative library link at %s, got %q %v", l, target, err)
		}
	}
	fm := adapters.Frontmatter(filepath.Join(link, "SKILL.md"))
	if fm["name"] != "my-skill" || fm["category"] != "coding/git" || fm["tags"] != "[git, x]" || !strings.Contains(fm["description"], "things") {
		t.Fatalf("frontmatter %v", fm)
	}
	if exists(filepath.Join(home, ".agents/skills/my-skill")) {
		t.Fatal("codex got a second link")
	}
	// no tools chosen: linked into the tool-neutral ~/.agents/skills
	if _, err := Create(home, dirs, Skill{ID: "loner", Description: "d"}); err != nil || !library.IsLibraryLink(filepath.Join(home, ".agents/skills/loner")) {
		t.Fatal("no-tool skill not linked into ~/.agents/skills", err)
	}

	// editing makes a new version; both tool links move, the old entry stays as it was
	if err := Save(link, "---\nname: my-skill\n---\nv2\n", dirs); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "SKILL.md")); strings.Contains(string(b), "v2") {
		t.Fatal("library entry edited in place")
	}
	for _, l := range []string{link, filepath.Join(home, ".codex/skills/my-skill")} {
		if b, _ := os.ReadFile(filepath.Join(l, "SKILL.md")); !strings.Contains(string(b), "v2") {
			t.Fatalf("%s not moved to the new version", l)
		}
	}
	if len(library.Versions("my-skill")) != 2 {
		t.Fatal("want two versions")
	}
	ids := []string{}
	for _, e := range catalog.Local(adapters.Detect()) {
		ids = append(ids, e.ID)
	}
	if !slices.Contains(ids, "my-skill") {
		t.Fatalf("not detected: %v", ids)
	}

	if _, err := Create(home, dirs, s); err == nil {
		t.Fatal("overwrote an existing skill")
	}
	for _, bad := range []string{"../x", "My Skill", "a/b", ""} {
		if _, err := Create(home, dirs, Skill{ID: bad, Description: "d"}); err == nil {
			t.Fatalf("accepted id %q", bad)
		}
	}
}

func TestTogglePack(t *testing.T) {
	home := isolate(t)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	dirs := adapters.SkillDirs()
	for _, id := range []string{"a", "b"} {
		if _, err := Create(home, dirs, Skill{ID: id, Description: "d", Tools: []string{"claude"}}); err != nil {
			t.Fatal(err)
		}
	}
	p := catalog.Pack{ID: "p", Skills: []string{"a", "b", "missing"}}
	if _, err := TogglePack(p, dirs, false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".claude/skills-disabled/a", ".claude/skills-disabled/b"} {
		if !exists(filepath.Join(home, rel)) {
			t.Fatalf("%s not disabled", rel)
		}
	}
	if _, err := TogglePack(p, dirs, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude/skills/a/SKILL.md")); err != nil {
		t.Fatal("not re-enabled through the link:", err)
	}

	store := &Packs{Path: filepath.Join(home, "cfg/packs.json")}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(catalog.Pack{ID: "../x"}); err == nil {
		t.Fatal("bad pack id saved")
	}
	if got, _ := store.Get("p"); len(got.Skills) != 3 {
		t.Fatalf("got %+v", got)
	}
	store.Delete("p")
	if ps, _ := store.List(); len(ps) != 0 {
		t.Fatal("not deleted")
	}
}

func skillFixture(t *testing.T, root string) string {
	src := filepath.Join(root, "src/s")
	os.MkdirAll(filepath.Join(src, "scripts"), 0o755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: s\ndescription: d\n---\nbody\n"), 0o644)
	os.WriteFile(filepath.Join(src, "scripts/run.sh"), []byte("echo hi\n"), 0o755)
	os.WriteFile(filepath.Join(src, ".env"), []byte("TOKEN=secret"), 0o644)
	os.Symlink("/etc/passwd", filepath.Join(src, "leak"))
	return src
}

func TestWritePluginLayout(t *testing.T) {
	home := isolate(t)
	src := skillFixture(t, home)
	if _, err := ExportDir(home, "/tmp/elsewhere"); err == nil {
		t.Fatal("export outside home allowed")
	}
	if _, err := ExportDir(home, "~/src"); err == nil {
		t.Fatal("non-empty export dir allowed")
	}
	dir, err := ExportDir(home, "~/out/my-pack")
	if err != nil {
		t.Fatal(err)
	}
	p := catalog.Pack{ID: "my-pack", Name: "My pack", Description: "two", Skills: []string{"s"}}
	find := func(id string) string { return map[string]string{"s": src}[id] }
	if err := WritePlugin(dir, p, "Ann", find); err != nil {
		t.Fatal(err)
	}
	var m Manifest
	b, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin/plugin.json"))
	if json.Unmarshal(b, &m) != nil || m.Name != "my-pack" || m.Author.Name != "Ann" || m.Version == "" {
		t.Fatalf("manifest %s", b)
	}
	if !exists(filepath.Join(dir, "skills/s/SKILL.md")) || !exists(filepath.Join(dir, "skills/s/scripts/run.sh")) {
		t.Fatal("skill files missing")
	}
	if exists(filepath.Join(dir, "skills/s/.env")) || exists(filepath.Join(dir, "skills/s/leak")) {
		t.Fatal(".env or symlink exported")
	}
	if err := WritePlugin(t.TempDir(), catalog.Pack{ID: "claude-tools", Skills: []string{"s"}}, "", find); err == nil {
		t.Fatal("reserved plugin name allowed")
	}
}

func TestPublishWithStubGH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	home := isolate(t)
	src := skillFixture(t, home)
	gitcfg := filepath.Join(home, "gitconfig")
	os.WriteFile(gitcfg, []byte("[user]\n\tname = Ann\n\temail = ann@example.com\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", gitcfg)

	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	os.WriteFile(filepath.Join(bin, "gh"), []byte(`#!/bin/sh
echo "$@" >> "`+log+`"
[ "$1" = api ] && echo stubuser
exit 0
`), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	plan, dir, err := Prepare(Item{Kind: "skill", ID: "my-skill", Path: src, Description: "Does things"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if plan.Repo != "stubuser/my-skill" || slices.Contains(plan.Files, ".env") || !slices.Contains(plan.Files, "SKILL.md") {
		t.Fatalf("plan %+v", plan)
	}
	url, err := Publish(plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/stubuser/my-skill" {
		t.Fatal(url)
	}
	got, _ := os.ReadFile(log)
	for _, want := range []string{"repo create my-skill --public --description Does things --source . --remote origin --push",
		"repo edit stubuser/my-skill --add-topic arig-skill"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("gh calls:\n%s\nmissing %q", got, want)
		}
	}
	if out, err := run(dir, 5e9, "git", "log", "--format=%an"); err != nil || out != "Ann" {
		t.Fatalf("commit author %q %v", out, err)
	}

	pp, pdir, err := Prepare(Item{Kind: "pack", ID: "my-pack", Pack: catalog.Pack{ID: "my-pack", Skills: []string{"s"}},
		SkillPath: func(string) string { return src }})
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(pdir)
	if !slices.Contains(pp.Topics, TopicPack) || !slices.Contains(pp.Files, ".claude-plugin/plugin.json") {
		t.Fatalf("pack plan %+v", pp)
	}
}
