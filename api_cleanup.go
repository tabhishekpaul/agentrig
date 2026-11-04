package agentrig

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/insights"
	"github.com/tabhishekpaul/agentrig/market"
)

const maxAltChecks = 40 // skills.sh lookups per "check for better versions"

func registerCleanup(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	dir := filepath.Join(cfg, "agentrig")
	m := market.New(Version)

	mux.HandleFunc("GET /api/cleanup", func(w http.ResponseWriter, r *http.Request) {
		tools := adapters.Detect()
		usage := insights.ClaudeUsage(filepath.Join(home, ".claude", "projects"), filepath.Join(dir, "usage-cache.json"))
		writeJSON(w, insights.Cleanup(tools, usage, insights.Duplicates(tools), time.Now()))
	})

	// Better versions: for each local skill id, a popular skills.sh skill of the
	// same name whose SKILL.md differs from yours.
	mux.HandleFunc("POST /api/cleanup/alternatives", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		paths := map[string]string{}
		for _, t := range adapters.Detect() {
			for _, it := range t.Items {
				if it.Kind == "skill" && paths[it.ID] == "" {
					paths[it.ID] = it.Path
				}
			}
		}
		type alt struct {
			market.Skill
			URL string `json:"url"`
		}
		out := map[string]alt{}
		checked := 0
		for _, id := range req.IDs {
			p, ok := paths[id] // only skills that are really installed here
			if !ok || checked >= maxAltChecks {
				continue
			}
			checked++
			local, _ := os.ReadFile(filepath.Join(p, "SKILL.md"))
			a, err := m.Alternative(id, string(local))
			if err != nil || a == nil {
				continue // offline or nothing better: just no suggestion
			}
			out[id] = alt{*a, "https://skills.sh/" + a.ID}
		}
		writeJSON(w, out)
	})
}
