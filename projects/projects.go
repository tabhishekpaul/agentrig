// Package projects pins skills to a project. A committed .arig/ folder (like
// .claude/, but for every tool) holds arig.json, which lists skills to pin (a
// library name like "pdf", or a skills.sh id like "owner/repo/skill"),
// arig.lock, the exact version (library hash) each one resolved to, and
// skills/, the project's own skills. Apply resolves every pin from the
// central library (bun-style: locked hash, else newest version, else
// download) and links it into every tool's project-level skills folder.
// Only links arig made are ever removed.
package projects

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/library"
	"github.com/tabhishekpaul/agentrig/market"
)

const (
	Folder       = ".arig"
	ManifestFile = ".arig/arig.json"
	LocalSkills  = ".arig/skills"    // the project's own skills, committed with it
	LockFile     = ".arig/arig.lock" // committed: pin -> library hash
	lockFile     = ".arig/lock.json" // machine-local record of what arig created (git-ignored)
	copyMarker   = ".arig-copy"      // dropped inside copies (no symlinks), so copies are recognisable too
	oldManifest  = ".arig.json"      // before the .arig/ folder; migrated on write
	oldLock      = ".arig.lock"
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
	newMarket = func() *market.Market { return market.New("") }
	mu        sync.Mutex
	copyLinks = runtime.GOOS == "windows" // tests flip it
)

// ValidSkill: skill ids are plain folder names.
func ValidSkill(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.HasPrefix(id, ".") &&
		!strings.ContainsAny(id, `/\:`) && filepath.Base(id) == id
}

// Remote reports whether a pin is a skills.sh id (owner/repo/skill).
func Remote(pin string) bool { _, _, _, err := market.SplitID(pin); return err == nil }

// ValidPin: a plain skill name or a skills.sh id.
func ValidPin(pin string) bool { return ValidSkill(pin) || Remote(pin) }

// linkName is the folder a pin gets in each tool: its last segment.
func linkName(pin string) string { return path.Base(pin) }

func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if errors.Is(err, os.ErrNotExist) {
		b, err = os.ReadFile(filepath.Join(dir, oldManifest))
	}
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
	seen := map[string]string{}
	for _, s := range m.Skills {
		if !ValidPin(s) {
			return fmt.Errorf("bad skill id %q", s)
		}
		if o, ok := seen[linkName(s)]; ok {
			return fmt.Errorf("%s and %s would share the folder %q", o, s, linkName(s))
		}
		seen[linkName(s)] = s
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
	if err := os.MkdirAll(filepath.Join(dir, Folder), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), append(b, '\n'), 0o644); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, oldManifest)) // moved into .arig/
	return nil
}

// localSkills lists the project's own skills: .arig/skills/<id>/SKILL.md.
func localSkills(dir string) map[string]string {
	out := map[string]string{}
	entries, _ := os.ReadDir(filepath.Join(dir, LocalSkills))
	for _, e := range entries {
		p := filepath.Join(dir, LocalSkills, e.Name())
		if e.IsDir() && ValidSkill(e.Name()) && exists(filepath.Join(p, "SKILL.md")) {
			out[e.Name()] = p
		}
	}
	return out
}

// effective is what a project uses: the pins plus every skill in
// .arig/skills. A project skill wins over a pin that would take its folder.
func effective(dir string, m Manifest) Manifest {
	local := localSkills(dir)
	m.Skills = slices.DeleteFunc(slices.Clone(m.Skills), func(p string) bool {
		_, own := local[linkName(p)]
		return own && p != linkName(p)
	})
	for id := range local {
		if !slices.Contains(m.Skills, id) {
			m.Skills = append(m.Skills, id)
		}
	}
	slices.Sort(m.Skills)
	return m
}

