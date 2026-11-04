package authoring

import (
	"context"
	"errors"
	"fmt"
	"github.com/tabhishekpaul/agentrig/internal/proc"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/catalog"
)

// GH is the GitHub CLI binary; tests point PATH at a stub.
var GH = "gh"

// Topics the marketplace crawler searches for.
const (
	TopicSkill = "arig-skill"
	TopicPack  = "arig-pack"
)

// Item is one thing to publish: a skill folder or a pack.
type Item struct {
	Kind string // "skill" | "pack"
	ID   string
	// skill: Path is its installed folder, Description from SKILL.md.
	Path        string
	Description string
	// pack: the pack and how to find its skills.
	Pack      catalog.Pack
	SkillPath func(string) string
}

// Plan is exactly what Publish will do, shown to the user before confirming.
type Plan struct {
	Repo        string   `json:"repo"` // owner/name
	Description string   `json:"description"`
	Topics      []string `json:"topics"`
	Files       []string `json:"files"`
	Author      string   `json:"author"` // git identity the commit uses
	Steps       []string `json:"steps"`
}

// Prepare stages it in a temp dir and returns the plan plus that dir (the
// caller removes it).
func Prepare(it Item) (Plan, string, error) {
	if !ValidID(it.ID) {
		return Plan{}, "", fmt.Errorf("%q can't be a repo name: use lowercase kebab-case", it.ID)
	}
	user, err := run("", 30*time.Second, GH, "api", "user", "--jq", ".login")
	if err != nil || user == "" {
		return Plan{}, "", fmt.Errorf("GitHub CLI isn't signed in: run `gh auth login` in a terminal (%v)", err)
	}
	author := GitUser()
	email, _ := run("", 5*time.Second, "git", "config", "user.email")
	if author == "" || email == "" {
		return Plan{}, "", errors.New("set your git identity first: git config --global user.name / user.email")
	}
	tmp, err := os.MkdirTemp("", "arig-publish-")
	if err != nil {
		return Plan{}, "", err
	}
	p := Plan{Repo: user + "/" + it.ID, Topics: []string{TopicSkill}, Author: author + " <" + email + ">"}
	switch it.Kind {
	case "skill":
		p.Description = it.Description
		err = CopySkill(it.Path, tmp)
	case "pack":
		p.Description = it.Pack.Description
		p.Topics = append(p.Topics, TopicPack)
		it.Pack.Repo = p.Repo
		err = WritePlugin(tmp, it.Pack, author, it.SkillPath)
	default:
		err = fmt.Errorf("unknown kind %q", it.Kind)
	}
	if err != nil {
		os.RemoveAll(tmp)
		return Plan{}, "", err
	}
	if r := []rune(p.Description); len(r) > 350 {
		p.Description = string(r[:350])
	}
	_ = filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(tmp, path)
			p.Files = append(p.Files, filepath.ToSlash(rel))
		}
		return nil
	})
	p.Steps = []string{
		fmt.Sprintf("Create the public GitHub repo %s", p.Repo),
		fmt.Sprintf("Commit %d files as %s and push them", len(p.Files), p.Author),
		fmt.Sprintf("Add the topic%s %s so the arig marketplace finds it", map[bool]string{true: "s"}[len(p.Topics) > 1], strings.Join(p.Topics, ", ")),
	}
	return p, tmp, nil
}

// Publish runs the plan for the files staged in dir. It is never retried
// automatically: a half-done publish leaves a repo the user can see and fix.
func Publish(p Plan, dir string) (string, error) {
	_, name, _ := strings.Cut(p.Repo, "/")
	steps := [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "add", "-A"},
		{"git", "commit", "-q", "-m", "Publish " + name + " with arig"},
		{GH, "repo", "create", name, "--public", "--description", p.Description, "--source", ".", "--remote", "origin", "--push"},
	}
	edit := []string{GH, "repo", "edit", p.Repo}
	for _, t := range p.Topics {
		edit = append(edit, "--add-topic", t)
	}
	steps = append(steps, edit)
	for _, s := range steps {
		if _, err := run(dir, 2*time.Minute, s[0], s[1:]...); err != nil {
			return "", err
		}
	}
	return "https://github.com/" + p.Repo, nil
}

// GitUser is the user's git user.name ("" if unset).
func GitUser() string {
	s, _ := run("", 5*time.Second, "git", "config", "user.name")
	return s
}

// run is gitsync's cli() pattern: timeout, no prompts, output in the error.
func run(dir string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %s", filepath.Base(name), args[0], strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
