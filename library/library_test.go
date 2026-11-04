package library

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/gitsync"
	"github.com/tabhishekpaul/agentrig/insights"
)

// TestMain points HOME at a throwaway folder so nothing here can ever touch
// the real library or skill folders, even a test that forgets to isolate.
func TestMain(m *testing.M) {
	home, _ := os.MkdirTemp("", "arig-library-test-")
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", "")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func isolate(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func write(t *testing.T, p, content string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// snapshot is a folder's content: rel path -> mode + bytes or link target.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		info, _ := d.Info()
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			out[rel] = "link " + l
		case d.IsDir():
			out[rel] = "dir"
		default:
			b, _ := os.ReadFile(p)
			out[rel] = fmt.Sprintf("%v %s", info.Mode().Perm(), b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func same(a, b map[string]string) bool { return fmt.Sprint(a) == fmt.Sprint(b) }

func skill(t *testing.T, dir, body string) {
	write(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+filepath.Base(dir)+"\n---\n"+body, 0o644)
	write(t, filepath.Join(dir, "scripts", "run.sh"), "echo "+body, 0o755)
	os.Symlink("scripts/run.sh", filepath.Join(dir, "run"))
}

func TestHash(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	skill(t, a, "x")
	// b: same files written in another order, plus everything sync ignores
	write(t, filepath.Join(b, "scripts", "run.sh"), "echo x", 0o644)
	os.Symlink("scripts/run.sh", filepath.Join(b, "run"))
	write(t, filepath.Join(b, "SKILL.md"), "---\nname: a\n---\nx", 0o644)
	write(t, filepath.Join(b, ".env"), "SECRET=1", 0o644)
	write(t, filepath.Join(b, ".DS_Store"), "junk", 0o644)
	write(t, filepath.Join(b, "node_modules", "x", "i.js"), "x", 0o644)
	os.MkdirAll(filepath.Join(b, "empty"), 0o755)
	ha, err := Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	if hb, _ := Hash(b); ha != hb || len(ha) != 12 || !ValidHash(ha) {
		t.Fatalf("same content, different hashes: %s %s", ha, hb)
	}
	if again, _ := Hash(a); again != ha {
		t.Fatal("not deterministic")
	}
	// via a link to the folder: same
	os.Symlink(a, filepath.Join(root, "via"))
	if hv, _ := Hash(filepath.Join(root, "via")); hv != ha {
		t.Fatal("hash through a link differs")
	}
	for name, change := range map[string]func(string){
		"content":     func(d string) { write(t, filepath.Join(d, "SKILL.md"), "changed", 0o644) },
		"link target": func(d string) { os.Remove(filepath.Join(d, "run")); os.Symlink("SKILL.md", filepath.Join(d, "run")) },
		"rename":      func(d string) { os.Rename(filepath.Join(d, "scripts"), filepath.Join(d, "bin")) },
		"new file":    func(d string) { write(t, filepath.Join(d, "x.txt"), "", 0o644) },
	} {
		d := filepath.Join(t.TempDir(), "s")
		skill(t, d, "x")
		write(t, filepath.Join(d, "SKILL.md"), "---\nname: a\n---\nx", 0o644)
		if h, _ := Hash(d); h != ha {
			t.Fatalf("%s: fixture differs", name)
		}
		change(d)
		if h, _ := Hash(d); h == ha {
			t.Errorf("%s didn't change the hash", name)
		}
	}
}

func TestPutAdoptDedupe(t *testing.T) {
	isolate(t)
	src := filepath.Join(t.TempDir(), "pdf")
	skill(t, src, "v1")
	write(t, filepath.Join(src, ".env"), "SECRET", 0o644)
	p, h, err := Put(src, "pdf")
	if err != nil || p != Path("pdf", h) {
		t.Fatal(p, h, err)
	}
	if _, err := os.Stat(filepath.Join(p, ".env")); err == nil {
		t.Error("Put copied a secret")
	}
	if fi, _ := os.Stat(filepath.Join(p, "scripts/run.sh")); fi.Mode().Perm() != 0o755 {
		t.Error("mode not kept")
	}
	if l, _ := os.Readlink(filepath.Join(p, "run")); l != "scripts/run.sh" {
		t.Error("inner link not kept as a link")
	}
	if p2, h2, _ := Put(src, "pdf"); p2 != p || h2 != h || len(Versions("pdf")) != 1 {
		t.Fatal("Put didn't dedupe")
	}

	// Adopt of identical content: same entry, source left alone
	twin := filepath.Join(t.TempDir(), "pdf")
	skill(t, twin, "v1")
	if p3, err := Adopt(twin, "pdf"); err != nil || p3 != p {
		t.Fatal(p3, err)
	}
	if _, err := os.Stat(twin); err != nil {
		t.Fatal("dedupe removed the source")
	}
	// new content: moved in whole
	v2 := filepath.Join(t.TempDir(), "pdf")
	skill(t, v2, "v2")
	before := snapshot(t, v2)
	p4, err := Adopt(v2, "pdf")
	if err != nil || p4 == p {
		t.Fatal(p4, err)
	}
	if _, err := os.Stat(v2); err == nil {
		t.Error("Adopt on one volume should move")
	}
	if !same(before, snapshot(t, p4)) {
		t.Error("moved entry differs")
	}
	if vs := Versions("pdf"); len(vs) != 2 {
		t.Fatalf("versions %+v", vs)
	}
	// owner/repo/skill names nest; bad names are refused
	if p5, _, err := Put(src, "anthropics/skills/pdf"); err != nil || !strings.Contains(p5, filepath.FromSlash(".arig/library/anthropics/skills/pdf/")) {
		t.Fatal(p5, err)
	}
	for _, bad := range []string{"", "..", "a/b", "a/../b/c", "../x", ".hidden", "a/b/c/d"} {
		if _, _, err := Put(src, bad); err == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
}

func TestLinkAndCopyFallback(t *testing.T) {
	home := isolate(t)
	src := filepath.Join(t.TempDir(), "pdf")
	skill(t, src, "v1")
	entry, _, _ := Put(src, "pdf")
	at := filepath.Join(home, ".claude", "skills", "pdf")
	if err := Link(at, entry); err != nil {
		t.Fatal(err)
	}
	if l, _ := os.Readlink(at); l != filepath.FromSlash("../../.arig/library/pdf/"+filepath.Base(entry)) {
		t.Fatalf("want a relative link, got %q", l)
	}
	real, _ := filepath.EvalSymlinks(entry)
	if !IsLibraryLink(at) || Target(at) != real {
		t.Fatal("not recognised as a library link")
	}
	if err := Link(at, entry); err == nil {
		t.Fatal("overwrote an existing path")
	}
	os.Symlink(src, filepath.Join(home, "other"))
	if IsLibraryLink(filepath.Join(home, "other")) || IsLibraryLink(src) {
		t.Fatal("non-library path recognised")
	}

	// Windows: a marked copy
	copyLinks = true
	defer func() { copyLinks = runtime.GOOS == "windows" }()
	cp := filepath.Join(home, ".cursor", "skills", "pdf")
	if err := Link(cp, entry); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(cp); fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("made a link with copyLinks")
	}
	if b, _ := os.ReadFile(filepath.Join(cp, "SKILL.md")); !strings.Contains(string(b), "v1") || Target(cp) != real {
		t.Fatal("copy not recognised")
	}
	s := Summarize(home, []string{filepath.Join(home, ".claude/skills"), filepath.Join(home, ".cursor/skills")})
	if s.Entries != 1 || s.Versions != 1 || s.Links != 2 || s.Bytes == 0 {
		t.Fatalf("%+v", s)
	}
}

func TestMigrateAndUndo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("migration doesn't run on Windows")
	}
	home := isolate(t)
	backup := filepath.Join(home, "cfg", "agentrig", "backup")
	sk := func(rel string) string { return filepath.Join(home, filepath.FromSlash(rel)) }

	skill(t, sk(".claude/skills/a"), "a")
	skill(t, sk(".cursor/skills/a"), "a") // identical: one entry
	skill(t, sk(".agents/skills/shared"), "shared")
	os.Symlink("../../.agents/skills/shared", sk(".claude/skills/shared")) // existing chain
	skill(t, sk(".claude/skills-disabled/off"), "off")
	skill(t, sk(".codex/skills/gitty"), "g")
	write(t, sk(".codex/skills/gitty/.git/HEAD"), "ref", 0o644)
	skill(t, sk(".claude/skills/escapes"), "e")
	os.Symlink("/etc/hosts", sk(".claude/skills/escapes/hosts"))
	write(t, sk(".claude/skills/notes/README.md"), "not a skill", 0o644)
	write(t, sk(".claude/skills/stray.md"), "file", 0o644)
	skill(t, sk(".claude/skills/broken"), "b")
	write(t, sk(".claude/skills/broken/secret.txt"), "unreadable", 0o000)
	t.Cleanup(func() { os.Chmod(sk(".claude/skills/broken/secret.txt"), 0o644) })
	os.MkdirAll(sk(".gemini"), 0o755)
	os.Symlink("../.claude/skills", sk(".gemini/skills")) // a whole dir linked to another

	before := map[string]map[string]string{}
	for _, rel := range []string{".claude/skills/a", ".cursor/skills/a", ".agents/skills/shared", ".claude/skills-disabled/off"} {
		before[rel] = snapshot(t, sk(rel))
	}
	untouched := map[string]map[string]string{}
	for _, rel := range []string{".codex/skills/gitty", ".claude/skills/escapes", ".claude/skills/notes"} {
		untouched[rel] = snapshot(t, sk(rel))
	}

	rep, err := Migrate(home, backup)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Migrated != 4 || rep.Created != 3 || len(rep.Skipped) != 2 {
		t.Fatalf("%+v", rep)
	}
	if os.Geteuid() != 0 && (len(rep.Failed) != 1 || !strings.Contains(rep.Failed[0], "broken")) {
		t.Fatalf("broken skill: %+v", rep.Failed)
	}
	for rel := range before {
		p := sk(rel)
		l, err := os.Readlink(p)
		if err != nil || filepath.IsAbs(l) || !strings.Contains(l, ".arig/library/") {
			t.Fatalf("%s: want a relative library link, got %q %v", rel, l, err)
		}
		if !same(snapshot(t, Target(p)), before[rel]) {
			t.Errorf("%s: library entry differs from the original", rel)
		}
	}
	if Target(sk(".claude/skills/a")) != Target(sk(".cursor/skills/a")) {
		t.Error("identical skills got two entries")
	}
	if l, _ := os.Readlink(sk(".claude/skills/shared")); l != "../../.agents/skills/shared" {
		t.Errorf("existing link changed to %q", l)
	}
	if b, _ := os.ReadFile(sk(".claude/skills/shared/SKILL.md")); !strings.Contains(string(b), "shared") {
		t.Error("link chain broken")
	}
	for rel, snap := range untouched {
		if !same(snapshot(t, sk(rel)), snap) {
			t.Errorf("%s was changed", rel)
		}
	}
	if fi, err := os.Lstat(sk(".claude/skills/broken")); err != nil || !fi.IsDir() {
		t.Fatal("broken skill not left in place")
	}
	if b, _ := os.ReadFile(sk(".claude/skills/stray.md")); string(b) != "file" {
		t.Error("stray file touched")
	}

	// backups: listed and restorable by Insights
	bs := insights.Backups(backup)
	if len(bs) != 1 || !strings.HasSuffix(bs[0].ID, "-library") || len(bs[0].Items) != 4 {
		t.Fatalf("backups %+v", bs)
	}
	if b, _ := os.ReadFile(filepath.Join(backup, bs[0].ID, ".claude/skills/a/SKILL.md")); !strings.Contains(string(b), "a") {
		t.Fatal("no backup copy")
	}

	// idempotent: the marker stops a rerun; without it, nothing is left to do
	if again, _ := Migrate(home, backup); again.Migrated != 0 || again.Log != "" {
		t.Fatalf("second run: %+v", again)
	}
	os.Remove(filepath.Join(RootAt(home), MarkerV1))
	if again, _ := Migrate(home, backup); again.Migrated != 0 {
		t.Fatalf("rerun without marker: %+v", again)
	}

	// a tool switches one skill off after the migration: Undo finds it
	os.MkdirAll(sk(".cursor/skills-disabled"), 0o755)
	os.Rename(sk(".cursor/skills/a"), sk(".cursor/skills-disabled/a"))
	last := LastLog(home)
	if last == nil || last.Links != 4 || last.Undone != nil {
		t.Fatalf("last log %+v", last)
	}
	n, err := Undo(last.Path)
	if err != nil || n != 4 {
		t.Fatal(n, err)
	}
	moved := map[string]string{".cursor/skills/a": ".cursor/skills-disabled/a"}
	for rel, snap := range before {
		if m, ok := moved[rel]; ok {
			rel = m
		}
		fi, err := os.Lstat(sk(rel))
		if err != nil || !fi.IsDir() {
			t.Fatalf("%s not a real folder again", rel)
		}
		if !same(snapshot(t, sk(rel)), snap) {
			t.Errorf("%s not byte-identical after undo:\n%v\n%v", rel, snapshot(t, sk(rel)), snap)
		}
	}
	if _, err := Undo(last.Path); err == nil {
		t.Error("undid twice")
	}
	if LastLog(home).Undone == nil {
		t.Error("log not marked undone")
	}
}

