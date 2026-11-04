package agentrig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsAPI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // adapters read ~/.claude etc. from here, never the real one
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for name, desc := range map[string]string{"cutter": "Edit and cut video clips", "pdf-fill": "Fill pdf forms"} {
		d := filepath.Join(home, ".claude", "skills", name)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: "+desc+"\n---\n"), 0o644)
	}
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"x":{"env":{"KEY":"secret"}}}}`), 0o600)
	mux := http.NewServeMux()
	registerSkills(mux)
	get := func(path string) (int, string) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}

	_, body := get("/api/skills?q=video")
	var es []struct{ ID, Category string }
	json.Unmarshal([]byte(body), &es)
	if len(es) != 1 || es[0].ID != "cutter" || es[0].Category != "media/video" {
		t.Fatalf("search: %s", body)
	}
	if _, body := get("/api/skills?category=documents"); !strings.Contains(body, "pdf-fill") || strings.Contains(body, "cutter") {
		t.Fatalf("category: %s", body)
	}
	if code, _ := get("/api/skills?category=../x"); code != 400 {
		t.Fatalf("bad category: %d", code)
	}
	if _, body := get("/api/skills?q=zzzz"); body != "[]\n" {
		t.Fatalf("no hits should be []: %q", body)
	}
	if _, body := get("/api/categories"); !strings.Contains(body, `"id":"media","name":"Media","count":1`) {
		t.Fatalf("categories: %s", body)
	}
	if _, body := get("/api/mcp/clients"); !strings.Contains(body, `"tool":"claude"`) || strings.Contains(body, "secret") {
		t.Fatalf("clients: %s", body)
	}
}
