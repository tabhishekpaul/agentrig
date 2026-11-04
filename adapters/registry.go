package adapters

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// env resolves the two path roots every tool uses: "~/" (home) and "$CFG/"
// (os.UserConfigDir: ~/Library/Application Support, ~/.config or %AppData%).
type env struct{ home, cfg string }

func realEnv() env {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	return env{home, cfg}
}

func (e env) path(p string) string {
	switch {
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(e.home, p[2:])
	case strings.HasPrefix(p, "$CFG/"):
		return filepath.Join(e.cfg, p[5:])
	}
	return p
}

type mcpSource struct {
	File string
	Key  []string // object holding the servers, e.g. ["mcpServers"]
}

// spec describes one tool declaratively; adding a tool is adding an entry.
type spec struct {
	ID, Name   string
	Detect     []string    // installed if any of these exists
	Skills     []string    // Agent Skills dirs (<dir>/<name>/SKILL.md); toggled via <dir>-disabled
	MCP        []mcpSource // JSON / JSONC files listing MCP servers
	CodexTOML  string      // Codex config.toml: plugins (toggleable) + MCP servers
	Extensions string      // dir of Gemini-style extensions (<dir>/<name>/gemini-extension.json)
}

const vscodeStorage = "$CFG/Code/User/globalStorage/"

var mcpServers = []string{"mcpServers"}

