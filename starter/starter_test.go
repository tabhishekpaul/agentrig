package starter

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/market"
)

// TestMain keeps every test away from the real home folder.
func TestMain(m *testing.M) {
	home, _ := os.MkdirTemp("", "arig-starter-")
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", "")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func TestEmbeddedList(t *testing.T) {
	es := Entries()
	covered := map[string]bool{}
	ids := map[string]bool{}
	for _, e := range es {
		if ids[e.ID] {
			t.Errorf("duplicate id %s", e.ID)
		}
		ids[e.ID] = true
		if !catalog.Valid(e.Category) || !strings.Contains(e.Category, "/") {
			t.Errorf("%s: bad category %q", e.ID, e.Category)
		}
		covered[e.Category] = true
		if e.Name == "" || e.Why == "" {
			t.Errorf("%s: needs a name and why", e.ID)
		}
		for _, f := range e.Flags {
			if f == "shell-pipe" || f == "binary" {
				t.Errorf("%s: starter skills must not carry %s", e.ID, f)
			}
		}
		switch e.Kind {
		case "skill":
			if _, _, _, err := market.SplitID(e.ID); err != nil {
				t.Error(err)
			}
		case "plugin":
			if e.Plugin == "" || len(e.Install) == 0 {
				t.Errorf("%s: a plugin needs its id and install commands", e.ID)
			}
		default:
			t.Errorf("%s: kind %q", e.ID, e.Kind)
		}
	}
	for _, c := range catalog.Categories {
		for _, s := range c.Subs {
			// agents/memory: no reputable, maintained memory skill on skills.sh yet
			if !covered[c.ID+"/"+s] && c.ID+"/"+s != "agents/memory" {
				t.Errorf("no starter pick for %s/%s", c.ID, s)
			}
		}
	}
}

func TestMarker(t *testing.T) {
	if Onboarded() {
		t.Fatal("fresh home is onboarded")
	}
	if err := MarkOnboarded(); err != nil {
		t.Fatal(err)
	}
	if !Onboarded() {
		t.Fatal("marker not written")
	}
}

func skillDL(extra ...market.File) market.Download {
	return market.Download{Hash: "abc123", Files: append([]market.File{{Path: "SKILL.md", Contents: "---\nname: x\n---\nhi"}}, extra...)}
}

func TestInstall(t *testing.T) {
	script := market.File{Path: "run.sh", Contents: "echo hi"}
	dls := map[string]market.Download{
		"a/b/plain":    skillDL(),
		"a/b/flagged":  skillDL(script),
		"a/b/declined": skillDL(script),
		"a/b/changed":  skillDL(market.File{Path: "run.sh", Contents: "curl https://x"}),
		"a/b/have":     skillDL(),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strings.CutPrefix(r.URL.Path, "/api/download/")
		d, ok := dls[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(d)
	}))
	defer srv.Close()

	test := []Entry{
		{ID: "a/b/plain", Kind: "skill"},
		{ID: "a/b/flagged", Kind: "skill", Flags: []string{"scripts"}},
		{ID: "a/b/declined", Kind: "skill", Flags: []string{"scripts"}},
		{ID: "a/b/changed", Kind: "skill", Flags: []string{"scripts"}},
		{ID: "a/b/have", Kind: "skill"},
		{ID: "plug", Kind: "plugin", Plugin: "plug@plug", Install: map[string][][]string{
			"claude": {{"claude", "plugin", "marketplace", "add", "o/r"}, {"claude", "plugin", "install", "plug@plug"}},
			"codex":  {{"codex", "plugin", "add", "plug@plug"}},
		}},
	}
	saved := entries
	entries = func() []Entry { return test }
	defer func() { entries = saved }()

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := func(tool string) string { return filepath.Join(home, tool, "skills") }
	// "have" already sits in tool one: it only goes into tool two.
	os.MkdirAll(filepath.Join(dir("one"), "have"), 0o755)

	var mu sync.Mutex
	var ran [][]string
	in := &Installer{
		Market:          &market.Market{Dir: filepath.Join(home, "cfg"), Base: srv.URL, HTTP: srv.Client(), SkillDir: dir},
		SkillTools:      func() []string { return []string{"one", "two"} },
		PluginInstalled: func(tool, plugin string) bool { return false },
		LookPath: func(b string) (string, error) {
			if b == "claude" {
				return "/bin/claude", nil
			}
			return "", errors.New("not found")
		},
		Run: func(argv []string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			ran = append(ran, argv)
			if argv[2] == "marketplace" {
				return "Marketplace already installed", errors.New("exit 1")
			}
			return "ok", nil
		},
	}
	if err := in.Start([]string{"nope"}, nil); err == nil {
		t.Fatal("unknown id accepted")
	}
	ids := []string{"a/b/plain", "a/b/flagged", "a/b/declined", "a/b/changed", "a/b/have", "plug"}
	if err := in.Start(ids, []string{"a/b/flagged", "a/b/changed"}); err != nil {
		t.Fatal(err)
	}
	if err := in.Start(ids, nil); err == nil {
		t.Error("second run started while the first was going")
	}
	var rows map[string]Row
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var running bool
		if rows, running = in.Progress(); !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("install never finished")
		}
	}
	want := map[string]string{"a/b/plain": "done", "a/b/flagged": "done", "a/b/declined": "failed", "a/b/changed": "failed", "a/b/have": "done", "plug": "done"}
	for id, st := range want {
		if rows[id].State != st {
			t.Errorf("%s: %+v, want %s", id, rows[id], st)
		}
	}
	if !strings.Contains(rows["a/b/changed"].Reason, "network") {
		t.Errorf("changed: %q", rows["a/b/changed"].Reason)
	}
	for _, p := range []string{"one/skills/plain", "two/skills/plain", "two/skills/flagged", "two/skills/have"} {
		if _, err := os.Stat(filepath.Join(home, p, "SKILL.md")); err != nil {
			t.Errorf("%s not installed: %v", p, err)
		}
	}
	for _, p := range []string{"two/skills/declined", "two/skills/changed"} {
		if _, err := os.Lstat(filepath.Join(home, p)); err == nil {
			t.Errorf("%s installed without consent", p)
		}
	}
	if !slices.Equal(in.Market.Lock()["a/b/have"].Tools, []string{"two"}) {
		t.Errorf("have: %+v", in.Market.Lock()["a/b/have"])
	}
	if len(ran) != 2 || ran[0][0] != "/bin/claude" || ran[1][3] != "plug@plug" {
		t.Errorf("plugin commands: %q", ran)
	}
}
