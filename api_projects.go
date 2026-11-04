package agentrig

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tabhishekpaul/agentrig/adapters"
	"github.com/tabhishekpaul/agentrig/catalog"
	"github.com/tabhishekpaul/agentrig/projects"
)

// underHome accepts only an existing absolute folder inside the user's home
// (symlinks resolved), and never home itself.
func underHome(p string) (string, error) {
	home, _ := os.UserHomeDir()
	if !filepath.IsAbs(p) {
		return "", errors.New("use an absolute path")
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(p))
	if err != nil {
		return "", errors.New("folder not found")
	}
	if h, err := filepath.EvalSymlinks(home); err == nil {
		home = h
	}
	rel, err := filepath.Rel(home, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("projects must be inside your home folder")
	}
	if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
		return "", errors.New("not a folder")
	}
	return real, nil
}

// project resolves ?path= to a registered project, so the API only ever
// writes into folders the user added.
func project(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := r.URL.Query().Get("path")
	if !projects.Known(p) {
		http.Error(w, "unknown project", http.StatusNotFound)
		return "", false
	}
	return p, true
}

func linksReply(w http.ResponseWriter, ls []projects.Link, err error) {
	if ls == nil && err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	res := map[string]any{"links": ls}
	if err != nil {
		res["error"] = err.Error() // partial: some links made, some not
	}
	writeJSON(w, res)
}

func registerProjects(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, projects.List())
	})
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		p, err := underHome(strings.TrimSpace(req.Path))
		if err == nil {
			err = projects.Add(p)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, map[string]string{"path": p})
	})
	// The system's choose-folder dialog: {path}, "" when cancelled.
	mux.HandleFunc("POST /api/projects/pick", func(w http.ResponseWriter, r *http.Request) {
		p, err := projects.PickFolder()
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, map[string]string{"path": p})
	})
	// Likely projects from AI tool sessions, editors and git repos.
	mux.HandleFunc("GET /api/projects/discover", func(w http.ResponseWriter, r *http.Request) {
		home, _ := os.UserHomeDir()
		writeJSON(w, projects.Discover(home))
	})
	mux.HandleFunc("DELETE /api/projects", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := project(w, r); ok {
			if err := projects.Remove(p); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		}
	})
	mux.HandleFunc("GET /api/projects/manifest", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := project(w, r); ok {
			m, err := projects.ReadManifest(p)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			writeJSON(w, m)
		}
	})
	mux.HandleFunc("PUT /api/projects/manifest", func(w http.ResponseWriter, r *http.Request) {
		p, ok := project(w, r)
		if !ok {
			return
		}
		var m projects.Manifest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&m); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := projects.WriteManifest(p, m); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, m)
	})
	mux.HandleFunc("GET /api/projects/status", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := project(w, r); ok {
			ls, err := projects.Status(p)
			linksReply(w, ls, err)
		}
	})
	mux.HandleFunc("POST /api/projects/apply", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := project(w, r); ok {
			ls, err := projects.Apply(p)
			linksReply(w, ls, err)
		}
	})
	// What the picker offers: every installed skill, and the tools that have
	// a project skills folder.
	mux.HandleFunc("GET /api/projects/skills-available", func(w http.ResponseWriter, r *http.Request) {
		type tool struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Installed bool   `json:"installed"`
			Dir       string `json:"dir"`
		}
		ts := adapters.Detect()
		var out []tool
		for _, d := range projects.Dirs {
			t := tool{ID: d.Tool, Name: d.Tool, Dir: d.Dir}
			for _, x := range ts {
				if x.ID == d.Tool {
					t.Name, t.Installed = x.Name, x.Installed
				}
			}
			out = append(out, t)
		}
		writeJSON(w, map[string]any{"skills": catalog.Local(ts), "tools": out})
	})
}