// Link is one skill in one tool's folder.
type Link struct {
	Skill   string   `json:"skill"` // the pin: a name or owner/repo/skill
	Tool    string   `json:"tool"`
	Path    string   `json:"path"`              // relative to the project
	State   string   `json:"state"`             // linked | missing | not-installed | needs-confirm | version-changed | conflict (a file arig didn't make)
	Local   bool     `json:"local,omitempty"`   // from the project's own .arig/skills
	Version string   `json:"version,omitempty"` // short library hash
	Source  string   `json:"source,omitempty"`  // project | library | skills.sh
	Flags   []string `json:"flags,omitempty"`   // safety flags of a held-back download
	Locked  string   `json:"locked,omitempty"`  // held back: the hash arig.lock pins
	Offered string   `json:"offered,omitempty"` // held back: the hash of what was downloaded
}

type lockEntry struct {
	Path string `json:"path"`
	Src  string `json:"src,omitempty"` // the folder it links to
	Copy bool   `json:"copy,omitempty"`
}

func readLock(dir string) []lockEntry {
	var l []lockEntry
	b, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		b, _ = os.ReadFile(filepath.Join(dir, oldLock))
	}
	_ = json.Unmarshal(b, &l)
	return l
}

// readPins reads arig.lock: pin -> library hash. Hashes that aren't one are
// dropped (the file is committed, so it's input like any other).
func readPins(dir string) map[string]string {
	l := map[string]string{}
	b, _ := os.ReadFile(filepath.Join(dir, LockFile))
	_ = json.Unmarshal(b, &l)
	for k, v := range l {
		if !ValidPin(k) || !library.ValidHash(v) {
			delete(l, k)
		}
	}
	return l
}

func writePins(dir string, l map[string]string) error {
	p := filepath.Join(dir, LockFile)
	if len(l) == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, _ := json.MarshalIndent(l, "", "  ") // sorted keys: clean diffs
	b = append(b, '\n')
	if old, err := os.ReadFile(p); err == nil && string(old) == string(b) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
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

// resolved is where one pin comes from.
type resolved struct {
	Path    string // what the links point to; "" = not available here
	Hash    string // library hash ("" for project skills and not-yet-stored globals)
	Source  string // project | library | skills.sh
	Flags   []string
	State   string // needs-confirm | version-changed: downloaded but held back
	Locked  string // hash in arig.lock, when held back
	Offered string // hash of the download held back
	global  bool   // Path is an installed global skill not in the library yet
}

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }

// resolve finds every pin without writing or downloading anything: the
// project's own skill; else the locked library version; else the newest
// library version; else (plain names) the installed global skill.
func resolve(dir string, m Manifest, pins map[string]string) map[string]resolved {
	local := localSkills(dir)
	var glob map[string]string
	out := map[string]resolved{}
	for _, pin := range m.Skills {
		src := "library"
		if Remote(pin) {
			src = "skills.sh"
		}
		if p, ok := local[pin]; ok {
			out[pin] = resolved{Path: p, Source: "project"}
			continue
		}
		if h := pins[pin]; h != "" && isDir(library.Path(pin, h)) {
			out[pin] = resolved{Path: library.Path(pin, h), Hash: h, Source: src}
			continue
		}
		if vs := library.Versions(pin); len(vs) > 0 {
			out[pin] = resolved{Path: vs[0].Path, Hash: vs[0].Hash, Source: src}
			continue
		}
		if !Remote(pin) {
			if glob == nil {
				glob = global()
			}
			if p := glob[pin]; p != "" {
				out[pin] = resolved{Path: p, Source: src, global: true}
				continue
			}
		}
		out[pin] = resolved{Source: src}
	}
	return out
}

