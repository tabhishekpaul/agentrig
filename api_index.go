package agentrig

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/insights"
	"github.com/tabhishekpaul/agentrig/skillindex"
)

// beforeIndexMode is the profile saved when index mode switches skills off,
// so switching it back on restores exactly what was on.
const beforeIndexMode = "Before index mode"

type indexView struct {
	skillindex.Settings
	Path      string `json:"path"`      // the index SKILL.md
	Skills    int    `json:"skills"`    // skills it lists
	Tokens    int    `json:"tokens"`    // per message now
	IndexOnly int    `json:"indexOnly"` // per message in index mode
}

func indexStatus(home string) indexView {
	tools := adapters.Detect()
	now, only := skillindex.Cost(tools)
	return indexView{Settings: skillindex.Load(), Path: filepath.Join(skillindex.Dir(home), "SKILL.md"),
		Skills: len(skillindex.Entries(tools)), Tokens: now, IndexOnly: only}
}

// indexLoop keeps the index in step with what's installed: tools, the
// marketplace, sync and hand edits all change skills.
func indexLoop(ctx context.Context, home string) {
	for {
		_, _ = skillindex.Sync(home, adapters.Detect())
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func registerIndex(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	profiles := insights.Profiles{File: filepath.Join(cfg, "agentrig", "profiles.json")}

	mux.HandleFunc("GET /api/index", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, indexStatus(home))
	})
	// {enabled} switches the index skill on/off; {indexMode} switches every
	// other skill off (saved as a profile first) or back to that profile.
	mux.HandleFunc("POST /api/index", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled   *bool `json:"enabled"`
			IndexMode *bool `json:"indexMode"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s := skillindex.Load()
		if req.Enabled != nil {
			s.Enabled = *req.Enabled
			if !s.Enabled {
				s.IndexMode = false // without the index, switched-off skills would be unreachable
			}
		}
		fail := func(err error) {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		}
		if req.IndexMode != nil && *req.IndexMode != s.IndexMode {
			tools := adapters.Detect()
			if *req.IndexMode {
				s.Enabled = true
				if err := profiles.Save(beforeIndexMode, tools); err != nil {
					fail(err)
					return
				}
				var off []insights.Change
				for _, t := range tools {
					for _, it := range t.Items {
						if it.Kind == "skill" && it.Enabled && it.Toggleable && it.ID != skillindex.Name {
							off = append(off, insights.Change{Tool: t.ID, Kind: "skill", ID: it.ID, Enabled: false})
						}
					}
				}
				if _, err := insights.Apply(off, adapters.Toggle); err != nil {
					fail(err)
					return
				}
			} else if p, ok := profiles.Get(beforeIndexMode); ok {
				if _, err := insights.Apply(insights.Diff(p, tools), adapters.Toggle); err != nil {
					fail(err)
					return
				}
			}
			s.IndexMode = *req.IndexMode
		}
		if err := skillindex.Save(s); err != nil {
			fail(err)
			return
		}
		if _, err := skillindex.Sync(home, adapters.Detect()); err != nil {
			fail(err)
			return
		}
		writeJSON(w, indexStatus(home))
	})
}
