// Package skillindex keeps one small "skills-index" skill in every tool. Its
// description is all a tool loads on each message (~60 tokens); its body is a
// categorized list of every skill on this computer with the path to its
// SKILL.md. The AI reads the index when a task might need a skill, then reads
// only the skill it picks, instead of every skill's description riding along
// on every message. With index mode on, the other skills are switched off in
// the tools and reached only this way.
package skillindex

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/library"
)

const Name = "skills-index"

// description is what every tool loads on every message: keep it short.
const description = "Index of every skill on this computer, by category, with the path to each SKILL.md. " +
	"At the start of a task, read this index, pick the skills that fit, then read their SKILL.md and follow it."

// Settings live in <config>/agentrig/index.json.
type Settings struct {
	Enabled   bool `json:"enabled"`   // keep the index skill in every tool
	IndexMode bool `json:"indexMode"` // other skills switched off; reached through the index
}

var (
	configDir = func() string { d, _ := os.UserConfigDir(); return filepath.Join(d, "agentrig") }
	mu        sync.Mutex
)

func settingsFile() string { return filepath.Join(configDir(), "index.json") }

// Load returns the settings; the index is on unless switched off.
func Load() Settings {
	s := Settings{Enabled: true}
	if b, err := os.ReadFile(settingsFile()); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func Save(s Settings) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(settingsFile(), append(b, '\n'), 0o644)
}

// Dir is the index skill's real folder: ~/.arig/index/skills-index.
func Dir(home string) string { return filepath.Join(home, ".arig", "index", Name) }

// Render builds the index SKILL.md from catalog entries (enabled or not).
func Render(entries []catalog.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: %s\ndescription: %s\n---\n\n", Name, description)
	b.WriteString("# Skills on this computer\n\n")
	b.WriteString("Pick the skills that fit the task, then read that skill's SKILL.md (full path below) and follow it, " +
		"including any files it points to in its folder. Skills marked *(off)* are switched off in this tool but work the same way through this index.\n")
	byCat := map[string][]catalog.Entry{}
	for _, e := range entries {
		if e.ID != Name {
			byCat[e.Category] = append(byCat[e.Category], e)
		}
	}
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	slices.SortFunc(cats, func(a, b string) int { return cmp.Or(cmp.Compare(rank(a), rank(b)), strings.Compare(a, b)) })
	for _, c := range cats {
		fmt.Fprintf(&b, "\n## %s\n\n", label(c))
		es := byCat[c]
		slices.SortFunc(es, func(a, b catalog.Entry) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
		for _, e := range es {
			off := ""
			if !e.Enabled {
				off = " *(off)*"
			}
			fmt.Fprintf(&b, "- **%s**%s: %s\n  `%s`\n", e.Name, off, short(e.Description), filepath.Join(e.Path, "SKILL.md"))
		}
	}
	return b.String()
}

// rank keeps the taxonomy's order, "other" last.
func rank(cat string) int {
	top, _, _ := strings.Cut(cat, "/")
	for i, c := range catalog.Categories {
		if c.ID == top {
			return i
		}
	}
	return len(catalog.Categories)
}

func label(cat string) string {
	top, sub, _ := strings.Cut(cat, "/")
	name := top
	for _, c := range catalog.Categories {
		if c.ID == top {
			name = c.Name
		}
	}
	if sub == "" {
		return name
	}
	up := map[string]string{"ui": "UI", "ux": "UX", "3d": "3D", "ci": "CI", "ml": "ML", "mcp": "MCP", "pdf": "PDF", "appsec": "AppSec"}
	if u, ok := up[sub]; ok {
		return name + " · " + u
	}
	return name + " · " + strings.ToUpper(sub[:1]) + sub[1:]
}

// short trims a description to one line of about 160 characters.
func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= 160 {
		return s
	}
	r := []rune(s)[:157]
	return strings.TrimRight(string(r), " ,.;:") + "…"
}

// Entries lists every skill once (the index itself excluded), preferring an
// enabled copy so the path shown is the live one.
func Entries(tools []adapters.Tool) []catalog.Entry {
	out := slices.DeleteFunc(catalog.Local(tools), func(e catalog.Entry) bool { return e.ID == Name })
	for i := range out {
		// The real folder (usually a library entry) stays put when a skill is
		// switched on or off; the tool's own path moves.
		if real, err := filepath.EvalSymlinks(out[i].Path); err == nil {
			out[i].Path = real
		}
	}
	return out
}

// Sync writes the index (only if it changed) and links it into every
// installed tool's skills folder, or removes those links when disabled.
// Returns whether the index text changed.
func Sync(home string, tools []adapters.Tool) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	s := Load()
	dirs := toolDirs(tools)
	if !s.Enabled {
		for _, d := range dirs {
			unlink(filepath.Join(d, Name), home)
		}
		return false, nil
	}
	text := Render(Entries(tools))
	md := filepath.Join(Dir(home), "SKILL.md")
	changed := false
	if old, err := os.ReadFile(md); err != nil || string(old) != text {
		if err := os.MkdirAll(Dir(home), 0o755); err != nil {
			return false, err
		}
		tmp := md + ".tmp"
		if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
			return false, err
		}
		if err := os.Rename(tmp, md); err != nil {
			return false, err
		}
		changed = true
	}
	var errs []error
	for _, d := range dirs {
		at := filepath.Join(d, Name)
		if _, err := os.Lstat(at); err == nil {
			continue // already there (ours, or the user's own skill of that name: left alone)
		}
		if _, err := os.Lstat(filepath.Join(d+"-disabled", Name)); err == nil {
			continue // the user switched the index off in this tool
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := library.Link(at, Dir(home)); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return changed, errs[0]
	}
	return changed, nil
}

// toolDirs: the main skills folder of every installed tool, each once.
func toolDirs(tools []adapters.Tool) []string {
	all := adapters.SkillDirs()
	var out []string
	for _, t := range tools {
		if !t.Installed {
			continue
		}
		ds := all[t.ID]
		if len(ds) == 0 {
			continue
		}
		d := ds[0]
		for _, x := range ds { // the shared ~/.agents/skills is where Codex and others look first
			if strings.HasSuffix(filepath.ToSlash(x), "/.agents/skills") {
				d = x
			}
		}
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// unlink removes the index link (never a real folder someone else made).
func unlink(at, home string) {
	if t, err := filepath.EvalSymlinks(at); err == nil {
		if want, err := filepath.EvalSymlinks(Dir(home)); err == nil && t == want {
			fi, _ := os.Lstat(at)
			if fi != nil && fi.Mode()&os.ModeSymlink != 0 {
				os.Remove(at)
				return
			}
			if fi != nil && fi.IsDir() && library.IsLibraryLink(at) { // Windows copy
				os.RemoveAll(at)
			}
		}
	}
}

// Cost is the per-message token estimate: the skills enabled now versus only
// the index in each tool (index mode).
func Cost(tools []adapters.Tool) (now, indexOnly int) {
	dirs := adapters.SkillDirs()
	for _, t := range tools {
		for _, it := range t.Items {
			if it.Kind == "skill" && it.Enabled {
				now += tokens(it.Name, it.Description)
			}
		}
		if t.Installed && len(dirs[t.ID]) > 0 {
			indexOnly += tokens(Name, description)
		}
	}
	return now, indexOnly
}

func tokens(name, desc string) int { return (len(name) + len(desc) + 3) / 4 }
