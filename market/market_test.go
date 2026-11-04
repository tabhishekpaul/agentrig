package market

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/catalog"
)

type file struct {
	name, body string
	typ        byte
}

func tgz(t *testing.T, files []file) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	tw.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "x"}})
	for _, f := range files {
		h := &tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: f.typ}
		if f.typ == 0 {
			h.Typeflag = tar.TypeReg
		}
		if f.typ == tar.TypeSymlink {
			h.Linkname, h.Size = f.body, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(f.body))
		}
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func TestExtractConfinement(t *testing.T) {
	good := []file{
		{name: "o-r-sha/README.md", body: "outside"},
		{name: "o-r-sha/skills/foo/SKILL.md", body: "---\nname: foo\n---\n"},
		{name: "o-r-sha/skills/foo/.env.local", body: "SECRET=1"},
		{name: "o-r-sha/skills/foo/link", body: "../../../etc/passwd", typ: tar.TypeSymlink},
		{name: "o-r-sha/skills/foo/sub/a.txt", body: "a"},
		{name: "o-r-sha/skills/foobar/x", body: "sibling with same prefix"},
	}
	dst := t.TempDir()
	if err := extract(bytes.NewReader(tgz(t, good)), "skills/foo", dst); err != nil {
		t.Fatal(err)
	}
	got, _ := files(dst)
	if len(got) != 2 || got["SKILL.md"] == "" || got["sub/a.txt"] == "" {
		t.Fatalf("extracted %v", got)
	}
	if _, err := os.Lstat(filepath.Join(dst, "link")); err == nil {
		t.Fatal("symlink was created")
	}

	for _, bad := range []string{"o-r-sha/skills/foo/../../../escape", "o-r-sha/skills/foo/a/../../x/../../../y"} {
		err := extract(bytes.NewReader(tgz(t, []file{{name: bad, body: "x"}})), "skills/foo", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Errorf("%s: got %v", bad, err)
		}
	}
	if err := extract(bytes.NewReader(tgz(t, good)), "skills/nope", t.TempDir()); err == nil {
		t.Error("missing folder should fail")
	}
	big := []file{{name: "o-r-sha/s/SKILL.md", body: "x"}, {name: "o-r-sha/s/big", body: strings.Repeat("x", maxSkill)}}
	if err := extract(bytes.NewReader(tgz(t, big)), "s", t.TempDir()); err == nil {
		t.Error("oversized skill should fail")
	}
}

func TestValidate(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, e := range []catalog.Entry{
		{Repo: "o/r", Dir: "../x", Ref: sha},
		{Repo: "o/r", Dir: "/abs", Ref: sha},
		{Repo: "../r", Ref: sha},
		{Repo: "o/r", Dir: "x", Ref: "main"},
	} {
		if _, err := validate(e); err == nil {
			t.Errorf("%+v should be rejected", e)
		}
	}
	if n, err := validate(catalog.Entry{Repo: "o/repo", Ref: sha}); n != "repo" || err != nil {
		t.Errorf("repo root: %q %v", n, err)
	}
}

// fixture serves an index and codeload tarballs for two commits of o/r.
type fixture struct {
	srv   *httptest.Server
	index catalog.Index
	trees map[string][]file // ref -> files
	gets  int
}

const ref1, ref2 = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"

