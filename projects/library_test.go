package projects

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tabhishekpaul/agentrig/library"
	"github.com/tabhishekpaul/agentrig/market"
)

func pins(t *testing.T, proj string) map[string]string {
	t.Helper()
	l := map[string]string{}
	b, _ := os.ReadFile(filepath.Join(proj, LockFile))
	json.Unmarshal(b, &l)
	return l
}

// libSkill puts a skill version into the library, aged so "newest" is clear.
func libSkill(t *testing.T, name, body string, age time.Duration) (string, string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "s")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: "+filepath.Base(name)+"\n---\n"+body), 0o644)
	p, h, err := library.Put(src, name)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	os.Chtimes(p, when, when)
	return p, h
}

func linkOf(t *testing.T, proj string, ls []Link, path string) Link {
	t.Helper()
	for _, l := range ls {
		if l.Path == path {
			return l
		}
	}
	t.Fatalf("no link %s in %+v", path, ls)
	return Link{}
}

func resolvesTo(t *testing.T, p, entry string) {
	t.Helper()
	got, err := filepath.EvalSymlinks(p)
	want, _ := filepath.EvalSymlinks(entry)
	if err != nil || got != want {
		t.Fatalf("%s resolves to %s, want %s (%v)", p, got, want, err)
	}
}

func TestResolveFromLibrary(t *testing.T) {
	proj, skills := setup(t)
	pdf, h := libSkill(t, "pdf", "v1", 0)
	if _, err := Edit(proj, true, "pdf", "a"); err != nil {
		t.Fatal(err)
	}
	ls, err := Apply(proj)
	if err != nil {
		t.Fatal(err)
	}
	l := linkOf(t, proj, ls, ".claude/skills/pdf")
	if l.State != "linked" || l.Version != h[:8] || l.Source != "library" {
		t.Fatalf("%+v", l)
	}
	resolvesTo(t, filepath.Join(proj, ".claude/skills/pdf"), pdf)
	if target, _ := os.Readlink(filepath.Join(proj, ".claude/skills/pdf")); filepath.IsAbs(target) {
		t.Fatal("absolute link")
	}
	// a global skill not in the library yet is stored there, then linked from it
	got := pins(t, proj)
	if got["pdf"] != h || got["a"] == "" {
		t.Fatalf("arig.lock %v", got)
	}
	resolvesTo(t, filepath.Join(proj, ".cursor/skills/a"), library.Path("a", got["a"]))
	if _, err := os.Stat(filepath.Join(skills, "a", "SKILL.md")); err != nil {
		t.Fatal("global skill touched")
	}
	gi, _ := os.ReadFile(filepath.Join(proj, ".gitignore"))
	if strings.Contains(string(gi), "arig.lock") || !strings.Contains(string(gi), "/.claude/skills/pdf\n") {
		t.Fatalf(".gitignore:\n%s", gi)
	}
	// Status agrees without changing anything
	st, _ := Status(proj)
	if l := linkOf(t, proj, st, ".cursor/skills/pdf"); l.State != "linked" || l.Version != h[:8] {
		t.Fatalf("%+v", l)
	}
}

func TestLockPinsVersion(t *testing.T) {
	proj, _ := setup(t)
	v1, h1 := libSkill(t, "pdf", "v1", time.Hour)
	Edit(proj, true, "pdf")
	if _, err := Apply(proj); err != nil || pins(t, proj)["pdf"] != h1 {
		t.Fatal(err, pins(t, proj))
	}
	// a newer version arrives: the lock keeps the project on v1
	v2, h2 := libSkill(t, "pdf", "v2", 0)
	if _, err := Apply(proj); err != nil || pins(t, proj)["pdf"] != h1 {
		t.Fatal(err)
	}
	resolvesTo(t, filepath.Join(proj, ".claude/skills/pdf"), v1)
	// the team bumps the lock (git pull): apply follows it
	os.WriteFile(filepath.Join(proj, LockFile), []byte(`{"pdf":"`+h2+`"}`), 0o644)
	st, _ := Status(proj)
	if l := linkOf(t, proj, st, ".claude/skills/pdf"); l.State != "missing" || l.Version != h2[:8] {
		t.Fatalf("stale link should show as missing: %+v", l)
	}
	if _, err := Apply(proj); err != nil {
		t.Fatal(err)
	}
	resolvesTo(t, filepath.Join(proj, ".claude/skills/pdf"), v2)
	// a locked version this machine lacks: newest local one, re-locked
	os.WriteFile(filepath.Join(proj, LockFile), []byte(`{"pdf":"0123456789ab"}`), 0o644)
	if _, err := Apply(proj); err != nil || pins(t, proj)["pdf"] != h2 {
		t.Fatal(err, pins(t, proj))
	}
	// a lock value that isn't a hash never reaches a path
	os.WriteFile(filepath.Join(proj, LockFile), []byte(`{"pdf":"../../../etc"}`), 0o644)
	if _, err := Apply(proj); err != nil || pins(t, proj)["pdf"] != h2 {
		t.Fatal(err, pins(t, proj))
	}
	// unpinned: arig.lock goes away with the last pin
	Edit(proj, false, "pdf")
	Apply(proj)
	if exists(filepath.Join(proj, LockFile)) || exists(filepath.Join(proj, ".claude/skills/pdf")) {
		t.Fatal("lock or link left behind")
	}
}

