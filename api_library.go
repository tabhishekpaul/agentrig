package agentrig

import (
	"errors"
	"net/http"
	"os"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/library"
)

func registerLibrary(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	mux.HandleFunc("GET /api/library", func(w http.ResponseWriter, r *http.Request) {
		var dirs []string
		for _, ds := range adapters.SkillDirs() {
			dirs = append(dirs, ds...)
		}
		writeJSON(w, library.Summarize(home, dirs))
	})
	// Undo the latest migration: links become real folders again.
	mux.HandleFunc("POST /api/library/undo", func(w http.ResponseWriter, r *http.Request) {
		last := library.LastLog(home)
		if last == nil {
			reply(w, nil, errors.New("no migration to undo"))
			return
		}
		n, err := library.Undo(last.Path)
		if err != nil && n == 0 {
			reply(w, nil, err)
			return
		}
		res := map[string]any{"restored": n}
		if err != nil {
			res["error"] = err.Error() // partial
		}
		writeJSON(w, res)
	})
}
