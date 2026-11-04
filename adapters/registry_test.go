package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONKeysHandlesJSONCAndEmpty(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "settings.json")
	os.WriteFile(f, []byte(`// comment
{
  /* block */ "url": "http://x//not-a-comment",
  "context_servers": { "b": {"env": {"KEY": "secret"}}, "a": {}, },
}`), 0o644)
	if got := strings.Join(jsonKeys(f, []string{"context_servers"}), ","); got != "a,b" {
		t.Fatalf("got %q", got)
	}
	os.WriteFile(f, nil, 0o644)
	if jsonKeys(f, mcpServers) != nil {
		t.Fatal("empty file should yield nothing")
	}
}

const codexTOML = `model = "x"

[plugins."browser@openai-bundled"]
enabled = true # keep me

[plugins.noflag]

[mcp_servers.node_repl]
command = "node"

[mcp_servers.node_repl.env]
TOKEN = "secret"
`

func TestCodexTOML(t *testing.T) {
	f := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(f, []byte(codexTOML), 0o600)

	plugins, mcp := codexItems(f)
	if len(plugins) != 2 || len(mcp) != 1 || mcp[0].Name != "node_repl" {
		t.Fatalf("plugins=%+v mcp=%+v", plugins, mcp)
	}
	if err := toggleCodexPlugin(f, "browser@openai-bundled", false); err != nil {
		t.Fatal(err)
	}
	if err := toggleCodexPlugin(f, "noflag", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	want := strings.Replace(codexTOML, "enabled = true # keep me", "enabled = false # keep me", 1)
	want = strings.Replace(want, "[plugins.noflag]\n", "[plugins.noflag]\nenabled = false\n", 1)
	if string(b) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b, want)
	}
	if toggleCodexPlugin(f, "missing", true) == nil {
		t.Fatal("toggled a plugin that doesn't exist")
	}
}

func TestSpecReadAndToggle(t *testing.T) {
	e := env{home: t.TempDir(), cfg: t.TempDir()}
	s := spec{ID: "x", Name: "X", Detect: []string{"~/.x"}, Skills: []string{"~/.x/skills"},
		MCP: []mcpSource{{"~/.x/mcp.json", mcpServers}}}

	if s.read(e).Installed {
		t.Fatal("detected a tool that isn't there")
	}
	os.MkdirAll(filepath.Join(e.home, ".x/skills/s1"), 0o755)
	os.WriteFile(filepath.Join(e.home, ".x/skills/s1/SKILL.md"), []byte("---\nname: s1\n---\n"), 0o644)
	os.WriteFile(filepath.Join(e.home, ".x/mcp.json"), []byte(`{"mcpServers":{"gh":{}}}`), 0o644)

	tool := s.read(e)
	if !tool.Installed || len(tool.Items) != 2 || tool.Items[0].Kind != "skill" || tool.Items[1].Name != "gh" {
		t.Fatalf("got %+v", tool)
	}
	if err := s.toggle(e, "skill", "s1", false); err != nil {
		t.Fatal(err)
	}
	if it := s.read(e).Items[0]; it.ID != "s1" || it.Enabled {
		t.Fatalf("skill not disabled: %+v", it)
	}
	if s.toggle(e, "mcp", "gh", false) == nil {
		t.Fatal("MCP toggle should be refused for now")
	}
}
