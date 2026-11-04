// Package gitsync keeps AI tool setups in step across machines through a
// git repo the user owns. The repo mirrors the home layout:
//
//	home/.arig/library/<name>/<hash>/…     the central skill library (entries never change)
//	home/.claude/skills/<name>/…           skills (links stay links)
//	home/.claude/skills-disabled/<name>/…  switched-off skills
//	state.json                             plugin on/off per tool
//
// MCP configs are never synced: they hold API keys.
package gitsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LibraryDir is the central skill library (package library), relative to
// home. It's synced first so the relative links into it never dangle, and
// only its entry folders travel: its top-level files (migration logs and
// marker) stay on each machine.
const LibraryDir = ".arig/library"

// Dirs are the skill folders kept in sync, relative to home.
var Dirs = func() []string {
	base := []string{".claude/skills", ".agents/skills", ".codex/skills", ".cursor/skills", ".gemini/skills",
		".gemini/antigravity/skills", ".copilot/skills", ".factory/skills", ".config/opencode/skill", ".config/opencode/skills"}
	all := []string{LibraryDir}
	for _, d := range base {
		all = append(all, d, d+"-disabled")
	}
	return all
}()

// State lets the caller sync settings that live inside config files
// (plugin on/off). Export returns tool -> id -> enabled; Apply sets them.
type State struct {
	Export func() map[string]map[string]bool
	Apply  func(map[string]map[string]bool) error
}

type Config struct {
	Repo       string    `json:"repo"`
	Machine    string    `json:"machine"`
	LastSync   time.Time `json:"lastSync"`
	LastResult string    `json:"lastResult,omitempty"`
	LastError  string    `json:"lastError,omitempty"`
}

type Syncer struct {
	Home  string // the user's home
	Dir   string // agentrig's own folder: config.json, repo/, backup/
	State State
	mu    sync.Mutex

	phase      atomic.Pointer[string] // what a running sync is doing, for the UI
	phaseStart time.Time
}

// Phase is what the sync in progress is doing right now ("" when idle).
func (s *Syncer) Phase() string {
	if p := s.phase.Load(); p != nil {
		return *p
	}
	return ""
}

// step records progress for the UI and logs how long the previous step took.
func (s *Syncer) step(what string) {
	if prev := s.Phase(); prev != "" {
		log.Printf("sync: %s (%s)", prev, time.Since(s.phaseStart).Round(time.Millisecond))
	}
	s.phase.Store(&what)
	s.phaseStart = time.Now()
}

func (s *Syncer) repo() string       { return filepath.Join(s.Dir, "repo") }
func (s *Syncer) configPath() string { return filepath.Join(s.Dir, "config.json") }

// Status returns the saved config (Repo empty = not connected).
func (s *Syncer) Status() Config {
	var c Config
	b, err := os.ReadFile(s.configPath())
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func (s *Syncer) save(c Config) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(s.configPath(), b, 0o600)
}

// Connect clones the user's repo (empty is fine) and runs a first sync.
// The first sync only adds: this machine's skills are committed before
// anything is pulled, so joining never deletes local work.
func (s *Syncer) Connect(repo string) (Config, error) { return s.connect(repo, false) }

// connect with fresh set skips the download: a repo arig just created is
// empty, so a local repo pointed at it is the same thing, a network round
// trip sooner.
func (s *Syncer) connect(repo string, fresh bool) (Config, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" || strings.HasPrefix(repo, "-") { // never let input become a git flag
		return Config{}, errors.New("enter a git repo URL, e.g. git@github.com:you/ai-setup.git")
	}
	s.mu.Lock()
	os.RemoveAll(s.repo())
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		s.mu.Unlock()
		return Config{}, err
	}
	var err error
	if fresh {
		if _, err = git(s.Dir, "init", "-q", "repo"); err == nil {
			_, err = git(s.repo(), "remote", "add", "origin", "--", repo)
		}
	} else {
		s.step("Downloading the repo…")
		_, err = git(s.Dir, "clone", "--", repo, "repo")
	}
	s.mu.Unlock()
	if err != nil {
		s.step("")
		return Config{}, fmt.Errorf("couldn't clone %s: %w", Redact(repo), err)
	}
	host, _ := os.Hostname()
	if err := s.save(Config{Repo: repo, Machine: host}); err != nil {
		return Config{}, err
	}
	return s.Run()
}

// Disconnect forgets the repo locally. Nothing on the remote or in the
// tools' folders changes.
func (s *Syncer) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.Remove(s.configPath())
	return os.RemoveAll(s.repo())
}

// Run does one full sync: commit local → merge remote (local wins on
// conflicts) → apply merged result locally (backing up what it replaces) → push.
func (s *Syncer) Run() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.Status()
	if c.Repo == "" {
		return c, errors.New("sync isn't set up")
	}
	result, err := s.run(c)
	s.step("")
	c.LastSync = time.Now()
	c.LastResult, c.LastError = result, ""
	if err != nil {
		c.LastError = err.Error()
	}
	if serr := s.save(c); err == nil {
		err = serr
	}
	return c, err
}