func newFixture(t *testing.T) (*fixture, *Market, string) {
	f := &fixture{trees: map[string][]file{
		ref1: {{name: "o-r-1/skills/foo/SKILL.md", body: "---\nname: foo\n---\nline1\nline2\n"}},
		ref2: {{name: "o-r-2/skills/foo/SKILL.md", body: "---\nname: foo\n---\nline1\nline2 changed\n"}, {name: "o-r-2/skills/foo/new.md", body: "new\n"}},
	}}
	f.index = catalog.Index{Version: 1, Skills: []catalog.Entry{
		{ID: "o/r/skills/foo", Name: "foo", Repo: "o/r", Dir: "skills/foo", Ref: ref1, Stars: 5},
		{ID: "o/r/skills/bar", Name: "bar", Repo: "o/r", Dir: "skills/bar", Ref: ref1, Flags: []string{"scripts"}},
	}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			f.gets++
			etag := `"` + f.index.Skills[0].Ref + `"`
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(304)
				return
			}
			w.Header().Set("ETag", etag)
			json.NewEncoder(w).Encode(f.index)
			return
		}
		ref := filepath.Base(r.URL.Path)
		if tree, ok := f.trees[ref]; ok && strings.HasPrefix(r.URL.Path, "/o/r/tar.gz/") {
			w.Write(tgz(t, tree))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.srv.Close)
	home := t.TempDir()
	m := &Market{Dir: filepath.Join(home, "cfg"), IndexURL: f.srv.URL + "/index.json", Codeload: f.srv.URL,
		HTTP: f.srv.Client(), SkillDir: func(tool string) string {
			if tool == "none" {
				return ""
			}
			return filepath.Join(home, tool, "skills")
		}}
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	return f, m, home
}

