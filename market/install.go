package market

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/library"
)

// Caps for one downloaded skill.
const (
	maxSkill = 50 << 20
	maxFiles = 5000
	bigSkill = 1 << 20 // over this, flagged "large"
)

// LockEntry records one marketplace install in lock.json, keyed by skill ID.
type LockEntry struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`  // owner/repo on GitHub
	SkillID     string    `json:"skillId"` // also the folder name in every tool
	Hash        string    `json:"hash"`    // skills.sh version hash
	Name        string    `json:"name"`
	InstalledAt time.Time `json:"installedAt"`
	Tools       []string  `json:"tools"`
	Path        string    `json:"path"` // library entry every tool links to (older installs: a copy under the config dir)
}

// ErrConfirm means the skill carries safety flags the user hasn't accepted.
var ErrConfirm = errors.New("this skill has safety flags; review them and confirm to install")

func (m *Market) lockPath() string      { return filepath.Join(m.Dir, "lock.json") }
func (m *Market) canon(n string) string { return filepath.Join(m.cacheDir(), "skills", n) }

// Lock returns every marketplace install.
func (m *Market) Lock() map[string]LockEntry {
	l := map[string]LockEntry{}
	if b, err := os.ReadFile(m.lockPath()); err == nil {
		_ = json.Unmarshal(b, &l)
	}
	return l
}

func (m *Market) saveLock(l map[string]LockEntry) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(m.lockPath(), b)
}

var (
	scriptExt = []string{".sh", ".bash", ".zsh", ".fish", ".py", ".js", ".mjs", ".cjs", ".ts", ".mts", ".ps1", ".psm1", ".rb", ".pl", ".php", ".bat", ".cmd", ".lua"}
	networkRe = regexp.MustCompile(`\b(curl|wget)\b|\bfetch\(|\brequests\.|https?://`)
	pipeRe    = regexp.MustCompile(`(?i)\|\s*(sudo\s+)?(ba|z)?sh\b|\biex\b|invoke-expression`)
	execMagic = [][]byte{[]byte("\x7fELF"), []byte("MZ"), {0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf}, {0xce, 0xfa, 0xed, 0xfe}, {0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}}
)

var mediaExt = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp", ".avif", ".heic", ".tif", ".tiff",
	".mp3", ".wav", ".ogg", ".flac", ".m4a", ".mp4", ".webm", ".mov", ".woff", ".woff2", ".ttf", ".otf"}

// Flags lists what a reader should know before installing these files:
// scripts, network (from scripts), shell-pipe, binary, large.
func Flags(files []File) []string {
	has := map[string]bool{}
	total := 0
	for _, f := range files {
		total += len(f.Contents)
		b := []byte(f.Contents)
		exe := slices.ContainsFunc(execMagic, func(p []byte) bool { return bytes.HasPrefix(b, p) })
		// Images, audio, video and fonts are data, not programs, even though
		// they contain NUL bytes; anything else binary is worth a warning.
		if exe || (bytes.IndexByte(b, 0) >= 0 && !slices.Contains(mediaExt, strings.ToLower(path.Ext(f.Path)))) {
			has["binary"] = true
			continue
		}
		if slices.Contains(scriptExt, strings.ToLower(path.Ext(f.Path))) || strings.HasPrefix(f.Contents, "#!") {
			has["scripts"] = true
			if networkRe.MatchString(f.Contents) {
				has["network"] = true
			}
		}
		if pipeRe.MatchString(f.Contents) {
			has["shell-pipe"] = true
		}
	}
	has["large"] = total > bigSkill
	var out []string
	for _, k := range []string{"scripts", "network", "shell-pipe", "binary", "large"} {
		if has[k] {
			out = append(out, k)
		}
	}
	return out
}

// Detail is what the dashboard shows for one skill before installing it.
type Detail struct {
	Description string     `json:"description"`
	Files       []FileInfo `json:"files"`
	Flags       []string   `json:"flags"`
	Hash        string     `json:"hash"`
	Page        string     `json:"page"` // skills.sh page
	Repo        string     `json:"repo"` // GitHub repo
}

type FileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

