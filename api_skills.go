package agentrig

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
)

// arigPath is the absolute path of this binary, which tools launch as `arig mcp`.
func arigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "arig"
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		return p
	}
	return exe
}

func registerSkills(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/skills", func(w http.ResponseWriter, r *http.Request) {
		cat := r.URL.Query().Get("category")
		if cat != "" && !catalog.Valid(cat) {
			http.Error(w, "unknown category", http.StatusBadRequest)
			return
		}
		es := catalog.InCategory(catalog.Local(adapters.Detect()), cat)
		if q := r.URL.Query().Get("q"); q != "" {
			es = catalog.Search(es, q, 0)
		}
		if es == nil {
			es = []catalog.Entry{}
		}
		writeJSON(w, es)
	})
	mux.HandleFunc("GET /api/categories", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, catalog.Counts(catalog.Local(adapters.Detect())))
	})
	mux.HandleFunc("GET /api/mcp/clients", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, adapters.MCPClients(arigPath()))
	})
	mux.HandleFunc("POST /api/mcp/clients", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tool    string `json:"tool"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := adapters.SetMCP(req.Tool, arigPath(), req.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}