func TestInstallUpdateUninstall(t *testing.T) {
	f, m, home := newFixture(t)
	claude := filepath.Join(home, "claude", "skills", "foo")
	codex := filepath.Join(home, "codex", "skills", "foo")

	if err := m.Install("o/r/skills/bar", []string{"claude"}, false); !errors.Is(err, ErrConfirm) {
		t.Fatalf("flagged install without confirm: %v", err)
	}
	if err := m.Install("o/r/skills/foo", []string{"none"}, false); err == nil {
		t.Fatal("tool without skills dir should fail")
	}
	// a skill of the same name the user already has
	os.MkdirAll(filepath.Join(home, "codex", "skills-disabled", "foo"), 0o755)
	if err := m.Install("o/r/skills/foo", []string{"claude", "codex"}, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("clash: %v", err)
	}
	os.RemoveAll(filepath.Join(home, "codex", "skills-disabled"))

	if err := m.Install("o/r/skills/foo", []string{"claude", "codex"}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{claude, codex, m.canon("foo")} {
		if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
	le := m.Lock()["o/r/skills/foo"]
	if le.Ref != ref1 || le.Name != "foo" || len(le.Tools) != 2 {
		t.Fatalf("lock: %+v", le)
	}
	if len(m.Updates()) != 0 {
		t.Fatal("no update expected")
	}

	// new commit published; user had disabled it in codex meanwhile
	os.MkdirAll(filepath.Join(home, "codex", "skills-disabled"), 0o755)
	os.Rename(codex, filepath.Join(home, "codex", "skills-disabled", "foo"))
	f.index.Skills[0].Ref = ref2
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if u := m.Updates(); len(u) != 1 || u[0].To != ref2 {
		t.Fatalf("updates: %+v", u)
	}
	d, err := m.Review("o/r/skills/foo")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-line2\n", "+line2 changed\n", "+++ b/new.md", "+new\n"} {
		if !strings.Contains(d, want) {
			t.Errorf("diff missing %q:\n%s", want, d)
		}
	}
	if m.Lock()["o/r/skills/foo"].Ref != ref1 {
		t.Fatal("review must not apply")
	}
	if err := m.Apply("o/r/skills/foo", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "codex", "skills-disabled", "foo", "new.md")); err != nil {
		t.Fatal("disabled copy not updated")
	}
	if _, err := os.Stat(codex); err == nil {
		t.Fatal("update re-enabled a disabled copy")
	}
	if m.Lock()["o/r/skills/foo"].Ref != ref2 {
		t.Fatal("lock not updated")
	}

	// uninstall removes only what it installed
	other := filepath.Join(home, "claude", "skills", "mine")
	os.MkdirAll(other, 0o755)
	if err := m.Uninstall("o/r/skills/foo"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{claude, filepath.Join(home, "codex", "skills-disabled", "foo"), m.canon("foo")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s still there", p)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("uninstall touched another skill")
	}
	if len(m.Lock()) != 0 {
		t.Error("lock not cleared")
	}
}

func TestIndexFetch(t *testing.T) {
	f, m, _ := newFixture(t)
	if s := m.Status(); s.State != "ok" || len(m.Index().Skills) != 2 {
		t.Fatalf("%+v", s)
	}
	if err := m.Refresh(); err != nil || f.gets != 2 || m.Status().State != "ok" {
		t.Fatalf("304: %v %d", err, f.gets)
	}
	// a fresh Market reads the cache
	m2 := &Market{Dir: m.Dir}
	m2.loadCache()
	if len(m2.Index().Skills) != 2 || m2.meta.ETag != `"`+ref1+`"` {
		t.Fatal("cache not loaded")
	}

	// offline keeps the cached index
	f.srv.Close()
	if err := m.Refresh(); err == nil || m.Status().State != "offline" || len(m.Index().Skills) != 2 {
		t.Fatalf("offline: %v %+v", err, m.Status())
	}

	// not published yet
	nf := httptest.NewServer(http.NotFoundHandler())
	defer nf.Close()
	m3 := &Market{Dir: t.TempDir(), IndexURL: nf.URL, HTTP: nf.Client()}
	if err := m3.Refresh(); err != nil || m3.Status().State != "missing" {
		t.Fatalf("404: %v %+v", err, m3.Status())
	}

	// wrong version
	v2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"version":2}`)) }))
	defer v2.Close()
	m4 := &Market{Dir: t.TempDir(), IndexURL: v2.URL, HTTP: v2.Client()}
	if err := m4.Refresh(); err == nil || m4.Index().Version != 0 {
		t.Fatal("version 2 accepted")
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

func TestScore(t *testing.T) {
	es := []catalog.Entry{
		{ID: "a", Name: "react-patterns", Description: "React hooks", Category: "coding/frontend", Stars: 10},
		{ID: "b", Name: "vue-guide", Category: "coding/frontend", Stars: 1000},
		{ID: "c", Name: "pdf", Category: "documents/pdf", Stars: 5000},
		{ID: "d", Name: "react-installed", Description: "react", Category: "coding/frontend"},
	}
	sig := []Signal{{Reason: "your project uses React", Cats: []string{"coding/frontend"}, Words: []string{"react"}}}
	got := Score(es, sig, func(e catalog.Entry) bool { return e.ID == "d" }, 10)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("%+v", got)
	}
	if got[0].Reasons[0] != "because your project uses React" {
		t.Fatal(got[0].Reasons)
	}
	p := Pairs([]catalog.Entry{{ID: "x", Repo: "o/r"}, {ID: "y", Repo: "o/z"}}, map[string]string{"o/r": "foo"}, func(catalog.Entry) bool { return false }, 5)
	if len(p) != 1 || p[0].ID != "x" {
		t.Fatalf("%+v", p)
	}
}

func TestScan(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "package.json"), []byte(`{"dependencies":{"react":"19"},"devDependencies":{"@sveltejs/kit":"2"}}`), 0o644)
	os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x"), 0o644)
	os.MkdirAll(filepath.Join(d, ".github", "workflows"), 0o755)
	os.MkdirAll(filepath.Join(d, "node_modules", "x"), 0o755)
	os.WriteFile(filepath.Join(d, "node_modules", "x", "Cargo.toml"), nil, 0o644)
	var got []string
	for _, s := range Scan(d) {
		got = append(got, s.Reason)
	}
	want := "your project uses GitHub Actions,your project uses Go,your project uses JavaScript,your project uses React,your project uses Svelte"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v", got)
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

func TestPopularity(t *testing.T) {
	m := &Market{}
	m.setIndex(catalog.Index{Version: 1, Skills: []catalog.Entry{
		{ID: "stars", Name: "a", Stars: 9000},
		{ID: "tie", Name: "b", Installs: 50, Stars: 1},
		{ID: "installs", Name: "c", Installs: 50, Stars: 2},
	}})
	_, got := m.Query("", "", "", 0, 10)
	if got[0].ID != "installs" || got[1].ID != "tie" || got[2].ID != "stars" {
		t.Fatalf("default sort: %v", got)
	}
	if popularity(catalog.Entry{Installs: 10, Stars: 1e6}) >= popularity(catalog.Entry{Stars: 1e6}) {
		t.Fatal("installs must win over the stars fallback when present")
	}
}
