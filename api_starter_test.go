package agentrig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabhishekpaul/agentrig/market"
)

func TestStarterAPI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", "")                              // no claude/codex CLI: plugins have no targets
	srv := httptest.NewServer(http.NotFoundHandler()) // stands in for skills.sh; nothing should call it
	defer srv.Close()
	t.Setenv("ARIG_SKILLS_URL", srv.URL)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	mux := http.NewServeMux()
	registerStarter(mux, market.New("dev"))
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	var got struct {
		Entries []struct {
			ID, Kind  string
			Installed bool
			Targets   []string
			Install   any
		}
		Onboarded bool
	}
	w := do("GET", "/api/starter", "")
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got.Entries) == 0 || got.Onboarded {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, e := range got.Entries {
		if e.Install != nil {
			t.Errorf("%s: install commands leaked to the UI", e.ID)
		}
		if want := e.Kind == "skill"; want != (len(e.Targets) == 1 && e.Targets[0] == "claude") {
			t.Errorf("%s: targets %v", e.ID, e.Targets)
		}
	}
	if w := do("POST", "/api/starter/install", `{"ids":["not/in/pack"]}`); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown id: %d", w.Code)
	}
	if w := do("POST", "/api/starter/done", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := os.Stat(filepath.Join(home, ".arig", "onboarded")); err != nil {
		t.Fatal("no marker:", err)
	}
	if !strings.Contains(do("GET", "/api/starter/progress", "").Body.String(), `"onboarded":true`) {
		t.Error("progress doesn't report onboarded")
	}
}
