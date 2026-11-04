package adapters

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Codex keeps MCP servers and plugins in config.toml:
//
//	[mcp_servers.node_repl]
//	[plugins."browser@openai-bundled"]
//	enabled = true
//
// ponytail: a line scanner, not a TOML parser. It handles the table headers
// Codex writes; switch to a real parser if inline tables show up.
var tomlHeader = regexp.MustCompile(`^\s*\[\s*(mcp_servers|plugins)\.(?:"([^"]+)"|([A-Za-z0-9_-]+))\s*\]\s*(#.*)?$`)
var tomlEnabled = regexp.MustCompile(`^(\s*enabled\s*=\s*)(true|false)(.*)$`)

type tomlEntry struct {
	kind, name string
	header     int // line index of the [table] header
	enabledAt  int // line index of `enabled = …`, -1 if absent
	enabled    bool
}

func scanCodexTOML(lines []string) []tomlEntry {
	var out []tomlEntry
	cur := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			cur = -1
			if m := tomlHeader.FindStringSubmatch(l); m != nil {
				name := m[2] + m[3]
				out = append(out, tomlEntry{kind: m[1], name: name, header: i, enabledAt: -1, enabled: true})
				cur = len(out) - 1
			}
			continue
		}
		if cur >= 0 {
			if m := tomlEnabled.FindStringSubmatch(l); m != nil {
				out[cur].enabledAt = i
				out[cur].enabled = m[2] == "true"
			}
		}
	}
	return out
}

func readLines(file string) ([]string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

func codexItems(file string) (plugins, mcp []Item) {
	lines, err := readLines(file)
	if err != nil {
		return nil, nil
	}
	for _, e := range scanCodexTOML(lines) {
		switch e.kind {
		case "plugins":
			plugins = append(plugins, Item{Kind: "plugin", ID: e.name, Name: e.name, Enabled: e.enabled, Toggleable: true})
		case "mcp_servers":
			mcp = append(mcp, Item{Kind: "mcp", ID: e.name, Name: e.name, Enabled: true})
		}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins, mcp
}

// toggleCodexPlugin rewrites only the plugin's `enabled` line; every other
// byte of config.toml (comments, order, spacing) stays as it was.
func toggleCodexPlugin(file, id string, enabled bool) error {
	lines, err := readLines(file)
	if err != nil {
		return err
	}
	val := fmt.Sprint(enabled)
	for _, e := range scanCodexTOML(lines) {
		if e.kind != "plugins" || e.name != id {
			continue
		}
		if e.enabledAt >= 0 {
			m := tomlEnabled.FindStringSubmatch(lines[e.enabledAt])
			lines[e.enabledAt] = m[1] + val + m[3]
		} else {
			lines = append(lines[:e.header+1], append([]string{"enabled = " + val}, lines[e.header+1:]...)...)
		}
		return writeAtomic(file, []byte(strings.Join(lines, "\n")))
	}
	return fmt.Errorf("plugin %q not found in %s", id, file)
}