// Claude Code is handled by claude.go (plugins live in its settings.json).
var specs = []spec{
	{ID: "codex", Name: "Codex (ChatGPT)", Detect: []string{"~/.codex"},
		Skills: []string{"~/.codex/skills", "~/.agents/skills"}, CodexTOML: "~/.codex/config.toml"},
	{ID: "cursor", Name: "Cursor", Detect: []string{"~/.cursor", "$CFG/Cursor"},
		Skills: []string{"~/.cursor/skills"}, MCP: []mcpSource{{"~/.cursor/mcp.json", mcpServers}}},
	{ID: "gemini", Name: "Gemini CLI", Detect: []string{"~/.gemini/settings.json", "~/.gemini/extensions", "~/.gemini/skills"},
		Skills: []string{"~/.gemini/skills"}, MCP: []mcpSource{{"~/.gemini/settings.json", mcpServers}}, Extensions: "~/.gemini/extensions"},
	{ID: "antigravity", Name: "Antigravity", Detect: []string{"$CFG/Antigravity", "~/.gemini/antigravity", "~/.gemini/antigravity-cli"},
		Skills: []string{"~/.gemini/antigravity/skills"},
		MCP:    []mcpSource{{"~/.gemini/config/mcp_config.json", mcpServers}, {"~/.gemini/antigravity/mcp_config.json", mcpServers}}},
	{ID: "copilot-cli", Name: "GitHub Copilot CLI", Detect: []string{"~/.copilot"},
		Skills: []string{"~/.copilot/skills"}, MCP: []mcpSource{{"~/.copilot/mcp-config.json", mcpServers}}},
	{ID: "vscode", Name: "VS Code (Copilot)", Detect: []string{"$CFG/Code/User"},
		MCP: []mcpSource{{"$CFG/Code/User/mcp.json", []string{"servers"}}}},
	{ID: "windsurf", Name: "Windsurf", Detect: []string{"~/.codeium/windsurf", "$CFG/Windsurf"},
		MCP: []mcpSource{{"~/.codeium/windsurf/mcp_config.json", mcpServers}}},
	{ID: "claude-desktop", Name: "Claude Desktop", Detect: []string{"$CFG/Claude/claude_desktop_config.json"},
		MCP: []mcpSource{{"$CFG/Claude/claude_desktop_config.json", mcpServers}}},
	{ID: "opencode", Name: "opencode", Detect: []string{"~/.config/opencode"},
		Skills: []string{"~/.config/opencode/skill", "~/.config/opencode/skills"},
		MCP:    []mcpSource{{"~/.config/opencode/opencode.json", []string{"mcp"}}, {"~/.config/opencode/opencode.jsonc", []string{"mcp"}}}},
	{ID: "zed", Name: "Zed", Detect: []string{"~/.config/zed"},
		MCP: []mcpSource{{"~/.config/zed/settings.json", []string{"context_servers"}}}},
	{ID: "cline", Name: "Cline", Detect: []string{vscodeStorage + "saoudrizwan.claude-dev"},
		MCP: []mcpSource{{vscodeStorage + "saoudrizwan.claude-dev/settings/cline_mcp_settings.json", mcpServers}}},
	{ID: "roo", Name: "Roo Code", Detect: []string{vscodeStorage + "rooveterinaryinc.roo-cline"},
		MCP: []mcpSource{{vscodeStorage + "rooveterinaryinc.roo-cline/settings/mcp_settings.json", mcpServers}}},
	{ID: "kiro", Name: "Kiro", Detect: []string{"~/.kiro"},
		MCP: []mcpSource{{"~/.kiro/settings/mcp.json", mcpServers}}},
	{ID: "amp", Name: "Amp", Detect: []string{"~/.config/amp"},
		MCP: []mcpSource{{"~/.config/amp/settings.json", []string{"amp.mcpServers"}}}},
	{ID: "qwen", Name: "Qwen Code", Detect: []string{"~/.qwen"},
		MCP: []mcpSource{{"~/.qwen/settings.json", mcpServers}}},
	{ID: "factory", Name: "Factory Droid", Detect: []string{"~/.factory"},
		Skills: []string{"~/.factory/skills"}, MCP: []mcpSource{{"~/.factory/mcp.json", mcpServers}}},
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func (s spec) read(e env) Tool {
	t := Tool{ID: s.ID, Name: s.Name, Items: []Item{}}
	for _, d := range s.Detect {
		if exists(e.path(d)) {
			t.Installed = true
			break
		}
	}
	if !t.Installed {
		return t
	}

	seen := map[string]bool{}
	var skills []Item
	for _, d := range s.Skills {
		dir := e.path(d)
		for _, it := range append(readSkills(dir, true), readSkills(dir+"-disabled", false)...) {
			it.Source = d
			if !seen[it.ID] {
				seen[it.ID] = true
				skills = append(skills, it)
			}
		}
	}
	sortByName(skills)
	t.Items = append(t.Items, skills...)

	var mcp []Item
	if s.CodexTOML != "" {
		plugins, m := codexItems(e.path(s.CodexTOML))
		for i := range plugins {
			plugins[i].Source = s.CodexTOML
		}
		for i := range m {
			m[i].Source = s.CodexTOML
		}
		t.Items = append(t.Items, plugins...)
		mcp = append(mcp, m...)
	}
	if s.Extensions != "" {
		t.Items = append(t.Items, readExtensions(e.path(s.Extensions))...)
	}
	for _, src := range s.MCP {
		for _, name := range jsonKeys(e.path(src.File), src.Key) {
			mcp = append(mcp, Item{Kind: "mcp", ID: name, Name: name, Source: src.File, Enabled: true})
		}
	}
	seen = map[string]bool{}
	for _, m := range mcp {
		if !seen[m.ID] {
			seen[m.ID] = true
			t.Items = append(t.Items, m)
		}
	}
	return t
}

func (s spec) toggle(e env, kind, id string, enabled bool) error {
	switch {
	case kind == "skill":
		for _, d := range s.Skills {
			dir := e.path(d)
			if exists(filepath.Join(dir, id)) || exists(filepath.Join(dir+"-disabled", id)) {
				return toggleSkillIn(dir, dir+"-disabled", id, enabled)
			}
		}
		return errNotFound(kind, id)
	case kind == "plugin" && s.CodexTOML != "":
		return toggleCodexPlugin(e.path(s.CodexTOML), id, enabled)
	}
	return errNotToggleable(s.Name, kind)
}

// readExtensions lists Gemini-style extensions (read-only for now).
func readExtensions(dir string) []Item {
	entries, _ := os.ReadDir(dir)
	var items []Item
	for _, en := range entries {
		if exists(filepath.Join(dir, en.Name(), "gemini-extension.json")) {
			items = append(items, Item{Kind: "plugin", ID: en.Name(), Name: en.Name(), Source: "~/.gemini/extensions", Enabled: true})
		}
	}
	return items
}

func sortByName(items []Item) {
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
}
