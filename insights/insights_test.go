package insights

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
)

func TestParseTranscript(t *testing.T) {
	long := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"huge"}}]},"x":"` + strings.Repeat("a", maxLine) + `"}`
	in := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-10-01T10:00:00Z","message":{"content":[{"type":"text","text":"hi"},{"type":"tool_use","name":"Skill","input":{"skill":"pdf"}}]}}`,
		`{"type":"assistant","timestamp":"2026-10-03T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"pdf"}}]}}`,
		`{"type":"user","timestamp":"2026-10-02T10:00:00Z","message":{"content":"<command-name>/superpowers:brainstorming</command-name>\n<command-args></command-args>"}}`,
		`{"type":"user","timestamp":"2026-10-02T10:00:00Z","message":{"content":"talking about <command-name>/pdf</command-name> mid-text"}}`,
		long,
		`not json`,
		`{"type":"assistant","timestamp":"2026-10-04T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Bash","input":{"skill":"nope"}}]}}`,
	}, "\n")
	got := parseTranscript(strings.NewReader(in))
	if got["pdf"].Count != 2 || !got["pdf"].Last.Equal(time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("pdf = %+v", got["pdf"])
	}
	if got["superpowers:brainstorming"].Count != 1 {
		t.Errorf("slash command not counted: %+v", got)
	}
	if _, ok := got["huge"]; ok {
		t.Error("over-long line should be skipped")
	}
	if len(got) != 2 {
		t.Errorf("unexpected names: %+v", got)
	}
}

func TestClaudeUsageCache(t *testing.T) {
	dir, cache := t.TempDir(), filepath.Join(t.TempDir(), "c.json")
	f := filepath.Join(dir, "proj", "s.jsonl")
	os.MkdirAll(filepath.Dir(f), 0o755)
	line := `{"type":"assistant","timestamp":"2026-10-01T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"pdf"}}]}}` + "\n"
	os.WriteFile(f, []byte(line), 0o644)
	if ClaudeUsage(dir, cache)["pdf"].Count != 1 {
		t.Fatal("first scan")
	}
	if b, _ := os.ReadFile(cache); strings.Contains(string(b), "tool_use") {
		t.Fatal("cache must not hold transcript text")
	}
	os.WriteFile(f, []byte(line+line), 0o644) // grown: re-read
	if n := ClaudeUsage(dir, cache)["pdf"].Count; n != 2 {
		t.Fatalf("after growth = %d", n)
	}
	old := time.Now().Add(-100 * 24 * time.Hour)
	os.Chtimes(f, old, old)
	if len(ClaudeUsage(dir, cache)) != 0 {
		t.Fatal("transcripts older than 90 days must be ignored")
	}
}

func TestBuildCostAndUnused(t *testing.T) {
	now := time.Now()
	tools := []adapters.Tool{
		{ID: "claude", Name: "Claude Code", Items: []adapters.Item{
			{Kind: "skill", ID: "a", Name: "a", Description: strings.Repeat("x", 39), Enabled: true, Toggleable: true},  // 10 tokens
			{Kind: "skill", ID: "b", Name: "b", Description: strings.Repeat("x", 399), Enabled: true, Toggleable: true}, // 100 tokens
			{Kind: "skill", ID: "c", Name: "c", Description: "off", Enabled: false, Toggleable: true},
			{Kind: "plugin", ID: "sp@market", Enabled: true},
		}},
		{ID: "codex", Name: "Codex", Items: []adapters.Item{
			{Kind: "skill", ID: "a", Name: "a", Description: strings.Repeat("x", 39), Enabled: true, Toggleable: true},
		}},
	}
	usage := map[string]Use{"a": {1, now.Add(-time.Hour)}, "sp:go": {3, now}, "clear": {9, now}}
	ov := Build(tools, usage, now)
	if ov.Enabled != 3 || ov.Tokens != 120 {
		t.Fatalf("totals = %d skills, %d tokens", ov.Enabled, ov.Tokens)
	}
	c := ov.Tools[0]
	if c.Tool != "claude" || c.Tokens != 110 || c.Skills[0].ID != "b" {
		t.Fatalf("claude cost = %+v", c)
	}
	if len(c.Unused) != 1 || c.Unused[0] != "b" {
		t.Errorf("unused = %v", c.Unused)
	}
	if ov.Tools[1].UsageKnown || ov.Tools[1].Unused != nil {
		t.Error("codex usage must be unknown")
	}
	if _, ok := ov.Usage["clear"]; ok || ov.Usage["sp:go"].Count != 3 {
		t.Errorf("usage filter = %+v", ov.Usage)
	}
}

func skill(t *testing.T, root, id, body string) adapters.Item {
	d := filepath.Join(root, id)
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(body), 0o644)
	fm := adapters.Frontmatter(filepath.Join(d, "SKILL.md"))
	return adapters.Item{Kind: "skill", ID: id, Name: fm["name"], Description: fm["description"], Path: d, Enabled: true}
}

