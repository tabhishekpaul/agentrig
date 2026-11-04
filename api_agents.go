package agentrig

import (
	"encoding/json"
	"net/http"

	"github.com/tabhishekpaul/agentrig/adapters"
)

// registerAgents serves the "arig agent" toggles on the AI access page.
func registerAgents(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, adapters.AgentStatus())
	})
	mux.HandleFunc("POST /api/agents", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tool    string `json:"tool"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := adapters.SetAgent(req.Tool, req.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}
