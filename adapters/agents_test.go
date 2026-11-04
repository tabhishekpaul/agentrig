package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgents(t *testing.T) {
	e, _ := fakeHome(t) // claude, cursor, codex, zed, opencode installed
	os.MkdirAll(filepath.Join(e.home, ".kiro"), 0o755)

	var tools []string
	for _, a := range agentStatus(e) {
		tools = append(tools, a.Tool)
		if a.Enabled {
			t.Fatalf("nothing written yet: %+v", a)
		}
	}
	if strings.Join(tools, ",") != "claude,opencode,kiro" { // codex/cursor have no agents
		t.Fatalf("tools: %v", tools)
	}

	for _, tool := range []string{"claude", "opencode", "kiro"} {
		if err := setAgent(e, tool, true); err != nil {
			t.Fatal(tool, err)
		}
		if err := setAgent(e, tool, true); err != nil { // rewriting our own file is fine
			t.Fatal(tool, err)
		}
	}
	md, _ := os.ReadFile(filepath.Join(e.home, ".claude", "agents", "arig.md"))
	if !strings.HasPrefix(string(md), "---\nname: arig\ndescription: \"Agentrig") || !strings.Contains(string(md), "tools: mcp__arig__*\n---\n") {
		t.Fatalf("claude agent:\n%s", md)
	}
	op, _ := os.ReadFile(filepath.Join(e.home, ".config", "opencode", "agents", "arig.md"))
	if !strings.Contains(string(op), "mode: subagent") {
		t.Fatalf("opencode agent:\n%s", op)
	}
	var k struct {
		Name, Prompt string
		Tools        []string
	}
	kb, _ := os.ReadFile(filepath.Join(e.home, ".kiro", "agents", "arig.json"))
	if json.Unmarshal(kb, &k) != nil || k.Name != "arig" || k.Tools[0] != "@arig" || !strings.HasPrefix(k.Prompt, agentMarker) {
		t.Fatalf("kiro agent: %s", kb)
	}
	for _, a := range agentStatus(e) {
		if !a.Enabled || a.Note != "" {
			t.Fatalf("written: %+v", a)
		}
	}

	if err := setAgent(e, "claude", false); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(e.home, ".claude", "agents", "arig.md")) {
		t.Fatal("not removed")
	}
	if err := setAgent(e, "claude", false); err != nil { // already off
		t.Fatal(err)
	}

	// a user's own arig.md is never overwritten or removed
	own := filepath.Join(e.home, ".config", "opencode", "agents", "arig.md")
	os.WriteFile(own, []byte("---\ndescription: mine\n---\nmy agent\n"), 0o644)
	for _, on := range []bool{true, false} {
		if err := setAgent(e, "opencode", on); err == nil {
			t.Fatalf("overwrote user's file (on=%v)", on)
		}
	}
	if b, _ := os.ReadFile(own); string(b) != "---\ndescription: mine\n---\nmy agent\n" {
		t.Fatalf("user's file changed: %s", b)
	}
	for _, a := range agentStatus(e) {
		if a.Tool == "opencode" && (a.Enabled || a.Note == "") {
			t.Fatalf("user file status: %+v", a)
		}
	}

	if err := setAgent(e, "codex", true); err == nil {
		t.Fatal("codex has no agents")
	}
	if err := setAgent(e, "gemini", true); err == nil {
		t.Fatal("gemini isn't installed")
	}
}
