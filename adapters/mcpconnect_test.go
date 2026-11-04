package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeHome(t *testing.T) (env, claudePaths) {
	home := t.TempDir()
	e := env{home, filepath.Join(home, "cfg")}
	cp := claudePaths{Root: filepath.Join(home, ".claude"), MCPFile: filepath.Join(home, ".claude.json"), Settings: filepath.Join(home, ".claude", "settings.json")}
	for _, d := range []string{cp.Root, filepath.Join(home, ".cursor"), filepath.Join(home, ".codex"), filepath.Join(home, ".config", "zed"), filepath.Join(home, ".config", "opencode")} {
		os.MkdirAll(d, 0o755)
	}
	return e, cp
}

func client(t *testing.T, cs []MCPClient, tool string) MCPClient {
	for _, c := range cs {
		if c.Tool == tool {
			return c
		}
	}
	t.Fatalf("no client %q in %+v", tool, cs)
	return MCPClient{}
}

func TestSetMCPKeepsOtherServers(t *testing.T) {
	e, cp := fakeHome(t)
	os.WriteFile(cp.MCPFile, []byte(`{"numStartups":3,"mcpServers":{"other":{"env":{"KEY":"secret"}}},"projects":{"/x":{"a":1}}}`), 0o600)

	if err := setMCP(e, cp, "claude", "/bin/arig", true); err != nil {
		t.Fatal(err)
	}
	var got struct {
		NumStartups int
		MCPServers  map[string]map[string]any `json:"mcpServers"`
		Projects    map[string]any
	}
	b, _ := os.ReadFile(cp.MCPFile)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.NumStartups != 3 || got.Projects["/x"] == nil || got.MCPServers["other"]["env"] == nil || got.MCPServers["arig"]["command"] != "/bin/arig" {
		t.Fatalf("lost data: %s", b)
	}
	if st, _ := os.Stat(cp.MCPFile); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode changed: %v", st.Mode())
	}
	if c := client(t, mcpClients(e, cp, "/bin/arig"), "claude"); !c.Configured || !c.Writable || strings.Contains(c.Snippet, "secret") {
		t.Fatalf("client: %+v", c)
	}

	if err := setMCP(e, cp, "claude", "/bin/arig", false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(cp.MCPFile)
	if strings.Contains(string(b), "/bin/arig") || !strings.Contains(string(b), "secret") {
		t.Fatalf("remove: %s", b)
	}
}

func TestSetMCPCreatesFileAndDialects(t *testing.T) {
	e, cp := fakeHome(t)
	if err := setMCP(e, cp, "cursor", "/bin/arig", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(e.home, ".cursor", "mcp.json"))
	if !strings.Contains(string(b), `"args": [`) || !strings.Contains(string(b), `"command": "/bin/arig"`) {
		t.Fatalf("cursor: %s", b)
	}
	if err := setMCP(e, cp, "opencode", "/bin/arig", true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(e.home, ".config", "opencode", "opencode.json"))
	if !strings.Contains(string(b), `"type": "local"`) {
		t.Fatalf("opencode: %s", b)
	}
}

func TestSetMCPRefusesJSONCAndTOML(t *testing.T) {
	e, cp := fakeHome(t)
	zed := filepath.Join(e.home, ".config", "zed", "settings.json")
	orig := "// my settings\n{\"theme\": \"x\"}\n"
	os.WriteFile(zed, []byte(orig), 0o644)
	if setMCP(e, cp, "zed", "/bin/arig", true) == nil {
		t.Fatal("wrote a file with comments")
	}
	if b, _ := os.ReadFile(zed); string(b) != orig {
		t.Fatalf("zed changed: %s", b)
	}
	cs := mcpClients(e, cp, "/bin/arig")
	if c := client(t, cs, "zed"); c.Writable || !strings.Contains(c.Snippet, "context_servers") {
		t.Fatalf("zed: %+v", c)
	}
	if setMCP(e, cp, "codex", "/bin/arig", true) == nil {
		t.Fatal("wrote TOML")
	}
	if c := client(t, cs, "codex"); c.Writable || !strings.Contains(c.Snippet, "[mcp_servers.arig]") {
		t.Fatalf("codex: %+v", c)
	}
	if setMCP(e, cp, "gemini", "/bin/arig", true) == nil {
		t.Fatal("wrote to an uninstalled tool")
	}
}

func TestClaudeCLIArgs(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "claude")
	os.WriteFile(bin, []byte("#!/bin/sh\necho \"$@\" >> "+log+"\n"), 0o755)
	if err := claudeCLI(bin, "/x/arig", true); err != nil {
		t.Fatal(err)
	}
	if err := claudeCLI(bin, "/x/arig", false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(log)
	want := "mcp add --scope user arig -- /x/arig mcp\nmcp remove --scope user arig\n"
	if string(got) != want {
		t.Fatalf("got %q", got)
	}
}
