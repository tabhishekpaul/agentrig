// Package library is the one central, content-addressed skill store, like
// bun's or pnpm's: every skill version lives once at
//
//	~/.arig/library/<name>/<hash>/      name = folder name, or owner/repo/skill for skills.sh
//
// and everything else (tool skill folders, project folders) links into it.
// <hash> is the first 12 hex digits of sha256 over the skill's files, so an
// entry never changes once written; a new version is a new entry.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/gitsync"
)

// Rel is where the library sits, relative to home.
const Rel = gitsync.LibraryDir

const (
	hashLen    = 12
	copyMarker = ".arig-library" // inside copies made instead of links; holds the entry path
)

// copyLinks makes Link copy instead of linking (tests flip it).
var copyLinks = false

var (
	segRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	hashRe = regexp.MustCompile(`^[0-9a-f]{12}$`)
)

// ValidHash reports whether h looks like a library version hash.
func ValidHash(h string) bool { return hashRe.MatchString(h) }

// Root is this user's library folder.
func Root() string { home, _ := os.UserHomeDir(); return RootAt(home) }

// RootAt is the library folder for home.
func RootAt(home string) string { return filepath.Join(home, filepath.FromSlash(Rel)) }

// ValidName: a plain folder name, or owner/repo/skill.
func ValidName(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) != 1 && len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if !segRe.MatchString(p) || p == ".." {
			return false
		}
	}
	return true
}

// Hash names a skill version: sha256 over its files sorted by relative path
// (a symlink contributes its target text), skipping what sync skips
// (gitsync.Ignored), so the same skill hashes the same on every machine.
func Hash(dir string) (string, error) {
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	type file struct{ rel, path, link string }
	var files []file
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		if gitsync.Ignored(d.Name(), d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			files = append(files, file{rel: filepath.ToSlash(rel), link: t})
		case d.Type().IsRegular():
			files = append(files, file{rel: filepath.ToSlash(rel), path: p})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	slices.SortFunc(files, func(a, b file) int { return strings.Compare(a.rel, b.rel) })
	h := sha256.New()
	for _, f := range files {
		if f.path == "" {
			fmt.Fprintf(h, "l %d:%s %d:%s\n", len(f.rel), f.rel, len(f.link), f.link)
			continue
		}
		b, err := os.ReadFile(f.path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "f %d:%s %d\n", len(f.rel), f.rel, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:hashLen], nil
}

// store is a library root; the exported functions use the user's.
type store string

func (s store) path(name, hash string) string {
	return filepath.Join(string(s), filepath.FromSlash(name), hash)
}

// Path is where version hash of name lives (whether or not it exists).
func Path(name, hash string) string { return store(Root()).path(name, hash) }

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }

// Version is one library entry.
type Version struct {
	Hash string    `json:"hash"`
	Path string    `json:"path"`
	Time time.Time `json:"time"`
}

// Versions lists name's entries, newest first.
// ponytail: "newest" is the entry folder's mtime, which a sync checkout resets;
// add an added-at record if picking among synced versions needs to be exact.
func Versions(name string) []Version { return store(Root()).versions(name) }

func (s store) versions(name string) []Version {
	if !ValidName(name) {
		return nil
	}
	dir := filepath.Join(string(s), filepath.FromSlash(name))
	entries, _ := os.ReadDir(dir)
	var out []Version
	for _, e := range entries {
		if !hashRe.MatchString(e.Name()) || !e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Version{Hash: e.Name(), Path: filepath.Join(dir, e.Name()), Time: fi.ModTime()})
	}
	slices.SortFunc(out, func(a, b Version) int { return b.Time.Compare(a.Time) })
	return out
}

// Put copies srcDir into the library as name (files sync skips are left
// out) and returns the entry's path and hash. An identical entry is reused.
func Put(srcDir, name string) (string, string, error) { return store(Root()).put(srcDir, name) }

func (s store) put(src, name string) (string, string, error) {
	if !ValidName(name) {
		return "", "", fmt.Errorf("bad library name %q", name)
	}
	h, err := Hash(src)
	if err != nil {
		return "", "", err
	}
	dst := s.path(name, h)
	if isDir(dst) {
		return dst, h, nil
	}
	if err := os.MkdirAll(string(s), 0o755); err != nil {
		return "", "", err
	}
	tmp, err := os.MkdirTemp(string(s), ".tmp-*") // same volume: renamed into place whole
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmp)
	real, _ := filepath.EvalSymlinks(src)
	if err := copyTree(real, tmp, true); err != nil {
		return "", "", err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", "", err
	}
	return dst, h, place(tmp, dst)
}

// place renames a finished folder into its entry path. Losing a race to an
// identical entry is fine: same hash, same content.
func place(from, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, dst); err != nil && !isDir(dst) {
		return err
	}
	return nil
}

// Adopt moves srcDir into the library as name when it can (same volume),
// else copies it, and returns the entry path. If the entry exists already,
// srcDir is left as it is. Callers check whether srcDir is still there.
func Adopt(srcDir, name string) (string, error) {
	p, _, err := store(Root()).adopt(srcDir, name)
	return p, err
}

func (s store) adopt(src, name string) (string, bool, error) {
	if !ValidName(name) {
		return "", false, fmt.Errorf("bad library name %q", name)
	}
	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() {
		return "", false, fmt.Errorf("%s is not a folder", src)
	}
	h, err := Hash(src)
	if err != nil {
		return "", false, err
	}
	dst := s.path(name, h)
	if isDir(dst) {
		return dst, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", false, err
	}
	if os.Rename(src, dst) == nil { // one atomic step: the entry appears complete
		return dst, true, nil
	}
	dst, _, err = s.put(src, name) // another volume
	return dst, false, err
}

