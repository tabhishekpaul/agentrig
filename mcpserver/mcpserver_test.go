package mcpserver

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/catalog"
)

// client drives a Server through in-memory pipes, like an AI tool over stdio.
type client struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Reader
	id  int
}

func start(t *testing.T, entries []catalog.Entry) *client { return startRig(t, entries, nil) }

func startRig(t *testing.T, entries []catalog.Entry, rig *Rig) *client {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &Server{Entries: func() []catalog.Entry { return entries }, Version: "test", Rig: rig}
	go func() { s.Serve(inR, outW); outW.Close() }()
	t.Cleanup(func() { inW.Close() })
	return &client{t: t, in: inW, out: bufio.NewReader(outR)}
}

func (c *client) send(line string) map[string]any {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatal(err)
	}
	b, err := c.out.ReadBytes('\n')
	if err != nil {
		c.t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		c.t.Fatalf("not json: %s", b)
	}
	return m
}

func (c *client) call(method string, params any) map[string]any {
	c.id++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	return c.send(string(b))
}

// tool calls a tool and returns its text and isError.
func (c *client) tool(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	r := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	res, ok := r["result"].(map[string]any)
	if !ok {
		c.t.Fatalf("%s: no result: %v", name, r)
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	return text, res["isError"] == true
}

func fixture(t *testing.T) []catalog.Entry {
	dir := t.TempDir()
	skill := filepath.Join(dir, "video-edit")
	os.MkdirAll(filepath.Join(skill, "scripts"), 0o755)
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: video-edit\n---\nCut clips with ffmpeg.\n"), 0o644)
	os.WriteFile(filepath.Join(skill, "scripts", "cut.sh"), []byte("ffmpeg -i in"), 0o644)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("nope"), 0o644)
	os.Symlink(filepath.Join(dir, "secret.txt"), filepath.Join(skill, "escape"))
	// the skill folder itself is reached through a symlink, as installers do
	os.Symlink(skill, filepath.Join(dir, "linked"))
	return []catalog.Entry{
		{ID: "video-edit", Name: "video-edit", Description: "Edit and cut video clips", Category: "media/video", Path: filepath.Join(dir, "linked")},
		{ID: "pdf", Name: "pdf", Description: "Fill PDF forms", Category: "documents/pdf", Path: dir},
	}
}

func TestProtocol(t *testing.T) {
	c := start(t, fixture(t))
	r := c.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}})
	if res := r["result"].(map[string]any); res["protocolVersion"] != "2025-03-26" || res["serverInfo"].(map[string]any)["name"] != "arig" {
		t.Fatalf("initialize: %v", r)
	}
	// a notification gets no reply: the next line read must answer the ping
	io.WriteString(c.in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	if r := c.call("ping", nil); r["id"].(float64) != 2 || r["error"] != nil {
		t.Fatalf("ping: %v", r)
	}
	r = c.call("tools/list", nil)
	var names []string
	for _, tl := range r["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tl.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "search_skills,get_skill,list_categories,rig_overview,marketplace_search,marketplace_skill,install_skill,set_skill_enabled,remove_skill" {
		t.Fatalf("tools: %v", names)
	}
	if r := c.call("nope", nil); r["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Fatalf("unknown method: %v", r)
	}
	if r := c.send("{bad"); r["error"].(map[string]any)["code"].(float64) != -32700 {
		t.Fatalf("parse error: %v", r)
	}
}

func TestTools(t *testing.T) {
	c := start(t, fixture(t))
	text, isErr := c.tool("search_skills", map[string]any{"query": "cut a video"})
	if isErr || !strings.HasPrefix(text, "1. video-edit (id: video-edit) [media/video]") || !strings.Contains(text, `get_skill {"id": "video-edit"}`) {
		t.Fatalf("search: %q", text)
	}
	if text, _ := c.tool("search_skills", map[string]any{"category": "documents"}); !strings.Contains(text, "pdf") || strings.Contains(text, "video") {
		t.Fatalf("category browse: %q", text)
	}
	if _, isErr := c.tool("search_skills", map[string]any{}); !isErr {
		t.Fatal("empty search should be a tool error")
	}
	if text, _ := c.tool("list_categories", nil); !strings.Contains(text, "media (1): media/video 1") {
		t.Fatalf("categories: %q", text)
	}

	text, isErr = c.tool("get_skill", map[string]any{"id": "video-edit"})
	if isErr || !strings.Contains(text, "Cut clips with ffmpeg.") || !strings.Contains(text, "- scripts/cut.sh") {
		t.Fatalf("get: %q", text)
	}
	if text, isErr := c.tool("get_skill", map[string]any{"id": "video-edit", "file": "scripts/cut.sh"}); isErr || text != "ffmpeg -i in" {
		t.Fatalf("get file: %q", text)
	}
	for _, bad := range []string{"../secret.txt", "/etc/passwd", "escape", "scripts/../../secret.txt", "scripts"} {
		if text, isErr := c.tool("get_skill", map[string]any{"id": "video-edit", "file": bad}); !isErr {
			t.Fatalf("read %q outside the skill: %q", bad, text)
		}
	}
	if _, isErr := c.tool("get_skill", map[string]any{"id": "../video-edit"}); !isErr {
		t.Fatal("unknown id should be a tool error")
	}
}