// A skill linked into the library on machine A arrives working on machine B,
// whose home path (and depth) differs.
func TestSyncTwoMachines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	a := filepath.Join(t.TempDir(), "a")
	t.Setenv("HOME", a)
	skill(t, filepath.Join(a, ".claude/skills/pdf"), "pdf")
	if rep, err := Migrate(a, filepath.Join(a, "backup")); err != nil || rep.Migrated != 1 {
		t.Fatal(rep, err)
	}
	sa := &gitsync.Syncer{Home: a, Dir: filepath.Join(a, "agentrig")}
	if _, err := sa.Connect(remote); err != nil {
		t.Fatal(err)
	}

	b := filepath.Join(t.TempDir(), "deeper", "home", "b")
	t.Setenv("HOME", b)
	os.MkdirAll(filepath.Join(b, ".claude"), 0o755) // B has Claude Code, no ~/.arig yet
	sb := &gitsync.Syncer{Home: b, Dir: filepath.Join(b, "agentrig")}
	if _, err := sb.Connect(remote); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(b, ".claude/skills/pdf")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("B didn't get the link")
	}
	if got, _ := os.ReadFile(filepath.Join(link, "SKILL.md")); !strings.Contains(string(got), "pdf") {
		l, _ := os.Readlink(link)
		out, _ := exec.Command("find", b, "-path", "*/.git", "-prune", "-o", "-print").CombinedOutput()
		t.Fatalf("link %q doesn't resolve on B:\n%s", l, out)
	}
	if fi, err := os.Stat(filepath.Join(link, "scripts/run.sh")); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Error("script lost its exec bit")
	}
	if !IsLibraryLink(link) {
		t.Error("not a library link on B")
	}
	for _, local := range []string{MarkerV1, filepath.Base(LastLog(a).Path)} {
		if _, err := os.Lstat(filepath.Join(RootAt(b), local)); err == nil {
			t.Errorf("%s synced to B", local)
		}
	}
	// B's own migration has nothing to move: the link stays a link
	if rep, _ := Migrate(b, filepath.Join(b, "backup")); rep.Migrated != 0 {
		t.Fatalf("%+v", rep)
	}
	// and a sync back changes nothing on A
	if _, err := sb.Run(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", a)
	if _, err := sa.Run(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(a, ".claude/skills/pdf/SKILL.md")); !strings.Contains(string(b), "pdf") {
		t.Fatal("A broke after syncing back")
	}
}
