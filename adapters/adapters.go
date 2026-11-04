// Package adapters reads (and later writes) each AI tool's native config.
package adapters

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

type Item struct {
	Kind        string `json:"kind"` // skill | plugin | mcp
	ID          string `json:"id"`   // stable key used by Toggle
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	Source      string `json:"source,omitempty"` // config file or folder it came from, as ~/… for display
	Enabled     bool   `json:"enabled"`
	Toggleable  bool   `json:"toggleable"`
}

var mu sync.Mutex // one config write at a time

// Toggle enables or disables one item in one tool's native config.
func Toggle(tool, kind, id string, enabled bool) error {
	mu.Lock()
	defer mu.Unlock()
	switch tool {
	case "claude":
		return claudeToggle(defaultClaudePaths(), kind, id, enabled)
	}
	for _, s := range specs {
		if s.ID == tool {
			return s.toggle(realEnv(), kind, id, enabled)
		}
	}
	return fmt.Errorf("unknown tool %q", tool)
}

type Tool struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Items     []Item `json:"items"`
}

// Detect reports every supported tool, installed or not.
func Detect() []Tool {
	e := realEnv()
	tools := []Tool{claude()}
	for _, s := range specs {
		tools = append(tools, s.read(e))
	}
	return tools
}

func errNotFound(kind, id string) error { return fmt.Errorf("%s %q not found", kind, id) }
func errNotToggleable(tool, kind string) error {
	return fmt.Errorf("%s: %s items can't be toggled yet", tool, kind)
}

// Frontmatter returns the single-line key: value pairs of a SKILL.md header
// (name, description, category, tags, …).
// ponytail: single-line values only; swap in a YAML parser if folded (>) descriptions show up.
func Frontmatter(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return map[string]string{}
	}
	defer f.Close()
	return ParseFrontmatter(f)
}

// ParseFrontmatter is Frontmatter for SKILL.md text that isn't on disk.
func ParseFrontmatter(r io.Reader) map[string]string {
	out := map[string]string{}
	s := bufio.NewScanner(r)
	if !s.Scan() || strings.TrimSpace(s.Text()) != "---" {
		return out
	}
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && !strings.HasPrefix(k, " ") {
			out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return out
}

// PluginStates returns tool -> plugin id -> enabled for every plugin that can
// be switched, for syncing between machines.
func PluginStates() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, t := range Detect() {
		for _, i := range t.Items {
			if i.Kind == "plugin" && i.Toggleable {
				if out[t.ID] == nil {
					out[t.ID] = map[string]bool{}
				}
				out[t.ID][i.ID] = i.Enabled
			}
		}
	}
	return out
}

// ApplyPluginStates switches plugins that are installed here to match want.
// Plugins not installed on this machine are left alone.
func ApplyPluginStates(want map[string]map[string]bool) error {
	have := PluginStates()
	for tool, ps := range want {
		for id, on := range ps {
			if cur, ok := have[tool][id]; ok && cur != on {
				if err := Toggle(tool, "plugin", id, on); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
