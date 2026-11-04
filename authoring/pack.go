package authoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/gitsync"
)

// Packs keeps the user's packs in one JSON file (UserConfigDir/agentrig/packs.json).
type Packs struct {
	Path string
	mu   sync.Mutex
}

func (s *Packs) List() ([]catalog.Pack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Packs) read() ([]catalog.Pack, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []catalog.Pack{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ps []catalog.Pack
	return ps, json.Unmarshal(b, &ps)
}

// Get returns the pack with id.
func (s *Packs) Get(id string) (catalog.Pack, error) {
	ps, err := s.List()
	if err != nil {
		return catalog.Pack{}, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p, nil
		}
	}
	return catalog.Pack{}, fmt.Errorf("no pack %q", id)
}

// Save adds p, or replaces the pack with the same id.
func (s *Packs) Save(p catalog.Pack) error {
	if !ValidID(p.ID) {
		return fmt.Errorf("pack id %q must be lowercase kebab-case", p.ID)
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.Category != "" && !catalog.Valid(p.Category) {
		return fmt.Errorf("unknown category %q", p.Category)
	}
	for _, id := range p.Skills {
		if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") { // installed folder names, not always kebab
			return fmt.Errorf("bad skill id %q", id)
		}
	}
	if p.Skills == nil {
		p.Skills = []string{}
	}
	return s.update(func(ps []catalog.Pack) []catalog.Pack {
		if i := slices.IndexFunc(ps, func(x catalog.Pack) bool { return x.ID == p.ID }); i >= 0 {
			ps[i] = p
			return ps
		}
		return append(ps, p)
	})
}

func (s *Packs) Delete(id string) error {
	return s.update(func(ps []catalog.Pack) []catalog.Pack {
		return slices.DeleteFunc(ps, func(x catalog.Pack) bool { return x.ID == id })
	})
}

func (s *Packs) update(f func([]catalog.Pack) []catalog.Pack) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.read()
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(f(ps), "", "  ")
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// TogglePack switches every skill of p on or off in every tool that has it
// (dirs is adapters.SkillDirs()). Returns how many tool/skill pairs it touched.
func TogglePack(p catalog.Pack, dirs map[string][]string, enabled bool) (int, error) {
	n := 0
	var errs []error
	for tool, ds := range dirs {
		for _, id := range p.Skills {
			for _, d := range ds {
				if exists(filepath.Join(d, id)) || exists(filepath.Join(d+"-disabled", id)) {
					if err := adapters.Toggle(tool, "skill", id, enabled); err != nil {
						errs = append(errs, fmt.Errorf("%s/%s: %w", tool, id, err))
					} else {
						n++
					}
					break
				}
			}
		}
	}
	return n, errors.Join(errs...)
}

// reserved plugin name prefixes Claude Code refuses as passing for its own.
var reserved = []string{"claude", "anthropic", "anthropics", "cc-plugin"}

func checkPluginName(id string) error {
	if !ValidID(id) {
		return fmt.Errorf("plugin name %q must be lowercase kebab-case", id)
	}
	for _, r := range reserved {
		if id == r || strings.HasPrefix(id, r+"-") {
			return fmt.Errorf("plugin name %q is reserved by Claude Code; rename the pack", id)
		}
	}
	return nil
}

// ExportDir resolves a user-typed export folder and checks it is inside home
// and missing or empty, so an export never mixes into existing files.
func ExportDir(home, dir string) (string, error) {
	if rest, ok := strings.CutPrefix(dir, "~/"); ok {
		dir = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("export folder must be an absolute path or start with ~/")
	}
	dir = filepath.Clean(dir)
	real := dir
	if r, err := filepath.EvalSymlinks(dir); err == nil { // a link must not point out of home
		real = r
	}
	h, _ := filepath.EvalSymlinks(home)
	for _, root := range []string{home, h} {
		if rel, err := filepath.Rel(root, real); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
				return "", fmt.Errorf("%s isn't empty; pick a new or empty folder", dir)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
			return dir, nil
		}
	}
	return "", errors.New("export folder must be inside your home folder")
}

// Manifest is .claude-plugin/plugin.json (Claude Code plugin manifest; only
// name is required, the rest keeps `claude plugin validate` warning-free).
type Manifest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName,omitempty"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Author      *Author  `json:"author,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

type Author struct {
	Name string `json:"name"`
}

// WritePlugin lays out p as a Claude Code plugin in dst:
//
//	.claude-plugin/plugin.json
//	skills/<id>/SKILL.md (+ the skill's other files)
//
// skillPath maps a skill id to its installed folder ("" if not installed).
func WritePlugin(dst string, p catalog.Pack, author string, skillPath func(string) string) error {
	if err := checkPluginName(p.ID); err != nil {
		return err
	}
	if len(p.Skills) == 0 {
		return errors.New("the pack has no skills")
	}
	for _, id := range p.Skills {
		src := skillPath(id)
		if src == "" {
			return fmt.Errorf("skill %q isn't installed here", id)
		}
		if err := CopySkill(src, filepath.Join(dst, "skills", id)); err != nil {
			return err
		}
	}
	if p.Description == "" {
		p.Description = "Skills: " + strings.Join(p.Skills, ", ")
	}
	m := Manifest{Name: p.ID, DisplayName: p.Name, Version: "0.1.0", Description: p.Description,
		Keywords: []string{"arig-pack"}}
	if author != "" {
		m.Author = &Author{author}
	}
	if p.Repo != "" {
		m.Repository = "https://github.com/" + p.Repo
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.MkdirAll(filepath.Join(dst, ".claude-plugin"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, ".claude-plugin", "plugin.json"), append(b, '\n'), 0o644)
}

// CopySkill copies a skill folder (following a symlinked root) to dst,
// skipping what gitsync never shares (.env*, files over 10 MB, junk) and any
// symlinks inside it, which could point anywhere on this machine.
func CopySkill(src, dst string) error {
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != root && gitsync.Ignored(d.Name(), d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(out, 0o755)
		case d.Type().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			info, _ := d.Info()
			return os.WriteFile(out, data, info.Mode().Perm())
		}
		return nil // symlinks, sockets, …
	})
}
