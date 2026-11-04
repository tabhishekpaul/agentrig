package main

import (
	"os"
	"path/filepath"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/insights"
	"github.com/tabhishekpaul/agentrig/market"
	"github.com/tabhishekpaul/agentrig/mcpserver"
)

// rig wires `arig mcp`'s management tools to the same code the dashboard uses.
func rig() *mcpserver.Rig {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	dir := filepath.Join(cfg, "agentrig")
	m := market.New(version)
	return &mcpserver.Rig{
		Tools: adapters.Detect,
		Usage: func() map[string]insights.Use {
			return insights.ClaudeUsage(filepath.Join(home, ".claude", "projects"), filepath.Join(dir, "usage-cache.json"))
		},
		Search:   m.Search,
		Describe: m.Describe,
		Install: func(id string, tools []string, accept bool) ([]string, error) {
			if len(tools) == 0 { // same targets the dashboard offers
				for _, t := range adapters.Detect() {
					if t.Installed && adapters.SkillDir(t.ID) != "" {
						tools = append(tools, t.ID)
					}
				}
			}
			return tools, m.Install(id, tools, accept)
		},
		Toggle: adapters.Toggle,
		Remove: func(tool, kind, id string) (string, error) {
			return adapters.Remove(tool, kind, id, filepath.Join(dir, "backup"))
		},
	}
}
