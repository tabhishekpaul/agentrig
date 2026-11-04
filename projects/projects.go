// Package projects pins skills to a project: a committed .arig.json lists the
// skills, and Apply links each one from the global skill folder into every
// tool's project-level skills folder. Only links arig made are ever removed.
package projects

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
)

const (
	ManifestFile = ".arig.json"
	lockFile     = ".arig.lock" // machine-local record of what arig created
	copyMarker   = ".arig-copy" // dropped inside copies (no symlinks), so copies are recognisable too
)

// Dirs is each tool's project-level skills folder (<dir>/<skill>/SKILL.md).
var Dirs = []struct{ Tool, Dir string }{
	{"claude", ".claude/skills"},
	{"codex", ".agents/skills"},
	{"cursor", ".cursor/skills"},
	{"gemini", ".gemini/skills"},
	{"copilot-cli", ".github/skills"},
	{"opencode", ".opencode/skills"},
}

func toolDir(tool string) string {
	for _, d := range Dirs {
		if d.Tool == tool {
			return d.Dir
		}
	}
	return ""
}

// Manifest is .arig.json. Tools empty = every installed tool in Dirs.
type Manifest struct {
	Skills []string `json:"skills"`
	Tools  []string `json:"tools,omitempty"`
}

// Overridable so tests never read the real tools or config dir.
var (
	detect    = adapters.Detect
	configDir = func() string { d, _ := os.UserConfigDir(); return filepath.Join(d, "agentrig") }
	mu        sync.Mutex
)

// ValidSkill: skill ids are plain folder names.
func ValidSkill(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.HasPrefix(id, ".") &&
		!strings.ContainsAny(id, `/\:`) && filepath.Base(id) == id
}

func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{Skills: []string{}}, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err != nil {
		return m, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	if m.Skills == nil {
		m.Skills = []string{}
	}
	return m, m.validate()
}

func (m Manifest) validate() error {
	for _, s := range m.Skills {
		if !ValidSkill(s) {
			return fmt.Errorf("bad skill id %q", s)
		}
	}
	for _, t := range m.Tools {
		if toolDir(t) == "" {
			return fmt.Errorf("tool %q has no project skills folder", t)
		}
	}
	return nil
}

func WriteManifest(dir string, m Manifest) error {
	if err := m.validate(); err != nil {
		return err
	}
	if m.Skills == nil {
		m.Skills = []string{}
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, ManifestFile), append(b, '\n'), 0o644)
}

// Link is one skill in one tool's folder.
type Link struct {
	Skill string `json:"skill"`
	Tool  string `json:"tool"`
	Path  string `json:"path"`  // relative to the project
	State string `json:"state"` // linked | missing | not-installed | conflict (a file arig didn't make)
}

type lockEntry struct {
	Path string `json:"path"`
	Copy bool   `json:"copy,omitempty"`
}

func readLock(dir string) []lockEntry {
	var l []lockEntry
	b, _ := os.ReadFile(filepath.Join(dir, lockFile))
	_ = json.Unmarshal(b, &l)
	return l
}

// global maps skill id -> global folder.
func global() map[string]string {
	out := map[string]string{}
	for _, e := range catalog.Local(detect()) {
		out[e.ID] = e.Path
	}
	return out
}

// tools resolves the manifest's tools: listed, or every installed one with a project folder.
func tools(m Manifest) []string {
	if len(m.Tools) > 0 {
		return m.Tools
	}
	var out []string
	for _, t := range detect() {
		if t.Installed && toolDir(t.ID) != "" {
			out = append(out, t.ID)
		}
	}
	return out
}

// Status reports every skill × tool in the manifest without changing anything.
func Status(dir string) ([]Link, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	return status(dir, m, global(), readLock(dir)), nil
}