// Describe downloads a skill and summarizes it without writing anything.
func (m *Market) Describe(id string) (Detail, error) {
	d, err := m.Get(id)
	if err != nil {
		return Detail{}, err
	}
	owner, repo, skill, _ := SplitID(id)
	out := Detail{Flags: Flags(d.Files), Hash: d.Hash,
		Page: "https://skills.sh/" + owner + "/" + repo + "/" + skill,
		Repo: "https://github.com/" + owner + "/" + repo}
	for _, f := range d.Files {
		out.Files = append(out.Files, FileInfo{f.Path, len(f.Contents)})
		if f.Path == "SKILL.md" {
			out.Description = frontmatter(f.Contents)["description"]
		}
	}
	slices.SortFunc(out.Files, func(a, b FileInfo) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

// stage writes d's files into a fresh temp folder next to the canonical
// copies (same volume, so it can be renamed into place). Every path is
// checked to stay inside it; only regular files are written, and nothing
// downloaded is ever executed.
func (m *Market) stage(d Download) (string, error) {
	if err := os.MkdirAll(filepath.Join(m.cacheDir(), "skills"), 0o755); err != nil {
		return "", err
	}
	dst, err := os.MkdirTemp(m.cacheDir(), "stage-*")
	if err != nil {
		return "", err
	}
	if err := writeFiles(d.Files, dst); err != nil {
		os.RemoveAll(dst)
		return "", err
	}
	return dst, nil
}

func writeFiles(files []File, dst string) error {
	if len(files) > maxFiles {
		return fmt.Errorf("skill has over %d files", maxFiles)
	}
	total, skill := 0, false
	for _, f := range files {
		p := f.Path
		parts := strings.Split(p, "/")
		if p == "" || path.Clean(p) != p || strings.ContainsAny(p, "\\:\x00") || slices.Contains(parts, "..") || slices.Contains(parts, ".") || !filepath.IsLocal(filepath.FromSlash(p)) {
			return fmt.Errorf("unsafe path in download: %q", p)
		}
		if slices.ContainsFunc(parts, func(s string) bool { return strings.HasPrefix(s, ".env") || s == ".git" || s == ".DS_Store" }) {
			continue
		}
		if total += len(f.Contents); total > maxSkill {
			return fmt.Errorf("skill is over %d MB", maxSkill>>20)
		}
		skill = skill || p == "SKILL.md"
		t := filepath.Join(dst, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(t), 0o755); err != nil {
			return err
		}
		// ponytail: the API carries no file modes, so nothing is made executable.
		if err := os.WriteFile(t, []byte(f.Contents), 0o644); err != nil {
			return err
		}
	}
	if !skill {
		return errors.New("download has no SKILL.md")
	}
	return nil
}

// Stage writes a download to a temporary folder and returns it with its
// library hash, so a caller can decide before anything enters the library.
// The caller removes the folder (library.Adopt moves it in).
func (m *Market) Stage(d Download) (dir, hash string, err error) {
	if dir, err = m.stage(d); err != nil {
		return "", "", err
	}
	if hash, err = library.Hash(dir); err != nil {
		os.RemoveAll(dir)
		return "", "", err
	}
	return dir, hash, nil
}

// ToLibrary puts a downloaded skill into the central library as id
// (owner/repo/skill) and returns the entry path.
func (m *Market) ToLibrary(id string, d Download) (string, error) {
	staged, _, err := m.Stage(d)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staged) // gone already if it was moved in
	return library.Adopt(staged, id)
}

// Install downloads one skill into the central library and links it into
// each tool's global skills folder (copies where links aren't available).
// Flagged skills need accept.
func (m *Market) Install(id string, tools []string, accept bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	_, _, name, err := SplitID(id)
	if err != nil {
		return err
	}
	lock := m.Lock()
	if _, ok := lock[id]; ok {
		return fmt.Errorf("%s is already installed", id)
	}
	for oid, o := range lock {
		if o.SkillID == name {
			return fmt.Errorf("the name %q is already used by %s; uninstall it first", name, oid)
		}
	}
	if len(tools) == 0 {
		return errors.New("choose at least one tool")
	}
	var dirs []string
	for _, t := range tools {
		d := m.SkillDir(t)
		if d == "" {
			return fmt.Errorf("%s has no skills folder", t)
		}
		for _, p := range []string{filepath.Join(d, name), filepath.Join(d+"-disabled", name)} {
			if _, err := os.Lstat(p); err == nil {
				return fmt.Errorf("a skill named %q already exists at %s; remove or rename it first", name, p)
			}
		}
		dirs = append(dirs, d)
	}
	dl, err := m.Get(id)
	if err != nil {
		return err
	}
	if len(Flags(dl.Files)) > 0 && !accept {
		return ErrConfirm
	}
	entry, err := m.ToLibrary(id, dl)
	if err != nil {
		return err
	}
	for i, d := range dirs {
		if err := library.Link(filepath.Join(d, name), entry); err != nil {
			for _, done := range dirs[:i+1] {
				if library.IsLibraryLink(filepath.Join(done, name)) {
					os.RemoveAll(filepath.Join(done, name))
				}
			}
			return err // the library entry stays: entries are shared and immutable
		}
	}
	src, _ := strings.CutSuffix(id, "/"+name)
	n := frontmatter(skillMD(dl))["name"]
	if n == "" {
		n = name
	}
	lock[id] = LockEntry{ID: id, Source: src, SkillID: name, Hash: dl.Hash, Name: n,
		InstalledAt: time.Now().UTC(), Tools: tools, Path: entry}
	return m.saveLock(lock)
}

// Uninstall removes exactly what Install put down: the link (or copy) in
// each recorded tool, enabled or disabled, plus an older install's private
// copy. The library entry stays. Paths are rebuilt from the validated id,
// never taken from lock.json as-is.
func (m *Market) Uninstall(id string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	lock := m.Lock()
	le, ok := lock[id]
	if !ok {
		return fmt.Errorf("%s wasn't installed from the marketplace", id)
	}
	_, _, name, err := SplitID(id)
	if err != nil {
		if !nameRe.MatchString(le.Name) { // installs from before skills.sh keyed by repo path
			return err
		}
		name = le.Name
	}
	for _, p := range m.placed(name, le.Tools) {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(m.canon(name)); err != nil {
		return err
	}
	delete(lock, id)
	m.mu.Lock()
	delete(m.updates, id)
	m.mu.Unlock()
	return m.saveLock(lock)
}

// placed lists where a skill's copies currently are (a tool may have disabled it).
func (m *Market) placed(name string, tools []string) []string {
	var out []string
	for _, t := range tools {
		d := m.SkillDir(t)
		if d == "" {
			continue
		}
		for _, p := range []string{filepath.Join(d, name), filepath.Join(d+"-disabled", name)} {
			if _, err := os.Lstat(p); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}

// Update is a newer version of an installed skill.
type Update struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	From string `json:"from"` // version hashes
	To   string `json:"to"`
}

// CheckUpdates downloads every installed skill and compares its hash with
// the installed one. Nothing is applied.
func (m *Market) CheckUpdates() []Update {
	lock := m.Lock()
	found := map[string]Update{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for id, le := range lock {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if d, err := m.Get(id); err == nil && d.Hash != le.Hash {
				mu.Lock()
				found[id] = Update{id, le.Name, le.Hash, d.Hash}
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	m.mu.Lock()
	m.updates, m.checked = found, time.Now()
	m.mu.Unlock()
	return m.Updates()
}

// Updates returns the result of the last check.
func (m *Market) Updates() []Update {
	m.mu.Lock()
	out := make([]Update, 0, len(m.updates))
	for _, u := range m.updates {
		out = append(out, u)
	}
	m.mu.Unlock()
	slices.SortFunc(out, func(a, b Update) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func (m *Market) pending(id string) (LockEntry, Download, error) {
	le, ok := m.Lock()[id]
	if !ok {
		return le, Download{}, fmt.Errorf("%s wasn't installed from the marketplace", id)
	}
	d, err := m.Get(id)
	if err != nil {
		return le, d, err
	}
	if d.Hash == le.Hash {
		return le, d, fmt.Errorf("%s is up to date", le.Name)
	}
	return le, d, nil
}

// Review diffs the installed copy against the current version and lists the
// new version's flags.
func (m *Market) Review(id string) (diff string, flags []string, err error) {
	le, d, err := m.pending(id)
	if err != nil {
		return "", nil, err
	}
	staged, err := m.stage(d)
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(staged)
	base := m.canon(le.SkillID) // installs from before the library
	if p := library.Path(le.ID, filepath.Base(le.Path)); le.Path == p {
		base = p
	}
	diff, err = DiffTrees(base, staged, 512<<10)
	return diff, Flags(d.Files), err
}

// Apply installs the current version of an installed skill everywhere it is.
func (m *Market) Apply(id string, accept bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	_, _, name, err := SplitID(id)
	if err != nil {
		return err
	}
	le, d, err := m.pending(id)
	if err != nil {
		return err
	}
	if len(Flags(d.Files)) > 0 && !accept {
		return ErrConfirm
	}
	entry, err := m.ToLibrary(id, d)
	if err != nil {
		return err
	}
	for _, p := range m.placed(name, le.Tools) {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
		if err := library.Link(p, entry); err != nil {
			return err
		}
	}
	os.RemoveAll(m.canon(name)) // an older install's private copy
	lock := m.Lock()
	le.Hash, le.Path = d.Hash, entry
	lock[id] = le
	m.mu.Lock()
	delete(m.updates, id)
	m.mu.Unlock()
	return m.saveLock(lock)
}

func frontmatter(md string) map[string]string {
	return adapters.ParseFrontmatter(strings.NewReader(md))
}

func skillMD(d Download) string {
	for _, f := range d.Files {
		if f.Path == "SKILL.md" {
			return f.Contents
		}
	}
	return ""
}

// copyTree copies the regular files and folders of src (a staged skill) to dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(t, 0o755)
		case d.Type().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			info, _ := d.Info()
			return os.WriteFile(t, b, info.Mode().Perm())
		}
		return nil
	})
}
