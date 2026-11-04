package mcpserver

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/insights"
	"github.com/tabhishekpaul/agentrig/market"
)

type fakeRig struct {
	calls []string
	tools []adapters.Tool
}

func (f *fakeRig) rig() *Rig {
	return &Rig{
		Tools: func() []adapters.Tool { return f.tools },
		Usage: func() map[string]insights.Use { return nil },
		Search: func(q string, limit int) ([]market.Skill, error) {
			return []market.Skill{{ID: "acme/skills/pdf", Name: "pdf", Installs: 1200, Category: "documents/pdf"}}, nil
		},
		Describe: func(id string) (market.Detail, error) {
			d := market.Detail{Description: "Fill PDFs", Files: []market.FileInfo{{Path: "SKILL.md", Size: 10}}}
			if strings.HasSuffix(id, "/risky") {
				d.Flags = []string{"scripts", "network"}
			}
			return d, nil
		},
		Install: func(id string, tools []string, accept bool) ([]string, error) {
			f.calls = append(f.calls, fmt.Sprintf("install %s %v %v", id, tools, accept))
			if len(tools) == 0 {
				tools = []string{"claude", "codex"}
			}
			return tools, nil
		},
		Toggle: func(tool, kind, id string, on bool) error {
			f.calls = append(f.calls, fmt.Sprintf("toggle %s %s %s %v", tool, kind, id, on))
			return nil
		},
		Remove: func(tool, kind, id string) (string, error) {
			f.calls = append(f.calls, fmt.Sprintf("remove %s %s %s", tool, kind, id))
			if id == "missing" {
				return "", errors.New(`skill "missing" not found`)
			}
			return "2026-01-01T00-00-00-removed", nil
		},
	}
}

func TestRigTools(t *testing.T) {
	f := &fakeRig{tools: []adapters.Tool{
		{ID: "claude", Name: "Claude Code", Installed: true, Items: []adapters.Item{
			{Kind: "skill", ID: "pdf", Name: "pdf", Description: strings.Repeat("x", 400), Enabled: true, Toggleable: true},
			{Kind: "mcp", ID: "github", Name: "github", Enabled: true},
		}},
		{ID: "codex", Name: "Codex", Installed: true, Items: []adapters.Item{
			{Kind: "skill", ID: "pdf", Name: "pdf", Description: "short", Enabled: false, Toggleable: true},
		}},
		{ID: "cursor", Name: "Cursor"},
	}}
	c := startRig(t, nil, f.rig())

	text, isErr := c.tool("rig_overview", nil)
	if isErr || !strings.Contains(text, "Claude Code (tool id: claude): 1 skills on, ≈101 tokens/message; MCP servers: github") ||
		!strings.Contains(text, "Codex (tool id: codex): 0 skills on") || strings.Contains(text, "Cursor") ||
		!strings.Contains(text, "Top cleanup suggestions") {
		t.Fatalf("overview: %q", text)
	}

	if text, isErr := c.tool("marketplace_search", map[string]any{"query": "pdf"}); isErr || !strings.Contains(text, "pdf (id: acme/skills/pdf) 1200 installs [documents/pdf]") {
		t.Fatalf("search: %q", text)
	}
	if text, _ := c.tool("marketplace_skill", map[string]any{"id": "acme/skills/risky"}); !strings.Contains(text, "scripts (contains scripts") || !strings.Contains(text, "- SKILL.md (10 bytes)") {
		t.Fatalf("skill: %q", text)
	}

	// flagged: refused without accept_flags, and Install is never reached
	if text, isErr := c.tool("install_skill", map[string]any{"id": "acme/skills/risky"}); !isErr || !strings.Contains(text, "network") || len(f.calls) != 0 {
		t.Fatalf("flagged install: %q %v", text, f.calls)
	}
	if text, isErr := c.tool("install_skill", map[string]any{"id": "acme/skills/risky", "accept_flags": true, "tools": []string{"claude"}}); isErr || !strings.Contains(text, "into claude") {
		t.Fatalf("accepted install: %q", text)
	}
	if text, _ := c.tool("install_skill", map[string]any{"id": "acme/skills/pdf"}); !strings.Contains(text, "into claude, codex") {
		t.Fatalf("default tools: %q", text)
	}

	// off everywhere: only claude has it on
	f.calls = nil
	text, isErr = c.tool("set_skill_enabled", map[string]any{"id": "pdf", "enabled": false})
	if isErr || !strings.Contains(text, "Claude Code: switched off") || !strings.Contains(text, "Codex: already off") || !strings.Contains(text, "Saves ≈101 tokens") {
		t.Fatalf("disable: %q", text)
	}
	if strings.Join(f.calls, "|") != "toggle claude skill pdf false" {
		t.Fatalf("toggle calls: %v", f.calls)
	}
	if _, isErr := c.tool("set_skill_enabled", map[string]any{"id": "pdf", "enabled": true, "tool": "cursor"}); !isErr {
		t.Fatal("skill not in that tool should be an error")
	}
	if _, isErr := c.tool("set_skill_enabled", map[string]any{"id": "pdf"}); !isErr {
		t.Fatal("missing enabled should be an error")
	}

	if text, isErr := c.tool("remove_skill", map[string]any{"id": "pdf", "tool": "claude"}); isErr || !strings.Contains(text, "backups (2026-01-01T00-00-00-removed)") {
		t.Fatalf("remove: %q", text)
	}
	if _, isErr := c.tool("remove_skill", map[string]any{"id": "pdf"}); !isErr {
		t.Fatal("remove without tool should be an error")
	}
}

func TestRigUnavailable(t *testing.T) {
	c := start(t, nil)
	if _, isErr := c.tool("rig_overview", nil); !isErr {
		t.Fatal("rig tools without a Rig should be tool errors")
	}
}