// fetch fills in what resolve couldn't: a global skill is put into the
// library; a skills.sh id is downloaded and staged, then checked before it
// goes in. arig.lock is committed, so it may come from someone else and is
// never taken as this user's consent:
//   - a pin whose download hashes differently from arig.lock is not
//     installed (version-changed) unless accepted in this call;
//   - a download with safety flags installs only if this exact (id, hash)
//     was accepted on this machine (accepted.json), or is accepted now.
//
// accept holds ids ("owner/repo/skill", whatever is downloaded now) or
// "owner/repo/skill@hash" (only that version); what it accepts is recorded
// in accepted.json.
func fetch(r map[string]resolved, pins map[string]string, accept []string) error {
	var mk *market.Market
	var errs []error
	for pin, x := range r {
		switch {
		case x.global:
			p, h, err := library.Put(x.Path, pin)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", pin, err))
				continue
			}
			r[pin] = resolved{Path: p, Hash: h, Source: x.Source}
		case x.Path == "" && Remote(pin):
			if mk == nil {
				mk = newMarket()
			}
			dl, err := mk.Get(pin)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			staged, h, err := mk.Stage(dl)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", pin, err))
				continue
			}
			now := slices.Contains(accept, pin) || slices.Contains(accept, pin+"@"+h)
			flags := market.Flags(dl.Files)
			x.Flags, x.Locked, x.Offered = flags, pins[pin], h
			switch {
			case pins[pin] != "" && pins[pin] != h && !now:
				x.State = "version-changed"
			case len(flags) > 0 && !now && !accepted(pin, h):
				x.State = "needs-confirm"
			}
			if x.State != "" {
				os.RemoveAll(staged)
				r[pin] = x
				continue
			}
			if now {
				if err := recordAccepted(pin, h); err != nil {
					errs = append(errs, err)
				}
			}
			p, err := library.Adopt(staged, pin)
			os.RemoveAll(staged) // gone already if it was moved in
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", pin, err))
				continue
			}
			r[pin] = resolved{Path: p, Hash: filepath.Base(p), Source: x.Source}
		}
	}
	return errors.Join(errs...)
}

// accepted.json (machine-local, in arig's config folder): skills.sh id ->
// library hashes this user accepted despite their safety flags.
func acceptedFile() string { return filepath.Join(configDir(), "accepted.json") }

func readAccepted() map[string][]string {
	a := map[string][]string{}
	b, _ := os.ReadFile(acceptedFile())
	_ = json.Unmarshal(b, &a)
	return a
}

func accepted(id, hash string) bool { return slices.Contains(readAccepted()[id], hash) }

func recordAccepted(id, hash string) error {
	a := readAccepted()
	if slices.Contains(a[id], hash) {
		return nil
	}
	a[id] = append(a[id], hash)
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(a, "", "  ")
	return os.WriteFile(acceptedFile(), append(b, '\n'), 0o600)
}

// Status reports every skill × tool in the manifest without changing anything.
func Status(dir string) ([]Link, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	m = effective(dir, m)
	return status(dir, m, resolve(dir, m, readPins(dir)), readLock(dir)), nil
}

func short(h string) string { return h[:min(len(h), 8)] }

