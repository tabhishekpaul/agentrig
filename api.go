package agentrig

import (
	"encoding/json"
	"net/http"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/gitsync"
)

type syncStatus struct {
	gitsync.Config
	Phase string `json:"phase,omitempty"` // live progress of a running sync
}

// syncView is what the UI sees: the repo URL with any embedded token hidden.
func syncView(c gitsync.Config) gitsync.Config {
	c.Repo = gitsync.Redact(c.Repo)
	return c
}

func syncReply(w http.ResponseWriter, c gitsync.Config, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, syncView(c))
}

func registerAPI(mux *http.ServeMux, s *gitsync.Syncer) {
	registerInsights(mux)
	mux.HandleFunc("GET /api/sync", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, syncStatus{syncView(s.Status()), s.Phase()})
	})
	mux.HandleFunc("POST /api/sync/connect", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Repo string `json:"repo"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		c, err := s.Connect(req.Repo)
		syncReply(w, c, err)
	})
	mux.HandleFunc("GET /api/sync/providers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, gitsync.Providers())
	})
	mux.HandleFunc("POST /api/sync/setup", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider string `json:"provider"`
			Create   bool   `json:"create"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		c, err := s.Setup(req.Provider, req.Create)
		syncReply(w, c, err)
	})
	mux.HandleFunc("POST /api/sync/run", func(w http.ResponseWriter, r *http.Request) {
		c, err := s.Run()
		syncReply(w, c, err)
	})
	mux.HandleFunc("POST /api/sync/disconnect", func(w http.ResponseWriter, r *http.Request) {
		syncReply(w, gitsync.Config{}, s.Disconnect())
	})
	mux.HandleFunc("GET /api/tools", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, adapters.Detect())
	})
	mux.HandleFunc("POST /api/toggle", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tool    string `json:"tool"`
			Kind    string `json:"kind"`
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := adapters.Toggle(req.Tool, req.Kind, req.ID, req.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
