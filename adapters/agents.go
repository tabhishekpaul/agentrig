package adapters

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// agentMarker is in every agent file arig writes; a file without it is the
// user's own and is never overwritten or removed.
const agentMarker = "<!-- arig-agent: written by arig, which may rewrite or remove this file. Delete this line to keep your own edits. -->"

const agentDesc = "Agentrig rig manager: finds, installs, switches on/off and tidies the AI skills installed across your coding tools " +
	"(via arig's MCP tools). Use when the user asks about their skills or MCP setup, wants a new skill, or wants to cut token use."

const agentPrompt = `You are arig, the manager of this computer's AI-tool setup (the "rig"), built on Agentrig.
You work through the arig MCP tools: rig_overview, search_skills, get_skill, list_categories, marketplace_search, marketplace_skill, install_skill, set_skill_enabled, remove_skill.
If those tools are missing, tell the user to connect arig's MCP server from the AI access page of the arig dashboard (run ` + "`arig`" + `).

How to work:
- Start with rig_overview: enabled skills per tool, their token cost per message, MCP servers and cleanup suggestions.
- To find a skill, look among installed skills first (search_skills), then on skills.sh (marketplace_search, then marketplace_skill for its description, files and safety flags).
- Before installing, tell the user what the skill does, which tools get it and any safety flags. Never pass accept_flags=true unless the user explicitly confirmed those flags in this conversation.
- Prefer switching skills off (set_skill_enabled) over removing them. Call remove_skill only after the user confirms; removed skills go to arig's backups and can be restored from the dashboard.
- Don't change anything the user didn't ask about without asking first.
- After changes, report what changed and the token savings per message, and remind the user to restart the affected tools.
Keep answers short.
`

// agentFormat is how one tool defines a custom agent / subagent.
type agentFormat struct {
	tool, file string // file under ~/
	render     func() []byte
}

// yamlStr quotes s for YAML (a JSON string is a valid YAML double-quoted scalar).
func yamlStr(s string) string { return tomlString(s) }

// markdownAgent renders YAML frontmatter lines followed by the marker and prompt.
func markdownAgent(front ...string) func() []byte {
	return func() []byte {
		var b bytes.Buffer
		b.WriteString("---\n")
		for _, l := range front {
			b.WriteString(l + "\n")
		}
		b.WriteString("---\n\n" + agentMarker + "\n\n" + agentPrompt)
		return b.Bytes()
	}
}

// Sources: code.claude.com/docs/en/sub-agents, opencode.ai/docs/agents,
// docs.factory.com/cli/configuration/custom-droids, docs.github.com (Copilot CLI
// custom agents), geminicli.com/docs/core/subagents,
// kiro.dev/docs/custom-agents/configuration-reference. Codex has no subagents
// and its custom prompts are deprecated in favour of skills, so it has none.
var agentFormats = []agentFormat{
	{"claude", "~/.claude/agents/arig.md", markdownAgent("name: arig", "description: "+yamlStr(agentDesc), "tools: mcp__arig__*")},
	{"opencode", "~/.config/opencode/agents/arig.md", markdownAgent("description: "+yamlStr(agentDesc), "mode: subagent")},
	{"factory", "~/.factory/droids/arig.md", markdownAgent("name: arig", "description: "+yamlStr(agentDesc), "model: inherit")},
	{"copilot-cli", "~/.copilot/agents/arig.agent.md", markdownAgent("name: arig", "description: "+yamlStr(agentDesc))},
	{"gemini", "~/.gemini/agents/arig.md", markdownAgent("name: arig", "description: "+yamlStr(agentDesc), "tools:", "  - mcp_arig_*")},
	{"kiro", "~/.kiro/agents/arig.json", func() []byte {
		return marshal(map[string]any{"name": "arig", "description": agentDesc, "prompt": agentMarker + "\n\n" + agentPrompt,
			"tools": []string{"@" + MCPName}, "includeMcpJson": true})
	}},
}

// Agent is arig's agent definition in one installed tool.
type Agent struct {
	Tool    string `json:"tool"`
	File    string `json:"file"` // as ~/… for display
	Enabled bool   `json:"enabled"`
	Note    string `json:"note,omitempty"` // why arig can't change it
}

// AgentStatus lists every installed tool that supports custom agents.
func AgentStatus() []Agent { return agentStatus(realEnv()) }

func agentStatus(e env) []Agent {
	out := []Agent{}
	for _, f := range agentFormats {
		if !toolInstalled(e, f.tool) {
			continue
		}
		a := Agent{Tool: f.tool, File: e.display(f.file)}
		owned, exist, err := agentOwned(e.path(f.file))
		switch {
		case err != nil:
			a.Note = err.Error()
		case exist && !owned:
			a.Note = "An agent file arig didn't write is already there; arig won't touch it."
		}
		a.Enabled = owned
		out = append(out, a)
	}
	return out
}

func toolInstalled(e env, tool string) bool {
	if tool == "claude" {
		return exists(filepath.Join(e.home, ".claude"))
	}
	for _, s := range specs {
		if s.ID == tool {
			return slices.ContainsFunc(s.Detect, func(d string) bool { return exists(e.path(d)) })
		}
	}
	return false
}

func agentOwned(p string) (owned, exist bool, err error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, true, err
	}
	return bytes.Contains(b, []byte(agentMarker)), true, nil
}

// SetAgent writes (on) or removes arig's agent file for one tool. A file arig
// didn't write is never overwritten or removed.
func SetAgent(tool string, on bool) error {
	mu.Lock()
	defer mu.Unlock()
	return setAgent(realEnv(), tool, on)
}

func setAgent(e env, tool string, on bool) error {
	i := slices.IndexFunc(agentFormats, func(f agentFormat) bool { return f.tool == tool })
	if i < 0 || !toolInstalled(e, tool) {
		return fmt.Errorf("%q isn't an installed tool with custom agents", tool)
	}
	f := agentFormats[i]
	p := e.path(f.file)
	owned, exist, err := agentOwned(p)
	if err != nil {
		return err
	}
	if exist && !owned {
		return fmt.Errorf("%s exists and wasn't written by arig; rename or delete it first", e.display(f.file))
	}
	if !on {
		if !exist {
			return nil
		}
		return os.Remove(p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return writeAtomic(p, f.render())
}