func status(dir string, m Manifest, glob map[string]string, lock []lockEntry) []Link {
	ours := map[string]bool{}
	for _, e := range lock {
		ours[e.Path] = owned(dir, e)
	}
	links := []Link{}
	for _, t := range tools(m) {
		for _, s := range m.Skills {
			rel := filepath.ToSlash(filepath.Join(toolDir(t), s))
			l := Link{Skill: s, Tool: t, Path: rel, State: "missing"}
			switch {
			case ours[rel]:
				l.State = "linked"
			case exists(filepath.Join(dir, rel)):
				l.State = "conflict"
			case glob[s] == "":
				l.State = "not-installed"
			}
			links = append(links, l)
		}
	}
	return links
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// owned: the lock says arig made it and it still looks like what arig makes.
// A lock path that isn't <tool dir>/<skill> is ignored, so a tampered lock
// can't point deletes anywhere else.
func owned(dir string, e lockEntry) bool {
	d, s := filepath.Split(filepath.FromSlash(e.Path))
	if !ValidSkill(s) || !slices.ContainsFunc(Dirs, func(x struct{ Tool, Dir string }) bool {
		return filepath.FromSlash(x.Dir) == filepath.Clean(d)
	}) {
		return false
	}
	p := filepath.Join(dir, e.Path)
	if e.Copy {
		return exists(filepath.Join(p, copyMarker))
	}
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// Apply makes the project match its manifest: links what's listed, removes
// links arig made earlier that are no longer listed, never touches anything
// else. Returns the resulting status.
func Apply(dir string) ([]Link, error) {
	mu.Lock()
	defer mu.Unlock()
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	glob := global()
	want := map[string]bool{}
	for _, l := range status(dir, m, glob, nil) {
		want[l.Path] = true
	}

	var lock []lockEntry
	for _, e := range readLock(dir) {
		switch {
		case !owned(dir, e):
			// gone, or replaced by the user: forget it, leave the file alone
		case want[e.Path] && (e.Copy || target(e.Path, root, glob) == readlink(filepath.Join(dir, e.Path))):
			lock = append(lock, e)
		default: // no longer listed, or the global skill moved
			if err := os.RemoveAll(filepath.Join(dir, e.Path)); err != nil {
				return nil, err
			}
		}
	}

	var errs []error
	for _, l := range status(dir, m, glob, lock) {
		if l.State != "missing" {
			continue
		}
		e, err := link(dir, root, l.Path, glob[l.Skill])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		lock = append(lock, e)
	}
	if err := writeLock(dir, lock); err != nil {
		return nil, err
	}
	if err := ignore(dir, m); err != nil {
		errs = append(errs, err)
	}
	return status(dir, m, glob, lock), errors.Join(errs...)
}

func readlink(p string) string { s, _ := os.Readlink(p); return s }

// target is the relative symlink from the project path to the global skill.
func target(rel, root string, glob map[string]string) string {
	src, err := filepath.EvalSymlinks(glob[filepath.Base(rel)])
	if err != nil {
		return ""
	}
	t, _ := filepath.Rel(filepath.Join(root, filepath.Dir(filepath.FromSlash(rel))), src)
	return t
}

func link(dir, root, rel, src string) (lockEntry, error) {
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return lockEntry{}, err
	}
	// A tool folder that is itself a symlink (e.g. .claude -> ~/.claude) would
	// put the link outside the project: refuse.
	parent, err := filepath.EvalSymlinks(filepath.Dir(p))
	if err != nil {
		return lockEntry{}, err
	}
	if r, err := filepath.Rel(root, parent); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return lockEntry{}, fmt.Errorf("%s leaves the project; not linking", filepath.Dir(rel))
	}
	if t := target(rel, root, map[string]string{filepath.Base(rel): src}); t != "" && runtime.GOOS != "windows" {
		if os.Symlink(t, p) == nil {
			return lockEntry{Path: rel}, nil
		}
	}
	// Windows, or symlinks not allowed: copy.
	srcFS, err := filepath.EvalSymlinks(src)
	if err != nil {
		return lockEntry{}, err
	}
	if err := os.CopyFS(p, os.DirFS(srcFS)); err != nil {
		os.RemoveAll(p)
		return lockEntry{}, err
	}
	if err := os.WriteFile(filepath.Join(p, copyMarker), []byte("copied by arig; `arig apply` replaces or removes this folder\n"), 0o644); err != nil {
		return lockEntry{}, err
	}
	return lockEntry{Path: rel, Copy: true}, nil
}

