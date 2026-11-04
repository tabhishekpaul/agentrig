// Package starter is the curated first-launch pack: one strong skill per
// category plus a few specials, and the installer that puts the ones the
// user picks into their tools.
package starter

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tabhishekpaul/agentrig/internal/proc"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tabhishekpaul/agentrig/market"
)

//go:embed starter.json
var raw []byte

// Entry is one starter pick.
type Entry struct {
	ID       string   `json:"id"`   // skills.sh owner/repo/skill, or a plugin's own id
	Kind     string   `json:"kind"` // skill | plugin
	Category string   `json:"category"`
	Name     string   `json:"name"`
	Why      string   `json:"why"`
	Installs int      `json:"installs,omitempty"`
	Stars    int      `json:"stars,omitempty"`
	Flags    []string `json:"flags"`   // safety flags of the reviewed version
	Default  bool     `json:"default"` // pre-checked
	Needs    []string `json:"needs,omitempty"`
	Setup    string   `json:"setup,omitempty"`
	Repo     string   `json:"repo,omitempty"`
	// Plugins: the id each tool lists it under, and the CLI commands that
	// install it, per tool. Compiled in, never taken from a request.
	Plugin  string                `json:"plugin,omitempty"`
	Install map[string][][]string `json:"install,omitempty"`
}

var entries = sync.OnceValue(func() []Entry {
	var es []Entry
	if err := json.Unmarshal(raw, &es); err != nil {
		panic("starter.json: " + err.Error())
	}
	return es
})

// Entries returns the starter list.
func Entries() []Entry { return entries() }

func byID(id string) (Entry, bool) {
	i := slices.IndexFunc(entries(), func(e Entry) bool { return e.ID == id })
	if i < 0 {
		return Entry{}, false
	}
	return entries()[i], true
}

// Marker is written once the user has seen the welcome page.
func Marker() string { home, _ := os.UserHomeDir(); return filepath.Join(home, ".arig", "onboarded") }

func Onboarded() bool { _, err := os.Stat(Marker()); return err == nil }