func TestSkillsShPins(t *testing.T) {
	dls := map[string]market.Download{
		"o/r/clean": {Hash: "aa", Files: []market.File{{Path: "SKILL.md", Contents: "---\nname: clean\n---\nplain"}}},
		"o/r/risky": {Hash: "bb", Files: []market.File{{Path: "SKILL.md", Contents: "x"}, {Path: "run.sh", Contents: "curl x"}}},
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if d, ok := dls[strings.TrimPrefix(r.URL.Path, "/api/download/")]; ok {
			json.NewEncoder(w).Encode(d)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("ARIG_SKILLS_URL", srv.URL)

	proj, _ := setup(t)
	if _, err := Edit(proj, true, "o/r/clean", "o/r/risky"); err != nil {
		t.Fatal(err)
	}
	ls, err := Apply(proj)
	if err != nil {
		t.Fatal(err)
	}
	clean := linkOf(t, proj, ls, ".claude/skills/clean")
	if clean.State != "linked" || clean.Source != "skills.sh" || clean.Skill != "o/r/clean" {
		t.Fatalf("%+v", clean)
	}
	resolvesTo(t, filepath.Join(proj, ".claude/skills/clean"), library.Path("o/r/clean", pins(t, proj)["o/r/clean"]))
	risky := linkOf(t, proj, ls, ".claude/skills/risky")
	if risky.State != "needs-confirm" || !slices.Equal(risky.Flags, []string{"scripts", "network"}) {
		t.Fatalf("%+v", risky)
	}
	if exists(filepath.Join(proj, ".claude/skills/risky")) || pins(t, proj)["o/r/risky"] != "" || len(library.Versions("o/r/risky")) != 0 {
		t.Fatal("flagged skill installed without confirmation")
	}
	// accepted: installed and locked
	ls, err = Apply(proj, "o/r/risky")
	if err != nil || linkOf(t, proj, ls, ".cursor/skills/risky").State != "linked" || pins(t, proj)["o/r/risky"] == "" {
		t.Fatal(err, ls)
	}
	// already in the library: no download on the next apply
	before := hits
	if _, err := Apply(proj); err != nil || hits != before {
		t.Fatal("re-downloaded", err)
	}

	// a teammate (empty library) clones the project: the lock is the
	// acceptance, so the flagged skill installs without asking again
	t.Setenv("HOME", t.TempDir())
	os.Remove(filepath.Join(proj, lockFile)) // machine-local, not committed
	for _, p := range []string{".claude/skills/clean", ".claude/skills/risky", ".cursor/skills/clean", ".cursor/skills/risky"} {
		os.Remove(filepath.Join(proj, p))
	}
	ls, err = Apply(proj)
	if err != nil || linkOf(t, proj, ls, ".claude/skills/risky").State != "linked" {
		t.Fatal(err, ls)
	}
}

func TestProjectCopyFallback(t *testing.T) {
	proj, _ := setup(t)
	copyLinks = true
	defer func() { copyLinks = runtime.GOOS == "windows" }()
	libSkill(t, "pdf", "v1", 0)
	Edit(proj, true, "pdf")
	ls, err := Apply(proj)
	if err != nil || linkOf(t, proj, ls, ".claude/skills/pdf").State != "linked" {
		t.Fatal(err, ls)
	}
	p := filepath.Join(proj, ".claude/skills/pdf")
	if fi, _ := os.Lstat(p); fi.Mode()&os.ModeSymlink != 0 || !exists(filepath.Join(p, copyMarker)) {
		t.Fatal("want a marked copy")
	}
	// a new locked version replaces the copy
	_, h2 := libSkill(t, "pdf", "v2", -time.Hour)
	os.WriteFile(filepath.Join(proj, LockFile), []byte(`{"pdf":"`+h2+`"}`), 0o644)
	if _, err := Apply(proj); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(p, "SKILL.md")); !strings.Contains(string(b), "v2") {
		t.Fatalf("copy not updated: %s", b)
	}
}