func writeLock(dir string, l []lockEntry) error {
	p := filepath.Join(dir, lockFile)
	if len(l) == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

const ignoreStart, ignoreEnd = "# >>> arig: linked project skills (recreate with `arig apply`)", "# <<< arig"

// ignore keeps the links out of git: they point into this machine's home.
// The block is derived from the manifest only (every tool folder, not just the
// ones installed here), so it's identical on every machine. Only touched in a
// git repo, and only between arig's markers.
func ignore(dir string, m Manifest) error {
	p := filepath.Join(dir, ".gitignore")
	old, err := os.ReadFile(p)
	if err != nil && !exists(filepath.Join(dir, ".git")) {
		return nil
	}
	ts := m.Tools
	if len(ts) == 0 {
		for _, d := range Dirs {
			ts = append(ts, d.Tool)
		}
	}
	block := []string{ignoreStart, "/" + lockFile}
	for _, t := range ts {
		for _, s := range m.Skills {
			block = append(block, "/"+toolDir(t)+"/"+s)
		}
	}
	block = append(block, ignoreEnd)

	text := strings.ReplaceAll(string(old), "\r\n", "\n")
	if i := strings.Index(text, ignoreStart+"\n"); i >= 0 {
		if j := strings.Index(text[i:], ignoreEnd+"\n"); j >= 0 {
			text = text[:i] + text[i+j+len(ignoreEnd)+1:]
		}
	}
	if len(m.Skills) > 0 {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += strings.Join(block, "\n") + "\n"
	}
	if text == string(old) {
		return nil
	}
	return os.WriteFile(p, []byte(text), 0o644)
}

// Skills returns the catalog entries for the skills a project lists that are
// installed globally on this machine (missing ones are left out).
func Skills(dir string) ([]catalog.Entry, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	var out []catalog.Entry
	for _, e := range catalog.Local(detect()) {
		if slices.Contains(m.Skills, e.ID) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Edit adds (or removes) skills in dir's manifest, creating it if needed.
func Edit(dir string, add bool, ids ...string) (Manifest, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return m, err
	}
	for _, id := range ids {
		if !ValidSkill(id) {
			return m, fmt.Errorf("bad skill id %q", id)
		}
		i := slices.Index(m.Skills, id)
		switch {
		case add && i < 0:
			m.Skills = append(m.Skills, id)
		case !add && i >= 0:
			m.Skills = slices.Delete(m.Skills, i, i+1)
		}
	}
	return m, WriteManifest(dir, m)
}

// Known projects, stored in UserConfigDir/agentrig/projects.json.

func listFile() string { return filepath.Join(configDir(), "projects.json") }

func List() []string {
	var ps []string
	b, _ := os.ReadFile(listFile())
	_ = json.Unmarshal(b, &ps)
	if ps == nil {
		ps = []string{}
	}
	return ps
}

// Known reports whether dir is a registered project.
func Known(dir string) bool { return slices.Contains(List(), filepath.Clean(dir)) }

// Add registers dir (an existing absolute directory), symlinks resolved.
func Add(dir string) error {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%q is not an absolute path", dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%q is not a folder", dir)
	}
	dir, _ = filepath.EvalSymlinks(dir) // one entry per folder, however it was reached
	mu.Lock()
	defer mu.Unlock()
	ps := List()
	if slices.Contains(ps, dir) {
		return nil
	}
	return saveList(append(ps, dir))
}

// Remove forgets dir; its files are left as they are.
func Remove(dir string) error {
	mu.Lock()
	defer mu.Unlock()
	return saveList(slices.DeleteFunc(List(), func(p string) bool { return p == filepath.Clean(dir) }))
}

func saveList(ps []string) error {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(ps, "", "  ")
	return os.WriteFile(listFile(), append(b, '\n'), 0o600)
}
