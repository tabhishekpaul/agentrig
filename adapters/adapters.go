// Package adapters reads (and later writes) each AI tool's native config.
package adapters

import (
	"bufio"
	"fmt"
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

// frontmatter pulls name/description from a SKILL.md header.
// ponytail: single-line values only; swap in a YAML parser if folded (>) descriptions show up.
func frontmatter(path string) (name, desc string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() || strings.TrimSpace(s.Text()) != "---" {
		return
	}
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			switch k {
			case "name":
				name = v
			case "description":
				desc = v
			}
		}
	}
	return
}