func TestDuplicates(t *testing.T) {
	r := t.TempDir()
	a := skill(t, r, "pdf", "---\nname: pdf\ndescription: Read PDFs\n---\nbody text\n")
	b := skill(t, r, "pdf-copy", "---\nname:   pdf\ndescription: Read PDFs\n---\n\n  body   text")
	c := skill(t, r, "pdf2", "---\nname: other\ndescription: Read PDFs\n---\ndifferent\n")
	d := skill(t, r, "solo", "---\nname: solo\ndescription: alone\n---\n")
	tools := []adapters.Tool{
		{ID: "claude", Items: []adapters.Item{a, b, c, d}},
		{ID: "codex", Items: []adapters.Item{a}},
	}
	got := map[string][]string{}
	for _, d := range Duplicates(tools) {
		got[d.Kind] = append(got[d.Kind], strings.Join(sortedCopy(d.IDs), ","))
	}
	if strings.Join(got["shared"], ";") != "pdf" {
		t.Errorf("shared = %v", got["shared"])
	}
	if strings.Join(got["identical"], ";") != "pdf,pdf-copy" {
		t.Errorf("identical = %v", got["identical"])
	}
	// same name group == identical group, so only the description group is new
	if strings.Join(got["similar"], ";") != "pdf,pdf-copy,pdf2" {
		t.Errorf("similar = %v", got["similar"])
	}
}

func TestProfileDiffAndApply(t *testing.T) {
	tools := []adapters.Tool{{ID: "claude", Items: []adapters.Item{
		{Kind: "skill", ID: "a", Enabled: true, Toggleable: true},
		{Kind: "skill", ID: "b", Enabled: false, Toggleable: true},
		{Kind: "plugin", ID: "p@m", Enabled: true, Toggleable: true},
		{Kind: "mcp", ID: "m", Enabled: true},
	}}}
	p := Profile{State: map[string]map[string]bool{"claude": {
		"skill:a": true, "skill:b": true, "plugin:p@m": false, "skill:gone": true,
	}}}
	diff := Diff(p, tools)
	want := []Change{{"claude", "plugin", "p@m", false}, {"claude", "skill", "b", true}}
	if len(diff) != 2 || diff[0] != want[0] || diff[1] != want[1] {
		t.Fatalf("diff = %+v", diff)
	}
	var calls int
	done, err := Apply(diff, func(tool, kind, id string, on bool) error {
		if calls++; calls == 2 {
			return errors.New("boom")
		}
		return nil
	})
	if err == nil || len(done) != 1 || done[0] != want[0] {
		t.Fatalf("apply = %+v, %v", done, err)
	}

	store := Profiles{File: filepath.Join(t.TempDir(), "profiles.json")}
	if err := store.Save("work", tools); err != nil {
		t.Fatal(err)
	}
	if got, ok := store.Get("work"); !ok || len(Diff(got, tools)) != 0 {
		t.Fatal("saved profile should match current state")
	}
	store.Delete("work")
	if len(store.List()) != 0 {
		t.Fatal("delete")
	}
}

func TestRestoreConfinedAndUndoable(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	id := "2026-11-01T10-00-00"
	item := filepath.Join(".claude", "skills", "pdf")
	os.MkdirAll(filepath.Join(dir, id, item), 0o755)
	os.WriteFile(filepath.Join(dir, id, item, "SKILL.md"), []byte("old"), 0o644)
	os.MkdirAll(filepath.Join(home, item), 0o755)
	os.WriteFile(filepath.Join(home, item, "SKILL.md"), []byte("new"), 0o644)
	os.WriteFile(filepath.Join(home, item, "extra.md"), []byte("x"), 0o644)

	for _, bad := range [][2]string{
		{id, "../../etc/passwd"},
		{id, "/etc/passwd"},
		{id, ".ssh/id_rsa"},
		{id, ".claude/skills/../../.ssh"},
		{"../x", ".claude/skills/pdf"},
		{id, ".claude/skills"},
	} {
		if _, err := Restore(dir, home, bad[0], bad[1]); err == nil {
			t.Errorf("Restore(%q, %q) should fail", bad[0], bad[1])
		}
	}

	bs := Backups(dir)
	if len(bs) != 1 || bs[0].Items[0].Path != ".claude/skills/pdf" || bs[0].Items[0].Files != 1 {
		t.Fatalf("backups = %+v", bs)
	}
	undo, err := Restore(dir, home, id, ".claude/skills/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, item, "SKILL.md")); string(b) != "old" {
		t.Errorf("restored = %q", b)
	}
	if _, err := os.Stat(filepath.Join(home, item, "extra.md")); err == nil {
		t.Error("restore should replace the folder, not merge into it")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, undo, item, "SKILL.md")); string(b) != "new" {
		t.Errorf("undo backup = %q", b)
	}
	if len(Backups(dir)) != 2 {
		t.Error("undo backup should be listed")
	}
}

func TestUsageCountsSkillReads(t *testing.T) {
	lines := `{"type":"assistant","timestamp":"2026-11-01T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/Users/x/.arig/library/frontend-design/c8a1301e066b/SKILL.md"}}]}}
{"type":"assistant","timestamp":"2026-11-02T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/Users/x/.claude/skills/pdf/SKILL.md"}}]}}
{"type":"assistant","timestamp":"2026-11-02T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/Users/x/.arig/index/skills-index/SKILL.md"}}]}}
{"type":"assistant","timestamp":"2026-11-02T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/Users/x/notes/README.md"}}]}}
`
	got := parseTranscript(strings.NewReader(lines))
	if got["frontend-design"].Count != 1 || got["pdf"].Count != 1 || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
