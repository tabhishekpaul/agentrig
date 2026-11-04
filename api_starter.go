package agentrig

import (
	"net/http"
	"slices"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/market"
	"github.com/tabhishekpaul/agentrig/starter"
)

type starterRow struct {
	starter.Entry
	Installed bool     `json:"installed"`
	Targets   []string `json:"targets"` // tools it would go into
}

func skillTools() []string {
	var out []string
	for _, t := range adapters.Detect() {
		if t.Installed && adapters.SkillDir(t.ID) != "" {
			out = append(out, t.ID)
		}
	}
	return out
}

func pluginInstalled(tool, plugin string) bool {
	for _, t := range adapters.Detect() {
		if t.ID == tool {
			return slices.ContainsFunc(t.Items, func(i adapters.Item) bool { return i.Kind == "plugin" && i.ID == plugin })
		}
	}
	return false
}

// registerStarter serves the first-launch pack. Nothing installs until the
// user presses Install on /welcome.
func registerStarter(mux *http.ServeMux, m *market.Market) {
	in := starter.New(m, skillTools, pluginInstalled)

	mux.HandleFunc("GET /api/starter", func(w http.ResponseWriter, r *http.Request) {
		tools := map[string]string{}
		for _, t := range adapters.Detect() {
			if t.Installed {
				tools[t.ID] = t.Name
			}
		}
		lock, st := m.Lock(), skillTools()
		var rows []starterRow
		for _, e := range starter.Entries() {
			row := starterRow{}
			if e.Kind == "plugin" {
				row.Targets = in.PluginTools(e)
				row.Installed = len(row.Targets) > 0 && !slices.ContainsFunc(row.Targets, func(t string) bool { return !pluginInstalled(t, e.Plugin) })
			} else {
				row.Targets = in.Missing(e.ID, st)
				_, inLock := lock[e.ID]
				row.Installed = inLock || (len(row.Targets) == 0 && len(st) > 0)
			}
			e.Install = nil // commands stay server-side
			row.Entry = e
			rows = append(rows, row)
		}
		writeJSON(w, map[string]any{"entries": rows, "tools": tools, "onboarded": starter.Onboarded()})
	})
	mux.HandleFunc("GET /api/starter/progress", func(w http.ResponseWriter, r *http.Request) {
		rows, running := in.Progress()
		writeJSON(w, map[string]any{"rows": rows, "running": running, "onboarded": starter.Onboarded()})
	})
	mux.HandleFunc("POST /api/starter/install", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs    []string `json:"ids"`
			Accept []string `json:"accept"`
		}
		if !decode(w, r, 64<<10, &req) {
			return
		}
		if err := in.Start(req.IDs, req.Accept); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/starter/done", func(w http.ResponseWriter, r *http.Request) {
		if err := starter.MarkOnboarded(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}
