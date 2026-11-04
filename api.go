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
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
