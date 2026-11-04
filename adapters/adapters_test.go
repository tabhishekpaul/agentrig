package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fakeClaude(t *testing.T) claudePaths {
	root := t.TempDir()
	p := claudePaths{Root: root, MCPFile: filepath.Join(root, "mcp.json"), Settings: filepath.Join(root, "settings.json")}
	os.MkdirAll(filepath.Join(p.skills(), "good"), 0o755)
	os.WriteFile(filepath.Join(p.skills(), "good", "SKILL.md"),
		[]byte("---\nname: good-skill\ndescription: \"Does: things\"\n---\nbody\n"), 0o644)
	os.MkdirAll(filepath.Join(p.skills(), "noskill"), 0o755)
	os.WriteFile(filepath.Join(p.skills(), ".DS_Store"), nil, 0o644)
	return p
}

func TestClaudeSkills(t *testing.T) {
	got := claudeSkills(fakeClaude(t).skills(), true)
	if len(got) != 1 || got[0].ID != "good" || got[0].Name != "good-skill" || got[0].Description != "Does: things" {
		t.Fatalf("got %+v", got)
	}
}

func TestToggleSkillRoundTrip(t *testing.T) {
	p := fakeClaude(t)
	// a relative symlink like the ones skill installers create must survive the move
	os.MkdirAll(filepath.Join(p.Root, "store", "linked"), 0o755)
	os.WriteFile(filepath.Join(p.Root, "store", "linked", "SKILL.md"), []byte("---\nname: linked\n---\n"), 0o644)
	os.Symlink("../store/linked", filepath.Join(p.skills(), "linked"))

	for _, id := range []string{"good", "linked"} {
		if err := claudeToggle(p, "skill", id, false); err != nil {
			t.Fatal(err)
		}
		if d := claudeSkills(p.disabled(), false); !has(d, id) {
			t.Fatalf("%s not disabled: %+v", id, d)
		}
		if err := claudeToggle(p, "skill", id, true); err != nil {
			t.Fatal(err)
		}
		if e := claudeSkills(p.skills(), true); !has(e, id) {
			t.Fatalf("%s not re-enabled: %+v", id, e)
		}
	}
	for _, bad := range []string{"", "..", "../x", "a/b", ".hidden"} {
		if claudeToggle(p, "skill", bad, false) == nil {
			t.Fatalf("accepted bad id %q", bad)
		}
	}
}

func TestTogglePluginKeepsOtherSettings(t *testing.T) {
	p := fakeClaude(t)
	os.WriteFile(p.Settings, []byte(`{"theme":{"x":[1,2]},"enabledPlugins":{"a@m":true}}`), 0o600)

	if err := claudeToggle(p, "plugin", "a@m", false); err != nil {
		t.Fatal(err)
	}
	var s map[string]json.RawMessage
	b, _ := os.ReadFile(p.Settings)
	json.Unmarshal(b, &s)
	if string(s["theme"]) == "" || string(compact(s["theme"])) != `{"x":[1,2]}` {
		t.Fatalf("lost other keys: %s", b)
	}
	if string(compact(s["enabledPlugins"])) != `{"a@m":false}` {
		t.Fatalf("plugin not disabled: %s", b)
	}
	if st, _ := os.Stat(p.Settings); st.Mode().Perm() != 0o600 {
		t.Fatalf("file mode changed to %v", st.Mode().Perm())
	}

	os.WriteFile(p.Settings, []byte(`{broken`), 0o600)
	if claudeToggle(p, "plugin", "a@m", true) == nil {
		t.Fatal("overwrote invalid settings.json")
	}
}

func has(items []Item, id string) bool {
	for _, i := range items {
		if i.ID == id {
			return true
		}
	}
	return false
}

func compact(b []byte) []byte {
	var v any
	json.Unmarshal(b, &v)
	out, _ := json.Marshal(v)
	return out
}
