package market

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fake is a skills.sh stand-in.
type fake struct {
	mu       sync.Mutex
	searches map[string][]Skill // q → results
	dls      map[string]Download
	hits     int
	srv      *httptest.Server
}

func newFake(t *testing.T) *fake {
	f := &fake{searches: map[string][]Skill{}, dls: map[string]Download{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits++
		if r.Header.Get("User-Agent") != UserAgent {
			http.Error(w, "no UA", 400)
			return
		}
		if r.URL.Path == "/api/search" {
			q := r.URL.Query().Get("q")
			if len(q) < 2 {
				json.NewEncoder(w).Encode(map[string]string{"error": "short"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"skills": f.searches[q]})
			return
		}
		if id, ok := strings.CutPrefix(r.URL.Path, "/api/download/"); ok {
			if d, ok := f.dls[id]; ok {
				json.NewEncoder(w).Encode(d)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) market(t *testing.T) (*Market, string) {
	home := t.TempDir()
	return &Market{Dir: filepath.Join(home, "cfg"), Base: f.srv.URL, HTTP: f.srv.Client(), Version: "v1.0.0",
		SkillDir: func(tool string) string {
			if tool == "none" {
				return ""
			}
			return filepath.Join(home, tool, "skills")
		}}, home
}

func sk(source, id string, installs int) Skill {
	return Skill{Source: source, SkillID: id, Name: id, Installs: installs}
}

func TestSearch(t *testing.T) {
	f := newFake(t)
	f.searches["video"] = []Skill{sk("a/b", "small", 5), sk("a/b", "big-video", 500), sk("../x", "evil", 9), sk("a/b", "sp ace", 1)}
	m, _ := f.market(t)

	got, err := m.Search(" video ", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a/b/big-video" || got[1].ID != "a/b/small" || got[0].Category != "media/video" {
		t.Fatalf("%+v", got)
	}
	if _, err := m.Search("video", 10); err != nil || f.hits != 1 {
		t.Fatalf("not cached: %v %d", err, f.hits)
	}
	if _, err := m.Search("v", 10); err == nil || f.hits != 1 {
		t.Fatal("1-char query should fail locally")
	}
	f.searches["design"] = []Skill{sk("a/b", "small", 5), sk("c/d", "ui", 50)}
	all, err := m.SearchMany([]string{"video", "design"}, 10)
	if err != nil || len(all) != 3 || all[0].Installs != 500 || all[1].ID != "c/d/ui" {
		t.Fatalf("%v %+v", err, all)
	}
	if ts := CategoryTerms("media/video"); !slices.Equal(ts, []string{"video"}) {
		t.Fatal(ts)
	}
	if ts := CategoryTerms("coding"); len(ts) != 8 || ts[3] != "python" {
		t.Fatal(ts)
	}
	for _, bad := range []string{"a/b", "a/b/c/d", "a/../c", "a/b/.x", "a/b/c d", ""} {
		if _, _, _, err := SplitID(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestFlags(t *testing.T) {
	for _, c := range []struct {
		files []File
		want  string
	}{
		{[]File{{"SKILL.md", "plain docs, see https://example.com"}}, ""},
		{[]File{{"SKILL.md", "x"}, {"scripts/a.py", "import os"}}, "scripts"},
		{[]File{{"SKILL.md", "x"}, {"run.sh", "curl -s x"}}, "scripts,network"},
		{[]File{{"SKILL.md", "x"}, {"tool", "#!/bin/sh\nwget x"}}, "scripts,network"},
		{[]File{{"SKILL.md", "Install: curl -fsSL x | sh"}}, "shell-pipe"},
		{[]File{{"SKILL.md", "irm x | iex"}}, "shell-pipe"},
		{[]File{{"SKILL.md", "an index of things"}}, ""},
		{[]File{{"SKILL.md", "x"}, {"bin/tool", "\x7fELF..."}}, "binary"},
		{[]File{{"SKILL.md", "x"}, {"a.dat", "ab\x00cd"}}, "binary"},
		{[]File{{"SKILL.md", strings.Repeat("x", 1<<20+1)}}, "large"},
	} {
		if got := strings.Join(Flags(c.files), ","); got != c.want {
			t.Errorf("%v: got %q want %q", c.files[len(c.files)-1].Path, got, c.want)
		}
	}
}

func TestWriteFilesConfinement(t *testing.T) {
	dst := t.TempDir()
	ok := []File{{"SKILL.md", "---\nname: foo\n---\n"}, {"sub/a.txt", "a"}, {".env.local", "SECRET=1"}, {"x/.git", "g"}}
	if err := writeFiles(ok, dst); err != nil {
		t.Fatal(err)
	}
	got, _ := files(dst)
	if len(got) != 2 || got["SKILL.md"] == "" || got["sub/a.txt"] == "" {
		t.Fatalf("wrote %v", got)
	}
	for _, bad := range []string{"../escape", "/etc/passwd", "a/../../x", "a/./b", "a//b", `a\..\..\x`, "C:x", "", "."} {
		err := writeFiles([]File{{"SKILL.md", "x"}, {bad, "x"}}, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Errorf("%q: got %v", bad, err)
		}
	}
	if err := writeFiles([]File{{"README.md", "x"}}, t.TempDir()); err == nil {
		t.Error("missing SKILL.md should fail")
	}
	if err := writeFiles([]File{{"SKILL.md", "x"}, {"big", strings.Repeat("x", maxSkill)}}, t.TempDir()); err == nil {
		t.Error("over the size cap should fail")
	}
}

func TestInstallUpdateUninstall(t *testing.T) {
	f := newFake(t)
	v1 := Download{Hash: "aaa1", Files: []File{{"SKILL.md", "---\nname: Foo\ndescription: does foo\n---\nv1\n"}, {"scripts/run.py", "print(1)"}}}
	f.dls["o/r/foo"] = v1
	m, home := f.market(t)

	d, err := m.Describe("o/r/foo")
	if err != nil || d.Description != "does foo" || len(d.Files) != 2 || !slices.Equal(d.Flags, []string{"scripts"}) || d.Page != "https://skills.sh/o/r/foo" {
		t.Fatalf("%v %+v", err, d)
	}
	if err := m.Install("o/r/foo", []string{"claude"}, false); !errors.Is(err, ErrConfirm) {
		t.Fatalf("flags not enforced: %v", err)
	}
	if err := m.Install("o/r/foo", []string{"claude", "codex"}, true); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(home, "claude", "skills", "foo", "SKILL.md")
	if _, err := os.Stat(claude); err != nil {
		t.Fatal(err)
	}
	le := m.Lock()["o/r/foo"]
	if le.Hash != "aaa1" || le.Source != "o/r" || le.SkillID != "foo" || le.Name != "Foo" || len(le.Tools) != 2 {
		t.Fatalf("%+v", le)
	}
	if err := m.Install("x/y/foo", []string{"claude"}, true); err == nil {
		t.Error("same folder name installed twice")
	}
	if u := m.CheckUpdates(); len(u) != 0 {
		t.Fatalf("no update expected: %+v", u)
	}

	// a new version upstream
	f.dls["o/r/foo"] = Download{Hash: "bbb2", Files: []File{{"SKILL.md", "---\nname: Foo\n---\nv2\n"}}}
	m.cache = nil
	u := m.CheckUpdates()
	if len(u) != 1 || u[0].From != "aaa1" || u[0].To != "bbb2" || len(m.Updates()) != 1 {
		t.Fatalf("%+v", u)
	}
	diff, flags, err := m.Review("o/r/foo")
	if err != nil || !strings.Contains(diff, "-v1\n+v2") || !strings.Contains(diff, "--- a/scripts/run.py") || len(flags) != 0 {
		t.Fatalf("%v %v\n%s", err, flags, diff)
	}
	// a tool disabled it meanwhile: the update lands there
	os.MkdirAll(filepath.Join(home, "codex", "skills-disabled"), 0o755)
	os.Rename(filepath.Join(home, "codex", "skills", "foo"), filepath.Join(home, "codex", "skills-disabled", "foo"))
	if err := m.Apply("o/r/foo", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, "codex", "skills-disabled", "foo", "SKILL.md"))
	if !strings.Contains(string(b), "v2") || m.Lock()["o/r/foo"].Hash != "bbb2" || len(m.Updates()) != 0 {
		t.Fatal("update not applied")
	}
	if _, err := os.Stat(filepath.Join(home, "claude", "skills", "foo", "scripts")); err == nil {
		t.Error("removed file survived the update")
	}
	if _, _, err := m.Review("o/r/foo"); err == nil {
		t.Error("up to date should not review")
	}

	other := filepath.Join(home, "claude", "skills", "mine")
	os.MkdirAll(other, 0o755)
	if err := m.Uninstall("o/r/foo"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{claude, filepath.Join(home, "codex", "skills-disabled", "foo"), m.canon("foo")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s still there", p)
		}
	}
	if _, err := os.Stat(other); err != nil || len(m.Lock()) != 0 {
		t.Error("uninstall touched another skill or left the lock")
	}

	// unsafe download: nothing lands
	f.dls["o/r/bad"] = Download{Hash: "ccc", Files: []File{{"SKILL.md", "x"}, {"../../escape", "x"}}}
	if err := m.Install("o/r/bad", []string{"claude"}, true); err == nil {
		t.Fatal("unsafe path installed")
	}
	if _, err := os.Stat(filepath.Join(home, "claude", "skills", "bad")); err == nil || len(m.Lock()) != 0 {
		t.Fatal("partial install left behind")
	}
	if err := m.Install("o/r/missing", []string{"claude"}, true); err == nil {
		t.Fatal("404 installed")
	}
}

func TestRecommend(t *testing.T) {
	f := newFake(t)
	f.searches["react"] = []Skill{sk("a/b", "react-hooks", 10), sk("a/b", "mine", 99)}
	f.searches["golang"] = []Skill{sk("c/d", "golang-style", 50), sk("a/b", "react-hooks", 10), sk("x/y", "popular-but-fuzzy", 9999)}
	f.searches["go"] = []Skill{sk("c/e", "go-tips", 5), sk("x/z", "google-docs", 9999)}
	m, _ := f.market(t)
	sig := []Signal{{"your project uses React", []string{"react"}}, {"your project uses Go", []string{"golang", "go"}}}
	got, err := m.Recommend(sig, func(s Skill) bool { return s.SkillID == "mine" }, 10)
	if err != nil || len(got) != 3 || got[0].ID != "c/d/golang-style" || got[1].ID != "a/b/react-hooks" || len(got[1].Reasons) != 1 || got[2].ID != "c/e/go-tips" {
		t.Fatalf("%v %+v", err, got)
	}

	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "package.json"), []byte(`{"dependencies":{"react":"19"},"devDependencies":{"@sveltejs/kit":"2"}}`), 0o644)
	os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x"), 0o644)
	os.MkdirAll(filepath.Join(d, ".github", "workflows"), 0o755)
	os.MkdirAll(filepath.Join(d, "node_modules", "x"), 0o755)
	os.WriteFile(filepath.Join(d, "node_modules", "x", "Cargo.toml"), nil, 0o644)
	var reasons, terms []string
	for _, s := range Scan(d) {
		reasons = append(reasons, s.Reason)
		terms = append(terms, s.Terms...)
	}
	want := "your project uses GitHub Actions,your project uses Go,your project uses JavaScript,your project uses React,your project uses Svelte"
	if strings.Join(reasons, ",") != want || !slices.Contains(terms, "golang") || !slices.Contains(terms, "react") {
		t.Fatalf("got %v %v", reasons, terms)
	}

	home := t.TempDir()
	in := filepath.Join(home, "p")
	os.Mkdir(in, 0o755)
	if _, err := ProjectDir(in, home); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{t.TempDir(), home, "rel/path", filepath.Join(home, "missing")} {
		if _, err := ProjectDir(bad, home); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestCheckLatest(t *testing.T) {
	status := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 200 {
			http.Error(w, "nope", status)
			return
		}
		w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"https://github.com/tabhishekpaul/agentrig/releases/tag/v1.2.0"}`))
	}))
	defer srv.Close()
	m := &Market{ReleasesURL: srv.URL, HTTP: srv.Client(), Version: "v1.1.0"}
	if err := m.CheckLatest(); err != nil || m.Upgrade() != nil {
		t.Fatal("no release yet must mean no banner", err)
	}
	status = 200
	if err := m.CheckLatest(); err != nil || m.Upgrade() == nil || m.Upgrade().Version != "v1.2.0" {
		t.Fatal("release not seen", err)
	}
	m.Version = "dev"
	if m.Upgrade() != nil {
		t.Fatal("dev nagged")
	}
}

func TestUnified(t *testing.T) {
	a := strings.Split("1 2 3 4 5 6 7 8 9 10 11 12", " ")
	b := strings.Split("1 2 3 4 X 6 7 8 9 10 11 12 13", " ")
	got := Unified("f", a, b)
	want := "--- a/f\n+++ b/f\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+X\n 6\n 7\n 8\n@@ -10,3 +10,4 @@\n 10\n 11\n 12\n+13\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"v1.2.3", "v1.2.4", true},
		{"v1.2.3", "v1.10.0", true},
		{"v1.2.3", "v1.2.3", false},
		{"v2.0.0", "v1.9.9", false},
		{"v1.0.0-rc.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0-rc.1", false},
		{"dev", "v9.9.9", false},
		{"v1.0.0", "garbage", false},
	} {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.cur, c.latest, got)
		}
	}
}
