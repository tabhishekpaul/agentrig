package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// claudePaths is overridable so tests never touch the real ~/.claude.
type claudePaths struct {
	Root     string // ~/.claude
	MCPFile  string // ~/.claude.json
	Settings string // ~/.claude/settings.json
}

func defaultClaudePaths() claudePaths {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".claude")
	return claudePaths{Root: root, MCPFile: filepath.Join(home, ".claude.json"), Settings: filepath.Join(root, "settings.json")}
}

// Disabled skills live in a sibling of skills/ at the same depth, so the
// relative symlinks some skill installers create (../../.agents/…) still resolve.
func (p claudePaths) skills() string   { return filepath.Join(p.Root, "skills") }
func (p claudePaths) disabled() string { return filepath.Join(p.Root, "skills-disabled") }

func claude() Tool { return claudeAt(defaultClaudePaths()) }

func claudeAt(p claudePaths) Tool {
	t := Tool{ID: "claude", Name: "Claude Code", Items: []Item{}}
	if _, err := os.Stat(p.Root); err != nil {
		return t
	}
	t.Installed = true
	t.Items = append(t.Items, withSource("~/.claude/skills", readSkills(p.skills(), true))...)
	t.Items = append(t.Items, withSource("~/.claude/skills", readSkills(p.disabled(), false))...)
	sortByName(t.Items)
	t.Items = append(t.Items, withSource("~/.claude/settings.json", claudePlugins(filepath.Join(p.Root, "plugins", "installed_plugins.json"), p.Settings))...)
	t.Items = append(t.Items, withSource("~/.claude.json", claudeMCP(p.MCPFile))...)
	return t
}

func withSource(src string, items []Item) []Item {
	for i := range items {
		items[i].Source = src
	}
	return items
}

func readSkills(dir string, enabled bool) []Item {
	entries, _ := os.ReadDir(dir)
	var items []Item
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue // .DS_Store, Codex's bundled .system skills
		}
		md := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(md); err != nil {
			continue // not a skill (e.g. .DS_Store, stray dirs)
		}
		name, desc := frontmatter(md)
		if name == "" {
			name = e.Name()
		}
		items = append(items, Item{Kind: "skill", ID: e.Name(), Name: name, Description: desc,
			Path: filepath.Dir(md), Enabled: enabled, Toggleable: true})
	}
	return items
}

func claudePlugins(installed, settings string) []Item {
	var f struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
		} `json:"plugins"`
	}
	b, err := os.ReadFile(installed)
	if err != nil || json.Unmarshal(b, &f) != nil {
		return nil
	}
	var s struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if b, err := os.ReadFile(settings); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	var items []Item
	for id, installs := range f.Plugins {
		it := Item{Kind: "plugin", ID: id, Name: id, Enabled: s.EnabledPlugins[id], Toggleable: true}
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
		items = append(items, Item{Kind: "mcp", ID: name, Name: name, Enabled: true})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func claudeToggle(p claudePaths, kind, id string, enabled bool) error {
	switch kind {
	case "skill":
		return toggleSkillIn(p.skills(), p.disabled(), id, enabled)
	case "plugin":
		return togglePlugin(p.Settings, id, enabled)
	}
	return errNotToggleable("Claude Code", kind)
}

// toggleSkillIn moves a skill between dir and its sibling disabled dir.
// Siblings sit at the same depth, so relative symlinks (../../.agents/…) still resolve.
func toggleSkillIn(dir, disabledDir, id string, enabled bool) error {
	// id comes from the browser: it must be a plain directory name, never a path.
	if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") {
		return fmt.Errorf("invalid skill id %q", id)
	}
	from, to := filepath.Join(disabledDir, id), filepath.Join(dir, id)
	if !enabled {
		from, to = to, from
	}
	if _, err := os.Lstat(from); err != nil {
		if _, err := os.Lstat(to); err == nil {
			return nil // already in the requested state
		}
		return errNotFound("skill", id)
	}
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("skill %q exists in both enabled and disabled folders; resolve by hand", id)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to) // moves symlinks as links, never their targets
}

// togglePlugin flips enabledPlugins[id] in settings.json. Other top-level keys
// are kept byte-for-byte (RawMessage); only their order becomes alphabetical.
func togglePlugin(settings, id string, enabled bool) error {
	top := map[string]json.RawMessage{}
	b, err := os.ReadFile(settings)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &top); err != nil {
			return fmt.Errorf("settings.json is not valid JSON, not touching it: %w", err)
		}
	}
	plugins := map[string]bool{}
	if raw, ok := top["enabledPlugins"]; ok {
		if err := json.Unmarshal(raw, &plugins); err != nil {
			return fmt.Errorf("unexpected enabledPlugins shape: %w", err)
		}
	}
	plugins[id] = enabled
	top["enabledPlugins"], _ = json.Marshal(plugins)
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(settings, append(out, '\n'))
}

// writeAtomic never leaves a half-written config if we crash mid-write.
func writeAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agentrig-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
