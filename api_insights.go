package agentrig

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/insights"
)

func registerInsights(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	dir := filepath.Join(cfg, "agentrig")
	profiles := insights.Profiles{File: filepath.Join(dir, "profiles.json")}
	backups := filepath.Join(dir, "backup")

	decode := func(w http.ResponseWriter, r *http.Request, v any) bool {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return false
		}
		return true
	}
	fail := func(w http.ResponseWriter, err error) bool {
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		}
		return err != nil
	}

	mux.HandleFunc("GET /api/insights", func(w http.ResponseWriter, r *http.Request) {
		usage := insights.ClaudeUsage(filepath.Join(home, ".claude", "projects"), filepath.Join(dir, "usage-cache.json"))
		writeJSON(w, insights.Build(adapters.Detect(), usage, time.Now()))
	})
	mux.HandleFunc("GET /api/insights/duplicates", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, insights.Duplicates(adapters.Detect()))
	})
	// Bulk switch (disable unused, undo). Returns what was applied, so the UI
	// can undo by sending the same list back inverted.
	mux.HandleFunc("POST /api/insights/toggle", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Changes []insights.Change `json:"changes"`
		}
		if !decode(w, r, &req) {
			return
		}
		done, err := insights.Apply(req.Changes, adapters.Toggle)
		if fail(w, err) {
			return
		}
		writeJSON(w, done)
	})

	mux.HandleFunc("GET /api/insights/profiles", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, profiles.List())
	})
	mux.HandleFunc("POST /api/insights/profiles", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &req) || fail(w, profiles.Save(req.Name, adapters.Detect())) {
			return
		}
		writeJSON(w, profiles.List())
	})
	mux.HandleFunc("POST /api/insights/profiles/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &req) || fail(w, profiles.Delete(req.Name)) {
			return
		}
		writeJSON(w, profiles.List())
	})
	// apply with dryRun shows the diff; without it switches and returns what changed.
	mux.HandleFunc("POST /api/insights/profiles/apply", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name   string `json:"name"`
			DryRun bool   `json:"dryRun"`
		}
		if !decode(w, r, &req) {
			return
		}
		p, ok := profiles.Get(req.Name)
		if !ok {
			http.Error(w, "no such profile", http.StatusNotFound)
			return
		}
		changes := insights.Diff(p, adapters.Detect())
		if req.DryRun {
			writeJSON(w, changes)
			return
		}
		done, err := insights.Apply(changes, adapters.Toggle)
		if fail(w, err) {
			return
		}
		writeJSON(w, done)
	})

	mux.HandleFunc("GET /api/insights/backups", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, insights.Backups(backups))
	})
	mux.HandleFunc("POST /api/insights/backups/restore", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		}
		if !decode(w, r, &req) {
			return
		}
		undo, err := insights.Restore(backups, home, req.ID, req.Path)
		if fail(w, err) {
			return
		}
		writeJSON(w, map[string]string{"undo": undo})
	})
}
