package agentrig

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/authoring"
	"github.com/tabhishekpaul/agentrig/catalog"
)

// installedSkill returns the installed skill with id, if any.
func installedSkill(id string) (catalog.Entry, bool) {
	for _, e := range catalog.Local(adapters.Detect()) {
		if e.ID == id {
			return e, true
		}
	}
	return catalog.Entry{}, false
}

func skillPath(id string) string {
	e, _ := installedSkill(id)
	return e.Path
}

func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, v)
}

var errNoSkill = errors.New("no installed skill with that id")

func registerAuthoring(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	packs := &authoring.Packs{Path: filepath.Join(cfg, "agentrig", "packs.json")}

	// What the create form needs: categories, tools that read skills, the drafting CLI.
	mux.HandleFunc("GET /api/authoring", func(w http.ResponseWriter, r *http.Request) {
		type tool struct{ ID, Name string }
		dirs := adapters.SkillDirs()
		var tools []tool
		for _, t := range adapters.Detect() {
			if _, ok := dirs[t.ID]; ok && t.Installed {
				tools = append(tools, tool{t.ID, t.Name})
			}
		}
		writeJSON(w, map[string]any{"categories": catalog.Categories, "tools": tools,
			"drafter": authoring.Drafter(), "skills": catalog.Local(adapters.Detect())})
	})
	mux.HandleFunc("POST /api/authoring/skills", func(w http.ResponseWriter, r *http.Request) {
		var s authoring.Skill
		if !decode(w, r, 1<<20, &s) {
			return
		}
		dir, err := authoring.Create(home, adapters.SkillDirs(), s)
		reply(w, map[string]string{"path": dir}, err)
	})
	mux.HandleFunc("GET /api/authoring/skills/{id}", func(w http.ResponseWriter, r *http.Request) {
		p := skillPath(r.PathValue("id"))
		if p == "" {
			reply(w, nil, errNoSkill)
			return
		}
		b, err := os.ReadFile(filepath.Join(p, "SKILL.md"))
		reply(w, map[string]string{"path": p, "content": string(b)}, err)
	})
	mux.HandleFunc("PUT /api/authoring/skills/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Content string }
		if !decode(w, r, 1<<20, &req) {
			return
		}
		p := skillPath(r.PathValue("id")) // only folders arig itself found
		if p == "" {
			reply(w, nil, errNoSkill)
			return
		}
		reply(w, map[string]bool{"ok": true}, os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(req.Content), 0o644))
	})
	mux.HandleFunc("POST /api/authoring/draft", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name, Description string }
		if !decode(w, r, 8192, &req) {
			return
		}
		text, err := authoring.Draft(req.Name, req.Description)
		reply(w, map[string]string{"text": text}, err)
	})

	mux.HandleFunc("GET /api/authoring/packs", func(w http.ResponseWriter, r *http.Request) {
		ps, err := packs.List()
		reply(w, ps, err)
	})
	mux.HandleFunc("PUT /api/authoring/packs", func(w http.ResponseWriter, r *http.Request) {
		var p catalog.Pack
		if !decode(w, r, 1<<16, &p) {
			return
		}
		reply(w, map[string]bool{"ok": true}, packs.Save(p))
	})
	mux.HandleFunc("DELETE /api/authoring/packs/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]bool{"ok": true}, packs.Delete(r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/authoring/packs/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Enabled bool }
		if !decode(w, r, 4096, &req) {
			return
		}
		p, err := packs.Get(r.PathValue("id"))
		if err != nil {
			reply(w, nil, err)
			return
		}
		n, err := authoring.TogglePack(p, adapters.SkillDirs(), req.Enabled)
		reply(w, map[string]int{"changed": n}, err)
	})
	mux.HandleFunc("POST /api/authoring/packs/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Dir string }
		if !decode(w, r, 4096, &req) {
			return
		}
		p, err := packs.Get(r.PathValue("id"))
		if err != nil {
			reply(w, nil, err)
			return
		}
		dir, err := authoring.ExportDir(home, req.Dir)
		if err == nil {
			err = authoring.WritePlugin(dir, p, authoring.GitUser(), skillPath)
		}
		reply(w, map[string]string{"path": dir}, err)
	})

	// dry: true returns the plan for the confirm dialog; false publishes.
	mux.HandleFunc("POST /api/authoring/publish", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Kind, ID string
			Dry      bool
		}
		if !decode(w, r, 4096, &req) {
			return
		}
		it := authoring.Item{Kind: req.Kind, ID: req.ID, SkillPath: skillPath}
		if req.Kind == "pack" {
			p, err := packs.Get(req.ID)
			if err != nil {
				reply(w, nil, err)
				return
			}
			it.Pack = p
		} else {
			e, ok := installedSkill(req.ID)
			if !ok {
				reply(w, nil, errNoSkill)
				return
			}
			it.Path, it.Description = e.Path, e.Description
		}
		plan, dir, err := authoring.Prepare(it)
		if err != nil {
			reply(w, nil, err)
			return
		}
		defer os.RemoveAll(dir)
		if req.Dry {
			writeJSON(w, plan)
			return
		}
		url, err := authoring.Publish(plan, dir)
		reply(w, map[string]string{"url": url}, err)
	})
}