func MarkOnboarded() error {
	if err := os.MkdirAll(filepath.Dir(Marker()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(Marker(), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Row is one entry's install progress.
type Row struct {
	State  string `json:"state"` // queued | installing | done | failed
	Reason string `json:"reason,omitempty"`
}

// Installer installs starter entries one at a time in the background.
type Installer struct {
	Market          *market.Market
	SkillTools      func() []string                     // installed tools with a skills folder
	PluginInstalled func(tool, plugin string) bool      // already in that tool
	LookPath        func(string) (string, error)        // exec.LookPath
	Run             func(argv []string) (string, error) // combined output

	mu      sync.Mutex
	rows    map[string]Row
	running bool
}

// New wires an Installer to the real system.
func New(m *market.Market, skillTools func() []string, pluginInstalled func(tool, plugin string) bool) *Installer {
	return &Installer{Market: m, SkillTools: skillTools, PluginInstalled: pluginInstalled, LookPath: exec.LookPath, Run: run}
}

func run(argv []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := proc.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput() // stdin is /dev/null: a prompt can't hang it
	return string(out), err
}

// Start queues ids and installs them in the background. A flagged skill
// installs only if its id is in accept.
func (in *Installer) Start(ids, accept []string) error {
	var todo []Entry
	for _, id := range ids {
		e, ok := byID(id)
		if !ok {
			return fmt.Errorf("%q isn't in the starter pack", id)
		}
		if !slices.ContainsFunc(todo, func(t Entry) bool { return t.ID == id }) {
			todo = append(todo, e)
		}
	}
	if len(todo) == 0 {
		return errors.New("pick at least one")
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.running {
		return errors.New("an install is already running")
	}
	in.running, in.rows = true, map[string]Row{}
	for _, e := range todo {
		in.rows[e.ID] = Row{State: "queued"}
	}
	go func() {
		for _, e := range todo {
			in.set(e.ID, Row{State: "installing"})
			note, err := in.install(e, slices.Contains(accept, e.ID))
			if err != nil {
				in.set(e.ID, Row{"failed", err.Error()})
			} else {
				in.set(e.ID, Row{"done", note})
			}
		}
		in.mu.Lock()
		in.running = false
		in.mu.Unlock()
	}()
	return nil
}

func (in *Installer) set(id string, r Row) {
	in.mu.Lock()
	in.rows[id] = r
	in.mu.Unlock()
}

// Progress returns every queued entry's state and whether the run goes on.
func (in *Installer) Progress() (map[string]Row, bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make(map[string]Row, len(in.rows))
	for k, v := range in.rows {
		out[k] = v
	}
	return out, in.running
}

func (in *Installer) install(e Entry, accept bool) (string, error) {
	if e.Kind == "plugin" {
		return in.plugin(e)
	}
	return in.skill(e, accept)
}

// Missing lists which of tools don't have this skill yet.
func (in *Installer) Missing(id string, tools []string) []string {
	_, _, name, err := market.SplitID(id)
	if err != nil {
		return nil
	}
	var out []string
	for _, t := range tools {
		d := in.Market.SkillDir(t)
		if !exists(filepath.Join(d, name)) && !exists(filepath.Join(d+"-disabled", name)) {
			out = append(out, t)
		}
	}
	return out
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func (in *Installer) skill(e Entry, accept bool) (string, error) {
	if _, ok := in.Market.Lock()[e.ID]; ok {
		return "already installed", nil
	}
	all := in.SkillTools()
	if len(all) == 0 {
		return "", errors.New("none of your tools has a skills folder")
	}
	tools := in.Missing(e.ID, all)
	if len(tools) == 0 {
		return "already in your tools", nil
	}
	dl, err := in.Market.Get(e.ID)
	if err != nil {
		return "", err
	}
	flags := market.Flags(dl.Files)
	// A flag the starter list didn't review means the skill changed: send the
	// user to the Marketplace to look at it rather than accepting blind.
	if extra := slices.DeleteFunc(slices.Clone(flags), func(f string) bool { return slices.Contains(e.Flags, f) }); len(extra) > 0 {
		return "", fmt.Errorf("it changed since the starter list was made (now: %s); review it in the Marketplace", strings.Join(extra, ", "))
	}
	if len(flags) > 0 && !accept {
		return "", fmt.Errorf("not installed: you didn't accept its flags (%s)", strings.Join(flags, ", "))
	}
	return "into " + strings.Join(tools, ", "), in.Market.Install(e.ID, tools, accept)
}

// PluginTools lists the tools a plugin can go into here (their CLI is on PATH).
func (in *Installer) PluginTools(e Entry) []string {
	var out []string
	for t, cmds := range e.Install {
		if len(cmds) > 0 && len(cmds[0]) > 0 {
			if _, err := in.LookPath(cmds[0][0]); err == nil {
				out = append(out, t)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (in *Installer) plugin(e Entry) (string, error) {
	tools := in.PluginTools(e)
	if len(tools) == 0 {
		return "", errors.New("needs the Claude Code or Codex command line, and neither is installed")
	}
	var done []string
	var errs []error
	for _, t := range tools {
		if in.PluginInstalled(t, e.Plugin) {
			done = append(done, t)
			continue
		}
		if err := in.commands(e.Install[t]); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t, err))
			continue
		}
		done = append(done, t)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	return "into " + strings.Join(done, ", ") + "; start a new session to use it", nil
}

// commands runs one tool's install steps. A setup step (adding the
// marketplace) that fails because it's already there is fine.
func (in *Installer) commands(cmds [][]string) error {
	for i, c := range cmds {
		bin, err := in.LookPath(c[0])
		if err != nil {
			return err
		}
		out, err := in.Run(append([]string{bin}, c[1:]...))
		if err == nil || (i < len(cmds)-1 && strings.Contains(strings.ToLower(out), "already")) {
			continue
		}
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return fmt.Errorf("%s: %s", strings.Join(c, " "), msg)
	}
	return nil
}
