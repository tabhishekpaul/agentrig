package main

import (
	"encoding/json"
	"net/http"

	"github.com/tabhishekpaul/agentrig/adapters"
)

func registerAPI(mux *http.ServeMux) {
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
