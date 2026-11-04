package projects

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Candidate is a folder that looks like a project: AI tools ran in it, an
// editor opened it, or it's a git repo in a usual code folder.
type Candidate struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Sources    []string  `json:"sources"` // e.g. "Claude Code · 12 sessions", "VS Code", "git"
	LastActive time.Time `json:"lastActive"`
	Added      bool      `json:"added"`
}

// Discover lists likely projects under home, most recently active first.
func Discover(home string) []Candidate {
	byPath := map[string]*Candidate{}
	add := func(p, source string, at time.Time) {
		p = filepath.Clean(p)
		if !looksLikeProject(home, p) {
			return
		}
		c := byPath[sameKey(p)]
		if c == nil {
			c = &Candidate{Path: p, Name: filepath.Base(p)}
			byPath[sameKey(p)] = c
		}
		if !slices.Contains(c.Sources, source) {
			c.Sources = append(c.Sources, source)
		}
		if at.After(c.LastActive) {
			c.LastActive = at
		}
	}
	claudeSessions(home, add)
	codexProjects(home, add)
	editorWorkspaces(home, add)
	gitRepos(home, add)

	known := List()
	out := make([]Candidate, 0, len(byPath))
	for _, c := range byPath {
		c.Added = slices.Contains(known, c.Path)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastActive.Equal(out[j].LastActive) {
			return out[i].LastActive.After(out[j].LastActive)
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// looksLikeProject keeps existing folders inside home, below it, and not in
// hidden or system folders (tool caches, temp worktrees, Library).
func looksLikeProject(home, p string) bool {
	rel, err := filepath.Rel(home, p)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return false
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	if top := strings.Split(rel, string(filepath.Separator))[0]; top == "Library" || top == "AppData" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

type addFunc func(path, source string, at time.Time)

// claudeSessions: ~/.claude/projects/<encoded>/<session>.jsonl; each session
// line records the folder it ran in ("cwd").
func claudeSessions(home string, add addFunc) {
	dirs, _ := os.ReadDir(filepath.Join(home, ".claude", "projects"))
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(home, ".claude", "projects", d.Name())
		files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		var newest string
		var at time.Time
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil && fi.ModTime().After(at) {
				newest, at = f, fi.ModTime()
			}
		}
		if newest == "" {
			continue
		}
		if cwd := sessionCwd(newest); cwd != "" {
			n := len(files)
			label := "Claude Code · 1 session"
			if n != 1 {
				label = "Claude Code · " + strconv.Itoa(n) + " sessions"
			}
			add(cwd, label, at)
		}
	}
}

// sessionCwd reads the first "cwd" from the start of a session file.
func sessionCwd(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for i := 0; i < 50 && s.Scan(); i++ {
		var l struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(s.Bytes(), &l) == nil && l.Cwd != "" {
			return l.Cwd
		}
	}
	return ""
}

var codexProject = regexp.MustCompile(`^\[projects\."((?:[^"\\]|\\.)+)"\]`)

// codexProjects: folders Codex was trusted in, from ~/.codex/config.toml.
// (Its session files are compressed, so they're not read.)
func codexProjects(home string, add addFunc) {
	f, err := os.Open(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if m := codexProject.FindStringSubmatch(strings.TrimSpace(s.Text())); m != nil {
			p := strings.ReplaceAll(m[1], `\\`, `\`)
			at := time.Time{}
			if fi, err := os.Stat(p); err == nil {
				at = fi.ModTime()
			}
			add(p, "Codex", at)
		}
	}
}

// editorWorkspaces: recent folders of VS Code-based editors, from
// <config>/<Editor>/User/workspaceStorage/*/workspace.json.
func editorWorkspaces(home string, add addFunc) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return
	}
	if runtime.GOOS == "darwin" && !strings.HasPrefix(cfg, home) { // tests point home elsewhere
		cfg = filepath.Join(home, "Library", "Application Support")
	}
	for _, ed := range []struct{ dir, name string }{
		{"Code", "VS Code"}, {"Cursor", "Cursor"}, {"Windsurf", "Windsurf"}, {"VSCodium", "VSCodium"}, {"Kiro", "Kiro"}, {"Antigravity", "Antigravity"},
	} {
		root := filepath.Join(cfg, ed.dir, "User", "workspaceStorage")
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			file := filepath.Join(root, e.Name(), "workspace.json")
			b, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			var w struct {
				Folder string `json:"folder"`
			}
			if json.Unmarshal(b, &w) != nil || !strings.HasPrefix(w.Folder, "file://") {
				continue // remote and multi-root workspaces are skipped
			}
			u, err := url.Parse(w.Folder)
			if err != nil {
				continue
			}
			at := time.Time{}
			if fi, err := os.Stat(filepath.Join(root, e.Name())); err == nil {
				at = fi.ModTime()
			}
			add(filepath.FromSlash(u.Path), ed.name, at)
		}
	}
}

// codeRoots are the folders people keep code in.
var codeRoots = []string{"development", "dev", "Developer", "code", "Code", "projects", "Projects", "src", "repos", "work",
	"workspace", "git", "GitHub", "Documents/GitHub", "Documents/Projects", "AndroidStudioProjects", "StudioProjects", "go/src"}

// gitRepos finds git repos up to three levels below the usual code folders.
func gitRepos(home string, add addFunc) {
	const maxDepth, maxRepos = 3, 400
	found := 0
	seen := map[string]bool{}
	for _, r := range codeRoots {
		root := filepath.Join(home, filepath.FromSlash(r))
		if seen[sameKey(root)] { // "code" and "Code" are one folder on macOS/Windows
			continue
		}
		seen[sameKey(root)] = true
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() || found >= maxRepos {
				if found >= maxRepos {
					return filepath.SkipAll
				}
				return nil
			}
			name := d.Name()
			if p != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "build" || name == "dist") {
				return filepath.SkipDir
			}
			if git, err := os.Stat(filepath.Join(p, ".git")); err == nil {
				at := git.ModTime()
				if fi, err := os.Stat(filepath.Join(p, ".git", "index")); err == nil {
					at = fi.ModTime() // changes on every commit/checkout/add
				}
				add(p, "git", at)
				found++
				return filepath.SkipDir // nested repos are usually submodules
			}
			if rel, _ := filepath.Rel(root, p); strings.Count(rel, string(filepath.Separator)) >= maxDepth-1 {
				return filepath.SkipDir
			}
			return nil
		})
	}
}

// sameKey: macOS and Windows file names ignore case by default, so "Code" and
// "code" are the same folder there.
func sameKey(p string) string {
	if runtime.GOOS == "linux" {
		return p
	}
	return strings.ToLower(p)
}
