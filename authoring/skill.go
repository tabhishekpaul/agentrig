// Package authoring creates and edits skills, groups them into packs, exports
// packs as Claude Code plugins and publishes either to GitHub.
package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/library"
)

var idRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidID reports whether id is plain kebab-case, safe as a folder or repo name.
func ValidID(id string) bool { return len(id) <= 64 && idRe.MatchString(id) }

// Skill is what the create form sends.
type Skill struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Body        string   `json:"body"`
	Tools       []string `json:"tools"`
}

// Canonical is the tool-neutral skill folder (~/.agents/skills, read by
// Codex and others). A new skill with no tools chosen is linked there.
func Canonical(home string) string { return filepath.Join(home, ".agents", "skills") }

// Create writes the new skill into the central library (~/.arig/library/<id>/<hash>)
// and links it into each chosen tool's skill folder (relative links, so they
// survive sync to another machine; copies where links aren't available).
// dirs is adapters.SkillDirs(). It never overwrites: an id that exists
// anywhere (enabled or disabled, in any tool) is refused. Returns the
// library entry.
func Create(home string, dirs map[string][]string, s Skill) (string, error) {
	if !ValidID(s.ID) {
		return "", fmt.Errorf("id %q must be lowercase kebab-case (a-z, 0-9, -)", s.ID)
	}
	if strings.TrimSpace(s.Description) == "" || len(s.Description) > 1024 || strings.ContainsAny(s.Description, "\r\n") {
		return "", errors.New("description is required: one line, up to 1024 characters")
	}
	if s.Category != "" && !catalog.Valid(s.Category) {
		return "", fmt.Errorf("unknown category %q", s.Category)
	}
	for _, t := range s.Tags {
		if !ValidID(t) {
			return "", fmt.Errorf("tag %q must be lowercase kebab-case", t)
		}
	}
	canon := Canonical(home)
	all := [][]string{{canon}}
	for _, ds := range dirs {
		all = append(all, ds)
	}
	for _, ds := range all {
		for _, d := range ds {
			if exists(filepath.Join(d, s.ID)) || exists(filepath.Join(d+"-disabled", s.ID)) {
				return "", fmt.Errorf("a skill named %q already exists in %s", s.ID, d)
			}
		}
	}
	links := []string{}
	for _, t := range s.Tools {
		ds, ok := dirs[t]
		if !ok {
			return "", fmt.Errorf("%q doesn't read skills", t)
		}
		if l := filepath.Join(linkDir(ds), s.ID); !slices.Contains(links, l) {
			links = append(links, l)
		}
	}
	if len(links) == 0 {
		links = append(links, filepath.Join(canon, s.ID))
	}

	tmp, err := os.MkdirTemp("", "arig-new-skill-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := os.WriteFile(filepath.Join(tmp, "SKILL.md"), []byte(skillMD(s)), 0o644); err != nil {
		return "", err
	}
	entry, _, err := library.Put(tmp, s.ID)
	if err != nil {
		return "", err
	}
	for i, l := range links {
		if err := library.Link(l, entry); err != nil { // undo, so a retry starts clean
			for _, done := range links[:i] {
				os.RemoveAll(done)
			}
			return "", err
		}
	}
	return entry, nil
}

// Save replaces a skill's SKILL.md. A skill that links into the library
// gets a new library version instead (entries never change), and every
// tool link to the old version moves to it; dirs is adapters.SkillDirs().
func Save(p, content string, dirs map[string][]string) error {
	if library.Target(p) == "" {
		return os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(content), 0o644)
	}
	var all []string
	for _, ds := range dirs {
		all = append(all, ds...)
	}
	_, err := library.Revise(p, all, func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
	})
	return err
}

// linkDir is the tool's first skill folder that exists, else its first one.
func linkDir(ds []string) string {
	for _, d := range ds {
		if exists(d) {
			return d
		}
	}
	return ds[0]
}

func skillMD(s Skill) string {
	q, _ := json.Marshal(strings.TrimSpace(s.Description)) // a JSON string is a valid YAML scalar
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: %s\ndescription: %s\n", s.ID, q)
	if s.Category != "" {
		fmt.Fprintf(&b, "category: %s\n", s.Category)
	}
	if len(s.Tags) > 0 {
		fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(s.Tags, ", "))
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(s.Body))
	b.WriteString("\n")
	return b.String()
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// drafters are the AI CLIs that can write a first draft, in preference order.
var drafters = []struct {
	name string
	args func(prompt string) []string
}{
	{"claude", func(p string) []string { return []string{"-p", p} }},
	{"codex", func(p string) []string { return []string{"exec", "--skip-git-repo-check", p} }},
}

// Drafter returns the first drafting CLI on PATH, or "".
func Drafter() string {
	for _, d := range drafters {
		if _, err := exec.LookPath(d.name); err == nil {
			return d.name
		}
	}
	return ""
}

// Draft asks the user's own AI CLI for a SKILL.md body. The caller shows it
// for review; nothing is saved here.
func Draft(name, description string) (string, error) {
	cli := Drafter()
	if cli == "" {
		return "", errors.New("neither claude nor codex is on PATH")
	}
	prompt := fmt.Sprintf("Write the body of an Agent Skill (SKILL.md) named %q whose description is: %q.\n"+
		"Output only the Markdown instructions an AI coding assistant should follow: no YAML frontmatter, no code fences around the whole answer, no preamble.",
		name, description)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	for _, d := range drafters {
		if d.name != cli {
			continue
		}
		cmd := exec.CommandContext(ctx, cli, d.args(prompt)...)
		cmd.Dir = os.TempDir() // no project context leaks into the draft
		out, err := cmd.Output()
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s took longer than 2 minutes", cli)
		}
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return "", fmt.Errorf("%s: %s", cli, strings.TrimSpace(string(ee.Stderr)))
			}
			return "", err
		}
		return stripFrontmatter(strings.TrimSpace(string(out))), nil
	}
	return "", nil
}

func stripFrontmatter(s string) string {
	if rest, ok := strings.CutPrefix(s, "---\n"); ok {
		if _, body, ok := strings.Cut(rest, "\n---"); ok {
			return strings.TrimSpace(body)
		}
	}
	return s
}
