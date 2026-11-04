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
	return []Tool{claude()}
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