func status(dir string, m Manifest, r map[string]resolved, lock []lockEntry) []Link {
	ours := map[string]string{} // path -> what it links to
	for _, e := range lock {
		if owned(dir, e) {
			ours[e.Path] = e.Src
		}
	}
	links := []Link{}
	for _, t := range tools(m) {
		for _, s := range m.Skills {
			x := r[s]
			rel := filepath.ToSlash(filepath.Join(toolDir(t), linkName(s)))
			l := Link{Skill: s, Tool: t, Path: rel, State: "missing", Local: x.Source == "project",
				Version: short(x.Hash), Source: x.Source}
			src, mine := ours[rel]
			switch {
			case x.State != "":
				l.State, l.Flags, l.Locked, l.Offered = x.State, x.Flags, x.Locked, x.Offered
			case mine && x.Path != "" && src == x.Path:
				l.State = "linked"
			case !mine && exists(filepath.Join(dir, rel)):
				l.State = "conflict"
			case x.Path == "":
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

// Apply makes the project match its manifest: resolves every pin (see
// resolve and fetch), records the versions in arig.lock, links them, removes
// links arig made earlier that are no longer wanted, and never touches
// anything else. Pins in accept may be installed despite safety flags.
// Returns the resulting status.
func Apply(dir string, accept ...string) ([]Link, error) {
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
	m = effective(dir, m)
	pins := readPins(dir)
	r := resolve(dir, m, pins)
	var errs []error
	if err := fetch(r, pins, accept); err != nil {
		errs = append(errs, err)
	}
	next := map[string]string{}
	for _, s := range m.Skills {
		switch x := r[s]; {
		case x.Hash != "":
			next[s] = x.Hash
		case x.Source != "project" && pins[s] != "":
			next[s] = pins[s] // not available here: keep the team's pin
		}
	}
	if !maps.Equal(next, pins) { // unchanged pins leave the committed file byte-for-byte alone
		if err := writePins(dir, next); err != nil {
			return nil, err
		}
	}

	want := map[string]string{}
	for _, l := range status(dir, m, r, nil) {
		if x := r[l.Skill]; x.Path != "" && x.State == "" {
			want[l.Path] = x.Path
		}
	}
	var lock []lockEntry
	for _, e := range readLock(dir) {
		switch {
		case !owned(dir, e):
			// gone, or replaced by the user: forget it, leave the file alone
		case e.Src != "" && want[e.Path] == e.Src && (e.Copy || target(e.Path, root, e.Src) == readlink(filepath.Join(dir, e.Path))):
			lock = append(lock, e)
		default: // no longer listed, or now resolves to another version
			if err := os.RemoveAll(filepath.Join(dir, e.Path)); err != nil {
				return nil, err
			}
		}
	}

	for _, l := range status(dir, m, r, lock) {
		if l.State != "missing" {
			continue
		}
		e, err := link(dir, root, l.Path, r[l.Skill].Path)
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
	return status(dir, m, r, lock), errors.Join(errs...)
}

func readlink(p string) string { s, _ := os.Readlink(p); return s }

// target is the relative symlink from the project path rel to src.
func target(rel, root, src string) string {
	real, err := filepath.EvalSymlinks(src)
	if err != nil {
		return ""
	}
	t, _ := filepath.Rel(filepath.Join(root, filepath.Dir(filepath.FromSlash(rel))), real)
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
	if t := target(rel, root, src); t != "" && !copyLinks {
		if os.Symlink(t, p) == nil {
			return lockEntry{Path: rel, Src: src}, nil
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
	return lockEntry{Path: rel, Src: src, Copy: true}, nil
}

func writeLock(dir string, l []lockEntry) error {
	_ = os.Remove(filepath.Join(dir, oldLock)) // moved into .arig/
	p := filepath.Join(dir, lockFile)
	if len(l) == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
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
			block = append(block, "/"+toolDir(t)+"/"+linkName(s))
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

// Skills returns the catalog entries a project uses: its own .arig/skills
// plus the pinned global skills installed on this machine (missing ones are
// left out).
func Skills(dir string) ([]catalog.Entry, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	local := localSkills(dir)
	var out []catalog.Entry
	for id, p := range local {
		fm := adapters.Frontmatter(filepath.Join(p, "SKILL.md"))
		name := fm["name"]
		if name == "" {
			name = id
		}
		cat := fm["category"]
		if !catalog.Valid(cat) {
			cat = catalog.Classify(name, fm["description"])
		}
		out = append(out, catalog.Entry{ID: id, Name: name, Description: fm["description"], Category: cat, Path: p})
	}
	for _, e := range catalog.Local(detect()) {
		if _, own := local[e.ID]; !own && slices.ContainsFunc(m.Skills, func(p string) bool { return linkName(p) == e.ID }) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b catalog.Entry) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// Edit adds (or removes) skills in dir's manifest, creating it if needed.
func Edit(dir string, add bool, ids ...string) (Manifest, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return m, err
	}
	for _, id := range ids {
		if !ValidPin(id) {
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