func (s *Syncer) run(c Config) (string, error) {
	r := s.repo()
	if _, err := git(r, "rev-parse", "--verify", "HEAD"); err != nil {
		git(r, "symbolic-ref", "HEAD", "refs/heads/main") // fresh empty repo: use main
	}
	branch, err := git(r, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}

	// 1. this machine → repo. The repo's tree is what this machine had at its
	// last sync, so a path missing locally was deleted here — except on the
	// first sync, which only adds (joining must never delete other machines' work).
	first := c.LastSync.IsZero()
	s.step("Collecting this computer's skills…")
	up := 0
	for _, d := range Dirs {
		src := filepath.Join(s.Home, d)
		if _, err := os.Stat(src); err != nil {
			continue // tool not on this machine: not a deletion
		}
		n, err := mirror(src, filepath.Join(r, "home", d), "", first, d == LibraryDir)
		if err != nil {
			return "", err
		}
		up += n
	}
	if s.State.Export != nil {
		if err := writeJSON(filepath.Join(r, "state.json"), mergeState(readState(r), s.State.Export())); err != nil {
			return "", err
		}
	}
	s.step(fmt.Sprintf("Saving %d changes…", up))
	if err := commit(r, "arig: "+c.Machine); err != nil {
		return "", err
	}

	// 2. merge what other machines pushed
	s.step("Getting changes from your other computers…")
	// One network round trip (fetch), then merge locally. A fresh, empty
	// remote has no branch yet: the fetch succeeds and there's nothing to merge.
	if _, err := git(r, "fetch", "origin"); err != nil {
		return "", err
	}
	remote := "origin/" + branch
	hasRemote := false
	if _, err := git(r, "rev-parse", "--verify", "--quiet", remote); err == nil {
		hasRemote = true
		if _, err := git(r, "merge", "--no-edit", "-X", "ours", remote); err != nil {
			if err := resolveOurs(r, c.Machine, err); err != nil {
				return "", fmt.Errorf("merge failed: %w", err)
			}
		}
	}

	// 3. repo → this machine, keeping a copy of anything replaced
	s.step("Applying changes on this computer…")
	backup := filepath.Join(s.Dir, "backup", time.Now().Format("2006-01-02T15-04-05"))
	down := 0
	for _, d := range Dirs {
		src := filepath.Join(r, "home", d)
		dst := filepath.Join(s.Home, d)
		if _, err := os.Stat(filepath.Dir(dst)); err != nil && !strings.HasPrefix(d, ".agents/") && d != LibraryDir {
			continue // tool not on this machine (e.g. no ~/.cursor): don't create it.
			// ~/.agents and the library are shared by all tools (others link into them), so they're always created.
		}
		if _, err := os.Stat(src); err != nil {
			if _, err := os.Stat(dst); err != nil {
				continue // neither side has this folder
			}
		}
		n, err := mirror(src, dst, filepath.Join(backup, d), false, d == LibraryDir)
		if err != nil {
			return "", err
		}
		down += n
	}
	if s.State.Apply != nil {
		if err := s.State.Apply(readState(r)); err != nil {
			return "", err
		}
	}

	// 4. publish, unless the remote already has everything (saves a round trip)
	if hasRemote {
		if ahead, _ := git(r, "rev-list", "--count", remote+"..HEAD"); ahead == "0" {
			return fmt.Sprintf("%d sent, %d received", up, down), nil
		}
	}
	s.step(fmt.Sprintf("Uploading %d changes…", up))
	if _, err := git(r, "push", "-u", "origin", branch); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d sent, %d received", up, down), nil
}

func commit(r, msg string) error {
	if _, err := git(r, "add", "-A"); err != nil {
		return err
	}
	if out, _ := git(r, "status", "--porcelain"); out == "" {
		return nil
	}
	_, err := git(r, "commit", "-q", "-m", msg)
	return err
}

// resolveOurs settles conflicts -X ours can't (edit vs delete) by keeping
// this machine's side.
func resolveOurs(r, machine string, pullErr error) error {
	out, err := git(r, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return err
	}
	if out == "" {
		return pullErr // not a conflict: report what actually went wrong
	}
	for _, f := range strings.Split(out, "\n") {
		if f == "" {
			continue
		}
		if _, err := git(r, "checkout", "--ours", "--", f); err != nil {
			if _, err := git(r, "rm", "-q", "--", f); err != nil {
				return err
			}
		}
	}
	// Always commit: keeping our side can leave no diff, but the merge must still be concluded.
	if _, err := git(r, "add", "-A"); err != nil {
		return err
	}
	_, err = git(r, "commit", "-q", "--no-edit", "-m", "arig: merge on "+machine)
	return err
}

func readState(r string) map[string]map[string]bool {
	m := map[string]map[string]bool{}
	if b, err := os.ReadFile(filepath.Join(r, "state.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// mergeState overlays this machine's values on what the repo already holds,
// so plugins installed only elsewhere keep their entries.
func mergeState(base, local map[string]map[string]bool) map[string]map[string]bool {
	for tool, ps := range local {
		if base[tool] == nil {
			base[tool] = map[string]bool{}
		}
		for id, on := range ps {
			base[tool][id] = on
		}
	}
	return base
}

func writeJSON(path string, v any) error {
	b, _ := json.MarshalIndent(v, "", "  ") // sorted keys: one per line, clean merges
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// git runs git without ever prompting: in the background there's no one to
// type a password, so auth must come from the user's existing setup.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes",
		"GIT_AUTHOR_NAME=arig", "GIT_AUTHOR_EMAIL=arig@localhost",
		"GIT_COMMITTER_NAME=arig", "GIT_COMMITTER_EMAIL=arig@localhost",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Redact hides credentials embedded in https URLs (https://token@github.com/…).
func Redact(repo string) string {
	if u, err := url.Parse(repo); err == nil && u.User != nil {
		u.User = url.User("***")
		return u.String()
	}
	return repo
}

// Loop syncs every interval while connected, until ctx ends.
func (s *Syncer) Loop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if s.Status().Repo != "" {
			s.Run()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