// Link makes at point to the library entry target with a relative symlink
// (so it survives syncing to a machine whose home path differs). Where
// symlinks aren't available it copies the entry and drops a marker file in
// the copy instead.
func Link(at, target string) error {
	if _, err := os.Lstat(at); err == nil {
		return fmt.Errorf("%s already exists", at)
	}
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	if !copyLinks && LinkDir(at, real) {
		return nil
	}
	if err := copyTree(real, at, false); err != nil {
		os.RemoveAll(at)
		return err
	}
	return os.WriteFile(filepath.Join(at, copyMarker), []byte(real+"\n"), 0o644)
}

// Target is the library entry p links to (or is a marked copy of), else "".
func Target(p string) string { return target(Root(), p) }

func target(root, p string) string {
	fi, err := os.Lstat(p)
	if err != nil {
		return ""
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		b, err := os.ReadFile(filepath.Join(p, copyMarker))
		if err != nil {
			return ""
		}
		p = strings.TrimSpace(string(b))
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || !filepath.IsLocal(rel) || !hashRe.MatchString(filepath.Base(rel)) {
		return ""
	}
	return real
}

// Revise makes a new version of the entry link points to: a copy of it,
// changed by change, put back under the same name. Every link to the old
// version in dirs (and their -disabled siblings) is moved to the new one;
// links elsewhere (projects pinned by arig.lock) stay on the old version.
// Returns the new entry.
func Revise(link string, dirs []string, change func(dir string) error) (string, error) {
	root := Root()
	old := target(root, link)
	if old == "" {
		return "", fmt.Errorf("%s isn't in the library", link)
	}
	realRoot, _ := filepath.EvalSymlinks(root)
	rel, _ := filepath.Rel(realRoot, filepath.Dir(old))
	name := filepath.ToSlash(rel)
	tmp, err := os.MkdirTemp("", "arig-revise-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	work := filepath.Join(tmp, "s")
	if err := copyTree(old, work, false); err != nil {
		return "", err
	}
	if err := change(work); err != nil {
		return "", err
	}
	entry, _, err := Put(work, name)
	if err != nil || entry == old {
		return entry, err
	}
	var errs []error
	for _, d := range dirs {
		for _, dir := range []string{d, d + "-disabled"} {
			es, _ := os.ReadDir(dir)
			for _, e := range es {
				p := filepath.Join(dir, e.Name())
				if target(root, p) != old {
					continue
				}
				if err := os.RemoveAll(p); err != nil {
					errs = append(errs, err)
				} else if err := Link(p, entry); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return entry, errors.Join(errs...)
}

// IsLibraryLink reports whether p is a link into the library (or a marked copy of an entry).
func IsLibraryLink(p string) bool { return Target(p) != "" }

// copyTree copies src to dst keeping file modes and symlinks (as links).
// With skip, files sync ignores are left out (what Hash leaves out).
func copyTree(src, dst string, skip bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != src && skip && gitsync.Ignored(d.Name(), d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(t, out)
		case d.IsDir():
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			return os.Chmod(out, info.Mode().Perm()|0o700)
		case d.Type().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(out, b, info.Mode().Perm())
		}
		return nil // sockets, devices: never part of a skill
	})
}

// Summary is what the dashboard shows about the library.
type Summary struct {
	Root     string     `json:"root"`
	Entries  int        `json:"entries"`  // distinct names
	Versions int        `json:"versions"` // name@hash folders
	Bytes    int64      `json:"bytes"`
	Links    int        `json:"links"` // tool skill folders that link into it
	Last     *LogInfo   `json:"lastMigration,omitempty"`
	Names    []NameInfo `json:"names"`
}

type NameInfo struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"` // hashes, newest first
}

// Summarize scans the library under home; dirs are the absolute tool skill
// folders to count links in.
func Summarize(home string, dirs []string) Summary {
	root := RootAt(home)
	s := Summary{Root: root, Names: []NameInfo{}}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != root {
			return filepath.SkipDir
		}
		if d.IsDir() && hashRe.MatchString(d.Name()) {
			rel, _ := filepath.Rel(root, filepath.Dir(p))
			name := filepath.ToSlash(rel)
			if ValidName(name) {
				if n := len(s.Names); n > 0 && s.Names[n-1].Name == name {
					s.Names[n-1].Versions = append(s.Names[n-1].Versions, d.Name())
				} else {
					s.Names = append(s.Names, NameInfo{Name: name, Versions: []string{d.Name()}})
				}
				s.Versions++
			}
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				s.Bytes += fi.Size()
			}
		}
		return nil
	})
	for i, n := range s.Names {
		var hs []string
		for _, v := range store(root).versions(n.Name) {
			hs = append(hs, v.Hash)
		}
		s.Names[i].Versions = hs
	}
	s.Entries = len(s.Names)
	seen := map[string]bool{}
	for _, d := range dirs {
		for _, dir := range []string{d, d + "-disabled"} {
			if r, err := filepath.EvalSymlinks(dir); err != nil || seen[r] {
				continue
			} else {
				seen[r] = true
			}
			es, _ := os.ReadDir(dir)
			for _, e := range es {
				if target(root, filepath.Join(dir, e.Name())) != "" {
					s.Links++
				}
			}
		}
	}
	s.Last = lastLog(root)
	return s
}

var errNotLink = errors.New("not a library link")
