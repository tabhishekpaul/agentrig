package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/insights"
	"github.com/tabhishekpaul/agentrig/market"
)

// Rig holds what the rig-management tools act through; main wires the real
// adapters/market/insights functions, tests wire fakes.
type Rig struct {
	Tools    func() []adapters.Tool
	Usage    func() map[string]insights.Use // Claude Code skill use, for cleanup
	Search   func(q string, limit int) ([]market.Skill, error)
	Describe func(id string) (market.Detail, error)
	// Install returns the tools it installed into (all with a skills folder when tools is empty).
	Install func(id string, tools []string, accept bool) ([]string, error)
	Toggle  func(tool, kind, id string, enabled bool) error
	Remove  func(tool, kind, id string) (backup string, err error)
}

var flagText = map[string]string{
	"scripts":    "contains scripts your AI may run",
	"network":    "its scripts reach the network",
	"shell-pipe": "pipes content into a shell",
	"binary":     "contains compiled binaries",
	"large":      "is over 1 MB",
}

func explainFlags(flags []string) string {
	var out []string
	for _, f := range flags {
		out = append(out, f+" ("+flagText[f]+")")
	}
	return strings.Join(out, "; ")
}

func num(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func obj(required []string, props map[string]any) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

var rigTools = []tool{
	{"rig_overview",
		"Overview of the user's AI coding tools on this computer: per tool, how many skills are switched on, " +
			"roughly how many tokens their descriptions add to every message, the MCP servers configured (names only), " +
			"and the top cleanup suggestions (unused, heavy or duplicate skills). Call this first when managing the user's setup.",
		obj(nil, map[string]any{})},
	{"marketplace_search",
		"Search skills.sh, the public skill marketplace, for skills that are NOT installed yet. Returns ids (owner/repo/skill), " +
			"install counts and categories. Use search_skills instead to look through skills already on this computer.",
		obj([]string{"query"}, map[string]any{
			"query": str("What the skill should do, in a few words."),
			"limit": num("Max results (default 10, max 50)."),
		})},
	{"marketplace_skill",
		"Details of one skills.sh skill before installing it: description, files, and safety flags " +
			"(scripts, network, shell-pipe, binary, large). Downloads it to inspect; nothing is installed.",
		obj([]string{"id"}, map[string]any{"id": str("Marketplace id owner/repo/skill, from marketplace_search.")})},
	{"install_skill",
		"Install a skills.sh skill into the user's tools. If the skill has safety flags this fails and lists them: " +
			"show the flags to the user and only call again with accept_flags=true after the user has explicitly confirmed. " +
			"Never set accept_flags on your own. The tool must be restarted to load the new skill.",
		obj([]string{"id"}, map[string]any{
			"id":           str("Marketplace id owner/repo/skill."),
			"tools":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tool ids to install into (e.g. \"claude\", \"codex\"; see rig_overview). Default: every installed tool with a skills folder."},
			"accept_flags": boolean("Only true when the user has confirmed this skill's safety flags."),
		})},
	{"set_skill_enabled",
		"Switch an installed skill on or off. Off keeps the files (moved aside) so it can be switched back on at any time, " +
			"and saves the tokens its description costs on every message. Prefer this over remove_skill.",
		obj([]string{"id", "enabled"}, map[string]any{
			"id":      str("Skill id (its folder name), as shown by rig_overview or search_skills."),
			"enabled": boolean("true to switch on, false to switch off."),
			"tool":    str("Optional tool id (e.g. \"claude\"). Default: every tool that has this skill."),
		})},
	{"remove_skill",
		"Remove an installed skill from one tool. The folder is moved to arig's backups and can be restored from the arig dashboard. " +
			"Only call after the user has confirmed the removal; switching off (set_skill_enabled) is usually enough.",
		obj([]string{"id", "tool"}, map[string]any{
			"id":   str("Skill id (its folder name)."),
			"tool": str("Tool id to remove it from (e.g. \"claude\")."),
		})},
}

func parse[T any](raw json.RawMessage) (T, error) {
	var a T
	if err := json.Unmarshal(raw, &a); err != nil {
		return a, fmt.Errorf("bad arguments: %v", err)
	}
	return a, nil
}

// rig runs a rig-management tool; ok is false for an unknown name.
func (s *Server) rig(name string, raw json.RawMessage) (text string, ok bool, err error) {
	if !slices.ContainsFunc(rigTools, func(t tool) bool { return t.Name == name }) {
		return "", false, nil
	}
	if s.Rig == nil {
		return "", true, errors.New("rig management isn't available in this arig")
	}
	switch name {
	case "rig_overview":
		text = s.overview()
	case "marketplace_search":
		text, err = s.marketSearch(raw)
	case "marketplace_skill":
		text, err = s.marketSkill(raw)
	case "install_skill":
		text, err = s.install(raw)
	case "set_skill_enabled":
		text, err = s.setEnabled(raw)
	case "remove_skill":
		text, err = s.remove(raw)
	}
	return text, true, err
}

func (s *Server) overview() string {
	tools := s.Rig.Tools()
	usage := s.Rig.Usage()
	cost := map[string]insights.ToolCost{}
	for _, tc := range insights.Build(tools, usage, time.Now()).Tools {
		cost[tc.Tool] = tc
	}
	var b strings.Builder
	total := 0
	for _, t := range tools {
		if !t.Installed {
			continue
		}
		var mcp []string
		for _, it := range t.Items {
			if it.Kind == "mcp" {
				mcp = append(mcp, it.Name)
			}
		}
		c := cost[t.ID]
		total += c.Tokens
		fmt.Fprintf(&b, "%s (tool id: %s): %d skills on, ≈%d tokens/message", t.Name, t.ID, c.Enabled, c.Tokens)
		if len(mcp) > 0 {
			fmt.Fprintf(&b, "; MCP servers: %s", strings.Join(mcp, ", "))
		}
		b.WriteByte('\n')
		if len(c.Skills) > 0 {
			var top []string
			for _, sc := range c.Skills[:min(len(c.Skills), 5)] {
				top = append(top, fmt.Sprintf("%s ≈%d", sc.ID, sc.Tokens))
			}
			fmt.Fprintf(&b, "  most expensive: %s\n", strings.Join(top, ", "))
		}
	}
	if b.Len() == 0 {
		return "No supported AI tools found on this computer."
	}
	fmt.Fprintf(&b, "Total: ≈%d tokens/message across all tools.\n", total)
	sug := insights.Cleanup(tools, usage, insights.Duplicates(tools), time.Now())
	if len(sug) > 0 {
		b.WriteString("\nTop cleanup suggestions:\n")
		for _, sg := range sug[:min(len(sug), 10)] {
			state := "off"
			if sg.Enabled {
				state = fmt.Sprintf("on, ≈%d tokens/message", sg.Tokens)
			}
			var why []string
			for _, r := range sg.Reasons {
				why = append(why, r.Text)
			}
			fmt.Fprintf(&b, "- %s %s in %s (%s): %s\n", sg.Kind, sg.ID, sg.Tool, state, strings.Join(why, "; "))
		}
	}
	return b.String()
}

func (s *Server) marketSearch(raw json.RawMessage) (string, error) {
	a, err := parse[struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}](raw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(a.Query) == "" {
		return "", errors.New("give a query")
	}
	if a.Limit <= 0 {
		a.Limit = 10
	}
	res, err := s.Rig.Search(a.Query, min(a.Limit, 50))
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		return "No skills.sh skill matches. Try other words.", nil
	}
	var b strings.Builder
	for i, sk := range res[:min(len(res), a.Limit, 50)] {
		fmt.Fprintf(&b, "%d. %s (id: %s) %d installs [%s]\n", i+1, sk.Name, sk.ID, sk.Installs, sk.Category)
	}
	b.WriteString("Details and safety flags: marketplace_skill {\"id\": \"<id>\"}\n")
	return b.String(), nil
}

