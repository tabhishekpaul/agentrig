package insights

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
)

func TestCleanup(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	big := filepath.Join(dir, "big")
	os.MkdirAll(big, 0o755)
	os.WriteFile(filepath.Join(big, "SKILL.md"), []byte(strings.Repeat("x", HeavyLoad*4)), 0o644)
	tools := []adapters.Tool{{ID: "claude", Name: "Claude Code", Installed: true, Items: []adapters.Item{
		{Kind: "skill", ID: "used", Name: "used", Enabled: true},
		{Kind: "skill", ID: "stale", Name: "stale", Enabled: true},
		{Kind: "skill", ID: "big", Name: "big", Path: big, Enabled: false},
		{Kind: "skill", ID: "copy", Name: "copy", Enabled: true},
		{Kind: "skill", ID: "wordy", Name: "wordy", Description: strings.Repeat("w", HeavyPerMessage*4), Enabled: true},
		{Kind: "plugin", ID: "sp@m", Name: "sp@m", Enabled: true},
		{Kind: "mcp", ID: "srv", Name: "srv", Enabled: true},
	}}}
	usage := map[string]Use{
		"used": {1, now}, "stale": {1, now.Add(-40 * 24 * time.Hour)}, "big": {1, now}, "copy": {1, now},
		"wordy": {1, now}, "sp:brainstorm": {1, now},
	}
	dups := []Dup{{Kind: "identical", IDs: []string{"used", "copy"}, Keep: "used"}}
	got := map[string][]string{}
	for _, s := range Cleanup(tools, usage, dups, now) {
		for _, r := range s.Reasons {
			got[s.ID] = append(got[s.ID], r.Kind)
		}
	}
	want := map[string]string{"stale": "unused", "big": "heavy", "copy": "duplicate", "wordy": "heavy"}
	for id, k := range want {
		if strings.Join(got[id], ",") != k {
			t.Errorf("%s: got %v, want %s", id, got[id], k)
		}
	}
	for _, id := range []string{"used", "sp@m", "srv"} {
		if got[id] != nil {
			t.Errorf("%s should not be suggested: %v", id, got[id])
		}
	}
}
