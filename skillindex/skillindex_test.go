package skillindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/adapters"
)

func TestSyncWritesAndLinksIndex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := t.TempDir()
	configDir = func() string { return cfg }
	skill := func(dir, name, desc string) string {
		p := filepath.Join(home, dir, name)
		os.MkdirAll(p, 0o755)
		os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: "+desc+"\n---\n"), 0o644)
		return p
	}
	pdf := skill(".claude/skills", "pdf", "Fill and merge PDF forms")
	vid := skill(".claude/skills-disabled", "video-editor", "Cut and trim video clips with ffmpeg")
	tools := []adapters.Tool{{ID: "claude", Name: "Claude Code", Installed: true, Items: []adapters.Item{
		{Kind: "skill", ID: "pdf", Name: "pdf", Description: "Fill and merge PDF forms", Path: pdf, Enabled: true},
		{Kind: "skill", ID: "video-editor", Name: "video-editor", Description: "Cut and trim video clips with ffmpeg", Path: vid},
	}}}

	changed, err := Sync(home, tools)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	link := filepath.Join(home, ".claude/skills", Name, "SKILL.md")
	b, err := os.ReadFile(link)
	if err != nil {
		t.Fatal("index not reachable from the tool:", err)
	}
	md := string(b)
	for _, want := range []string{"name: skills-index", "## Documents · PDF", "## Media · Video", "**video-editor** *(off)*", filepath.Join(pdf, "SKILL.md")} {
		if !strings.Contains(md, want) {
			t.Errorf("index missing %q:\n%s", want, md)
		}
	}
	if changed, _ := Sync(home, tools); changed {
		t.Error("unchanged skills should not rewrite the index")
	}

	// switched off: the link goes, a user's own folder of that name never does
	Save(Settings{Enabled: false})
	Sync(home, tools)
	if _, err := os.Lstat(filepath.Join(home, ".claude/skills", Name)); err == nil {
		t.Error("index link should be removed when disabled")
	}
	mine := skill(".claude/skills", Name, "my own")
	Sync(home, tools)
	if _, err := os.Stat(filepath.Join(mine, "SKILL.md")); err != nil {
		t.Error("a real folder named skills-index must never be removed")
	}
}

func TestShortAndCost(t *testing.T) {
	if s := short(strings.Repeat("word ", 80)); len([]rune(s)) > 160 || !strings.HasSuffix(s, "…") {
		t.Errorf("short: %q", s)
	}
	_, idx := Cost([]adapters.Tool{{ID: "claude", Installed: true}})
	if idx < 30 || idx > 90 {
		t.Errorf("index description should cost ~60 tokens, got %d", idx)
	}
}
