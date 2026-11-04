package market

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/catalog"
)

// Caps for third-party downloads.
const (
	maxDownload = 200 << 20 // the whole repo tarball, compressed
	maxSkill    = 50 << 20  // the extracted skill folder
	maxFiles    = 5000
)

var (
	repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	refRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
)

// LockEntry records one marketplace install in lock.json, keyed by entry ID.
type LockEntry struct {
	Repo        string    `json:"repo"`
	Dir         string    `json:"dir"`
	Ref         string    `json:"ref"`
	Name        string    `json:"name"` // skill folder name in every tool
	InstalledAt time.Time `json:"installedAt"`
	Tools       []string  `json:"tools"`
	Path        string    `json:"path"` // canonical copy under the config dir
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

// validate checks the index fields that end up in URLs and paths and returns
// the skill folder name: the last element of Dir, or the repo name.
func validate(e catalog.Entry) (string, error) {
	if !repoRe.MatchString(e.Repo) || slices.ContainsFunc(strings.Split(e.Repo, "/"), func(s string) bool { return s == "." || s == ".." }) {
		return "", fmt.Errorf("bad repo %q", e.Repo)
	}
	if !refRe.MatchString(e.Ref) {
		return "", fmt.Errorf("%s isn't pinned to a commit", e.ID)
	}
	if e.Dir != "" && (!filepath.IsLocal(e.Dir) || path.Clean(e.Dir) != e.Dir || slices.Contains(strings.Split(e.Dir, "/"), "..")) {
		return "", fmt.Errorf("bad folder %q", e.Dir)
	}
	n := path.Base(e.Dir)
	if e.Dir == "" || e.Dir == "." {
		n = path.Base(e.Repo)
	}
	if !nameRe.MatchString(n) {
		return "", fmt.Errorf("unusable skill folder name %q", n)
	}
	return n, nil
}

// Install downloads one skill at its pinned commit and copies it into each
// tool's global skills folder. Flagged skills need accept.
func (m *Market) Install(id string, tools []string, accept bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	e, ok := m.Entry(id)
	if !ok {
		return errNotIndexed
	}
	name, err := validate(e)
	if err != nil {
		return err
	}
	if len(e.Flags) > 0 && !accept {
		return ErrConfirm
	}
	lock := m.Lock()
	if _, ok := lock[id]; ok {
		return fmt.Errorf("%s is already installed", e.Name)
	}
	for oid, o := range lock {
		if o.Name == name {
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

	staged, err := m.stage(e)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	canon := m.canon(name)
	os.RemoveAll(canon) // not in the lock, so a leftover from an interrupted install
	if err := os.Rename(staged, canon); err != nil {
		return err
	}
	for i, d := range dirs {
		if err := copyTree(canon, filepath.Join(d, name)); err != nil {
			for _, done := range dirs[:i+1] {
				os.RemoveAll(filepath.Join(done, name))
			}
			os.RemoveAll(canon)
			return err
		}
	}
	lock[id] = LockEntry{Repo: e.Repo, Dir: e.Dir, Ref: e.Ref, Name: name, InstalledAt: time.Now().UTC(), Tools: tools, Path: canon}
	return m.saveLock(lock)
}

// Uninstall removes exactly what Install put down: the copy in each recorded
// tool (enabled or disabled) and the canonical copy. Paths are rebuilt from
// the validated name, never taken from lock.json as-is.
func (m *Market) Uninstall(id string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	lock := m.Lock()
	le, ok := lock[id]
	if !ok {
		return fmt.Errorf("%s wasn't installed from the marketplace", id)
	}
	if !nameRe.MatchString(le.Name) {
		return fmt.Errorf("lock.json: bad name %q", le.Name)
	}
	for _, p := range m.placed(le) {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(m.canon(le.Name)); err != nil {
		return err
	}
	delete(lock, id)
	return m.saveLock(lock)
}

// placed lists where le's copies currently are (a tool may have disabled it).
func (m *Market) placed(le LockEntry) []string {
	var out []string
	for _, t := range le.Tools {
		d := m.SkillDir(t)
		if d == "" {
			continue
		}
		for _, p := range []string{filepath.Join(d, le.Name), filepath.Join(d+"-disabled", le.Name)} {
			if _, err := os.Lstat(p); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}

// Update is an available newer commit for an installed skill.
type Update struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Updates compares lock.json with the index. Nothing is applied.
func (m *Market) Updates() []Update {
	var out []Update
	for id, le := range m.Lock() {
		if e, ok := m.Entry(id); ok && e.Ref != "" && e.Ref != le.Ref {
			out = append(out, Update{id, e.Name, le.Ref, e.Ref})
		}
	}
	slices.SortFunc(out, func(a, b Update) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Review fetches the new commit and diffs it against the installed copy.
func (m *Market) Review(id string) (string, error) {
	le, e, err := m.pending(id)
	if err != nil {
		return "", err
	}
	staged, err := m.stage(e)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staged)
	return DiffTrees(m.canon(le.Name), staged, 512<<10)
}

func (m *Market) pending(id string) (LockEntry, catalog.Entry, error) {
	le, ok := m.Lock()[id]
	if !ok {
		return le, catalog.Entry{}, fmt.Errorf("%s wasn't installed from the marketplace", id)
	}
	e, ok := m.Entry(id)
	if !ok {
		return le, e, errNotIndexed
	}
	if e.Ref == le.Ref {
		return le, e, fmt.Errorf("%s is up to date", e.Name)
	}
	name, err := validate(e)
	if err != nil {
		return le, e, err
	}
	if name != le.Name {
		return le, e, fmt.Errorf("%s was renamed upstream; uninstall and install it again", le.Name)
	}
	return le, e, nil
}

// Apply installs the new commit of an installed skill everywhere it is.
func (m *Market) Apply(id string, accept bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	le, e, err := m.pending(id)
	if err != nil {
		return err
	}
	if len(e.Flags) > 0 && !accept {
		return ErrConfirm
	}
	staged, err := m.stage(e)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	canon := m.canon(le.Name)
	old := canon + ".old"
	os.RemoveAll(old)
	if err := os.Rename(canon, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Rename(staged, canon); err != nil {
		os.Rename(old, canon)
		return err
	}
	os.RemoveAll(old)
	for _, p := range m.placed(le) {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
		if err := copyTree(canon, p); err != nil {
			return err
		}
	}
	lock := m.Lock()
	le.Ref, le.Repo, le.Dir = e.Ref, e.Repo, e.Dir
	lock[id] = le
	return m.saveLock(lock)
}

// stage downloads e's repo at e.Ref into a fresh temp folder next to the
// canonical copies (same volume, so it can be renamed into place).
func (m *Market) stage(e catalog.Entry) (string, error) {
	if err := os.MkdirAll(filepath.Join(m.cacheDir(), "skills"), 0o755); err != nil {
		return "", err
	}
	dst, err := os.MkdirTemp(m.cacheDir(), "stage-*")
	if err != nil {
		return "", err
	}
	if err := m.download(e, dst); err != nil {
		os.RemoveAll(dst)
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		os.RemoveAll(dst)
		return "", fmt.Errorf("%s has no SKILL.md at this commit", e.ID)
	}
	return dst, nil
}

func (m *Market) download(e catalog.Entry, dst string) error {
	res, err := m.HTTP.Get(fmt.Sprintf("%s/%s/tar.gz/%s", m.Codeload, e.Repo, e.Ref))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("download %s: %s", e.Repo, res.Status)
	}
	return extract(&capped{res.Body, maxDownload}, e.Dir, dst)
}

var errTooBig = errors.New("download is too large")

type capped struct {
	r io.Reader
	n int64
}

func (c *capped) Read(p []byte) (int, error) {
	if c.n <= 0 {
		return 0, errTooBig
	}
	if int64(len(p)) > c.n {
		p = p[:c.n]
	}
	n, err := c.r.Read(p)
	c.n -= int64(n)
	return n, err
}

// extract writes the files under dir of a codeload tarball (one top-level
// folder wrapping the repo) into dst. Only regular files and folders are
// written; nothing is ever executed.
// ponytail: symlinks and hard links are dropped rather than checked, so no
// write can be redirected outside dst; support in-tree links if skills need them.
func extract(r io.Reader, dir, dst string) error {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	var total int64
	files := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		_, rel, ok := strings.Cut(h.Name, "/") // drop "owner-repo-sha/"
		if !ok {
			continue // pax_global_header
		}
		rel = strings.TrimSuffix(rel, "/")
		if dir != "" && dir != "." {
			var in bool
			if rel, in = strings.CutPrefix(rel, dir+"/"); !in {
				continue
			}
		}
		if rel == "" {
			continue
		}
		parts := strings.Split(rel, "/")
		if strings.HasPrefix(rel, "/") || slices.Contains(parts, "..") || !filepath.IsLocal(filepath.FromSlash(rel)) {
			return fmt.Errorf("unsafe path in download: %q", h.Name)
		}
		if slices.ContainsFunc(parts, func(p string) bool { return strings.HasPrefix(p, ".env") || p == ".git" || p == ".DS_Store" }) {
			continue
		}
		p := filepath.Join(dst, filepath.FromSlash(rel))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			files++
			total += h.Size
			if files > maxFiles || total > maxSkill {
				return fmt.Errorf("skill folder is over the limit (%d files / %d MB)", maxFiles, maxSkill>>20)
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			mode := fs.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(tr, h.Size))
			if err2 := f.Close(); err == nil {
				err = err2
			}
			if err != nil {
				return err
			}
		}
	}
	if files == 0 {
		return fmt.Errorf("folder %q not found at this commit", dir)
	}
	return nil
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
