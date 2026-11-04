package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

func claude() Tool {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".claude")
	t := Tool{ID: "claude", Name: "Claude Code", Items: []Item{}}
	if _, err := os.Stat(root); err != nil {
		return t
	}
	t.Installed = true
	t.Items = append(t.Items, claudeSkills(filepath.Join(root, "skills"))...)
	t.Items = append(t.Items, claudePlugins(filepath.Join(root, "plugins", "installed_plugins.json"))...)
	t.Items = append(t.Items, claudeMCP(filepath.Join(home, ".claude.json"))...)
	return t
}

func claudeSkills(dir string) []Item {
	entries, _ := os.ReadDir(dir)
	var items []Item
	for _, e := range entries {
		md := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(md); err != nil {
			continue // not a skill (e.g. .DS_Store, stray dirs)
		}
		name, desc := frontmatter(md)
		if name == "" {
			name = e.Name()
		}
		items = append(items, Item{Kind: "skill", Name: name, Description: desc, Path: filepath.Dir(md)})
	}
	return items
}

func claudePlugins(path string) []Item {
	var f struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
		} `json:"plugins"`
	}
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &f) != nil {
		return nil
	}
	var items []Item
	for id, installs := range f.Plugins {
		it := Item{Kind: "plugin", Name: id}
		if len(installs) > 0 {
			it.Description = "v" + installs[0].Version
			it.Path = installs[0].InstallPath
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

// claudeMCP lists user-scoped MCP servers. Only names leave this function:
// the config holds env vars and API keys that must never reach the UI.
func claudeMCP(path string) []Item {
	var f struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &f) != nil {
		return nil
	}
	var items []Item
	for name := range f.MCPServers {
		items = append(items, Item{Kind: "mcp", Name: name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}