func (s *Server) marketSkill(raw json.RawMessage) (string, error) {
	a, err := parse[struct {
		ID string `json:"id"`
	}](raw)
	if err != nil {
		return "", err
	}
	d, err := s.Rig.Describe(a.ID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\nPage: %s\nRepo: %s\n", a.ID, d.Description, d.Page, d.Repo)
	if len(d.Flags) > 0 {
		fmt.Fprintf(&b, "Safety flags: %s. Installing needs the user's explicit confirmation.\n", explainFlags(d.Flags))
	} else {
		b.WriteString("Safety flags: none.\n")
	}
	fmt.Fprintf(&b, "Files (%d):\n", len(d.Files))
	for _, f := range d.Files[:min(len(d.Files), 50)] {
		fmt.Fprintf(&b, "- %s (%d bytes)\n", f.Path, f.Size)
	}
	if len(d.Files) > 50 {
		fmt.Fprintf(&b, "… and %d more\n", len(d.Files)-50)
	}
	return b.String(), nil
}

func (s *Server) install(raw json.RawMessage) (string, error) {
	a, err := parse[struct {
		ID     string   `json:"id"`
		Tools  []string `json:"tools"`
		Accept bool     `json:"accept_flags"`
	}](raw)
	if err != nil {
		return "", err
	}
	if !a.Accept {
		d, err := s.Rig.Describe(a.ID)
		if err != nil {
			return "", err
		}
		if len(d.Flags) > 0 {
			return "", fmt.Errorf("not installed: %s has safety flags: %s. Show these to the user; "+
				"only if they explicitly confirm, call install_skill again with accept_flags=true", a.ID, explainFlags(d.Flags))
		}
	}
	tools, err := s.Rig.Install(a.ID, a.Tools, a.Accept)
	if errors.Is(err, market.ErrConfirm) {
		return "", fmt.Errorf("not installed: %s has safety flags; ask the user to confirm, then call again with accept_flags=true", a.ID)
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Installed %s into %s. Restart those tools to load it.", a.ID, strings.Join(tools, ", ")), nil
}

func (s *Server) setEnabled(raw json.RawMessage) (string, error) {
	a, err := parse[struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
		Tool    string `json:"tool"`
	}](raw)
	if err != nil {
		return "", err
	}
	if a.ID == "" || a.Enabled == nil {
		return "", errors.New("give id and enabled")
	}
	var lines []string
	saved := 0
	for _, t := range s.Rig.Tools() {
		if !t.Installed || (a.Tool != "" && t.ID != a.Tool) {
			continue
		}
		i := slices.IndexFunc(t.Items, func(it adapters.Item) bool { return it.Kind == "skill" && it.ID == a.ID })
		if i < 0 {
			continue
		}
		it := t.Items[i]
		switch {
		case it.Enabled == *a.Enabled:
			lines = append(lines, t.Name+": already "+onOff(it.Enabled))
		case !it.Toggleable:
			lines = append(lines, t.Name+": can't be switched by arig")
		default:
			if err := s.Rig.Toggle(t.ID, "skill", a.ID, *a.Enabled); err != nil {
				lines = append(lines, t.Name+": failed: "+err.Error())
				continue
			}
			lines = append(lines, t.Name+": switched "+onOff(*a.Enabled))
			saved += insights.Tokens(it.Name, it.Description)
		}
	}
	if len(lines) == 0 {
		where := "any tool"
		if a.Tool != "" {
			where = a.Tool
		}
		return "", fmt.Errorf("no skill %q in %s; see rig_overview or search_skills", a.ID, where)
	}
	out := a.ID + ":\n- " + strings.Join(lines, "\n- ")
	if saved > 0 {
		verb := "Saves"
		if *a.Enabled {
			verb = "Adds"
		}
		out += fmt.Sprintf("\n%s ≈%d tokens per message. Restart the tool to apply.", verb, saved)
	}
	return out, nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (s *Server) remove(raw json.RawMessage) (string, error) {
	a, err := parse[struct {
		ID   string `json:"id"`
		Tool string `json:"tool"`
	}](raw)
	if err != nil {
		return "", err
	}
	if a.ID == "" || a.Tool == "" {
		return "", errors.New("give id and tool")
	}
	backup, err := s.Rig.Remove(a.Tool, "skill", a.ID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed %s from %s. It is in arig's backups (%s) and can be restored from the arig dashboard.", a.ID, a.Tool, backup), nil
}
